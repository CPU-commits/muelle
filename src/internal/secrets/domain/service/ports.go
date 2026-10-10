package service

import (
	"context"
	"time"

	"github.com/CPU-commits/muelle/src/internal/secrets/domain/entity"
)

// OwnerResolver traduce los slugs de la ruta al dueño de las variables.
// Los stages y las apps son de catalog; el adaptador lee sus tablas.
// Si un slug no existe devuelve repository.ErrNotFound (shared/domain/repository).
type OwnerResolver interface {
	Stage(ctx context.Context, project, stage string) (entity.Owner, error)
	App(ctx context.Context, project, stage, app string) (entity.Owner, error)
}

// ChangeMarker marca con cambios pendientes las apps afectadas por un cambio
// de variables (§5.2 del spec): para una app, su config_changed_at; para un
// stage, el de todas sus apps. Se llama en el mismo InTx que el cambio.
type ChangeMarker interface {
	Mark(ctx context.Context, owner entity.Owner, at time.Time) error
}
