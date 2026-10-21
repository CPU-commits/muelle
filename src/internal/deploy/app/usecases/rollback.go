package usecases

import (
	"context"
	"maps"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
)

// Rollback encola un deploy con las imágenes del succeeded anterior al
// actual (POST …/rollback, contrato §2.7).
type Rollback struct{ d Deps }

func NewRollback(d Deps) *Rollback { return &Rollback{d: d} }

// CheckApp es el 404 de una app que no existe, antes de leer el cuerpo.
func (u *Rollback) CheckApp(ctx context.Context, ref dto.AppRef) error {
	_, err := u.d.Apps.Get(ctx, ref.Project, ref.Stage, ref.App)
	return err
}

// Execute elige el destino al encolar, en la misma transacción que el
// INSERT: las refs resueltas (con digest) del succeeded anterior al actual
// pasan a ser los overrides. Sin deploy actual o sin uno anterior,
// ErrNoPreviousDeploy. Como pide refs con digest, el próximo deploy sin
// cuerpo las repite (contrato §2.2): el rollback no se deshace solo.
func (u *Rollback) Execute(ctx context.Context, ref dto.AppRef) (dto.Queued, error) {
	app, err := u.d.acceptingApp(ctx, ref)
	if err != nil {
		return dto.Queued{}, err
	}
	a, err := actorOf(ctx)
	if err != nil {
		return dto.Queued{}, err
	}
	a.trigger = entity.TriggerRollback
	return u.d.enqueue(ctx, app, a, func(ctx context.Context) (map[string]string, error) {
		cur, err := u.d.Deploys.Current(ctx, app.ID)
		if err != nil {
			return nil, err
		}
		if cur == nil {
			return nil, service.ErrNoPreviousDeploy
		}
		prev, err := u.d.Deploys.PreviousSucceeded(ctx, app.ID, cur.ID)
		if err != nil {
			return nil, err
		}
		if prev == nil {
			return nil, service.ErrNoPreviousDeploy
		}
		return maps.Clone(prev.Images), nil
	})
}
