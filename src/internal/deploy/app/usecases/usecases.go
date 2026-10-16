package usecases

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// Deps son las piezas que comparten los casos de uso de los deploys.
type Deps struct {
	Tx      srepo.Transactor
	Apps    service.Apps
	Deploys repository.DeployRepository
	Queue   service.Queue
	Hub     *Hub
	Clock   clock.Clock
	Audit   audit.Recorder
}

func appTarget(a service.App) string { return "app:" + a.Project + "/" + a.Stage + "/" + a.Slug }

// statusURL es la ruta de un deploy (contrato §2.2: status_url y Location).
func statusURL(a service.App, id int64) string {
	return fmt.Sprintf("/api/v1/projects/%s/stages/%s/apps/%s/deploys/%d",
		url.PathEscape(a.Project), url.PathEscape(a.Stage), url.PathEscape(a.Slug), id)
}

// actor es quién pide un deploy (contrato §2.1), del principal del ctx: una
// sesión es panel/user y un token es api/token. El nombre se copia al crear
// el deploy, así sigue aunque se borre el usuario o el token.
type actor struct {
	trigger entity.Trigger
	typ     string
	id      int64
	name    string
}

// errNoActor: un deploy sin principal no tendría a quién atribuirse. Las
// rutas siempre lo ponen; es un error de programación (500).
var errNoActor = errors.New("deploy: la petición no trae un usuario ni un token")

func actorOf(ctx context.Context) (actor, error) {
	p, ok := shared.PrincipalFrom(ctx)
	switch {
	case !ok:
		return actor{}, errNoActor
	case p.Token != nil:
		return actor{trigger: entity.TriggerAPI, typ: entity.ActorToken, id: p.Token.ID, name: p.Token.Name}, nil
	case p.UserID != 0:
		return actor{trigger: entity.TriggerPanel, typ: entity.ActorUser, id: p.UserID, name: p.Username}, nil
	}
	return actor{}, errNoActor
}

// acceptingApp lee la app (404) y verifica que acepte deploys (409 mientras
// se borra, contrato §1.1).
func (d Deps) acceptingApp(ctx context.Context, ref dto.AppRef) (service.App, error) {
	app, err := d.Apps.Get(ctx, ref.Project, ref.Stage, ref.App)
	if err != nil {
		return service.App{}, err
	}
	if !d.Queue.Accepting(app.ID) {
		return service.App{}, service.ErrAppRemoving
	}
	return app, nil
}

// enqueue encola un deploy de la app (§7 paso 1): en una transacción, lo
// inserta queued con las imágenes que da images (los overrides), deja
// superseded los otros queued de la app y guarda deploy.create. Después del
// commit avisa a los streams de los reemplazados (contrato §2.6) y despierta
// a la cola. No llama a Docker.
func (d Deps) enqueue(ctx context.Context, app service.App, a actor, images func(ctx context.Context) (map[string]string, error)) (dto.Queued, error) {
	var dep entity.Deploy
	var superseded []int64
	err := d.Tx.InTx(ctx, func(ctx context.Context) error {
		imgs, err := images(ctx)
		if err != nil {
			return err
		}
		if imgs == nil {
			imgs = map[string]string{}
		}
		now := d.Clock.Now()
		dep = entity.Deploy{AppID: app.ID, Trigger: a.trigger, Images: imgs,
			ActorType: a.typ, ActorID: &a.id, ActorName: a.name, CreatedAt: now}
		if err := d.Deploys.Create(ctx, &dep); err != nil {
			return err
		}
		if superseded, err = d.Deploys.SupersedeQueued(ctx, app.ID, dep.ID, now); err != nil {
			return err
		}
		d.Audit.Record(ctx, "deploy.create", appTarget(app),
			map[string]any{"deploy": dep.ID, "trigger": string(dep.Trigger), "images": imgs})
		return nil
	})
	if err != nil {
		return dto.Queued{}, err
	}
	for _, id := range superseded {
		d.Hub.Notify(id)
	}
	d.Queue.Wake(app.ID)
	return dto.Queued{ID: dep.ID, Status: entity.StatusQueued, StatusURL: statusURL(app, dep.ID)}, nil
}
