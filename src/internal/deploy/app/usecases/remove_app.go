// Package usecases tiene los casos de uso del módulo deploy.
package usecases

import (
	"context"
	"errors"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// removeBlock es cuánto deja de aceptar deploys una app que se está
// borrando: lo que tarda catalog en borrar la fila (contrato §1.1).
const removeBlock = 2 * time.Minute

// Workers es lo que RemoveApp usa del despachador.
type Workers interface {
	Block(appID int64, dur time.Duration)
	Unblock(appID int64)
	// Stop detiene el worker de la app; service.ErrWorkerBusy si no terminó
	// en su plazo.
	Stop(ctx context.Context, appID int64) error
}

// RemoveApp borra lo que una app tiene en Swarm. La llama catalog al borrar la
// app, antes de borrar la fila.
type RemoveApp struct {
	apps    service.Apps
	deploys repository.DeployRepository
	stacks  service.Stacks
	workers Workers
	hub     *Hub
	clock   clock.Clock
}

func NewRemoveApp(apps service.Apps, deploys repository.DeployRepository, stacks service.Stacks, w Workers, hub *Hub, c clock.Clock) *RemoveApp {
	return &RemoveApp{apps: apps, deploys: deploys, stacks: stacks, workers: w, hub: hub, clock: c}
}

// Execute, en orden (contrato §1.1, decisión 8 del plan): deja de aceptar
// deploys de la app por 2 min; detiene su worker (el deploy en curso cierra
// failed "la app se borró durante el deploy"); marca failed sus queued ("la
// app se borró") y avisa a sus streams; y ejecuta docker stack rm (los
// volúmenes quedan: sus datos no se borran en la v1). Un queued que llegue
// después lo cierra el despachador, que no reclama nada de una app bloqueada.
//
// Si el worker no se detiene en 15 s, no sigue: vuelve a aceptar deploys y
// devuelve service.ErrWorkerBusy (409), sin tocar los queued ni el stack;
// seguir con un worker vivo podría aplicar el stack después del stack rm. Si
// algo más falla, también vuelve a aceptar deploys. Una app que no está en la
// base: solo el stack rm.
func (u *RemoveApp) Execute(ctx context.Context, in dto.AppRef) error {
	stack := naming.Stack(in.Project, in.Stage, in.App)
	app, err := u.apps.Get(ctx, in.Project, in.Stage, in.App)
	if errors.Is(err, srepo.ErrNotFound) {
		return u.stacks.Remove(ctx, stack)
	}
	if err != nil {
		return err
	}
	u.workers.Block(app.ID, removeBlock)
	if err := u.workers.Stop(ctx, app.ID); err != nil {
		u.workers.Unblock(app.ID)
		return err
	}
	ids, err := u.deploys.FailQueued(ctx, app.ID, reasonQueuedRemoved, u.clock.Now())
	if err != nil {
		u.workers.Unblock(app.ID)
		return err
	}
	for _, id := range ids {
		u.hub.Notify(id)
	}
	if err := u.stacks.Remove(ctx, stack); err != nil {
		u.workers.Unblock(app.ID)
		return err
	}
	return nil
}
