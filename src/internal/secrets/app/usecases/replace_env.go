package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/secrets/app/dto"
	"github.com/CPU-commits/muelle/src/internal/secrets/domain/entity"
	secretsrepo "github.com/CPU-commits/muelle/src/internal/secrets/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/secrets/domain/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// ReplaceEnv reemplaza todo el set de variables de un stage o una app (PUT),
// como el sync-env de GitHub Actions, y marca con cambios pendientes las apps afectadas.
type ReplaceEnv struct {
	tx     repository.Transactor
	owners service.OwnerResolver
	repo   secretsrepo.EnvRepository
	marker service.ChangeMarker
	crypto *service.EnvCrypto
	clock  clock.Clock
	audit  audit.Recorder
}

func NewReplaceEnv(tx repository.Transactor, owners service.OwnerResolver, repo secretsrepo.EnvRepository,
	marker service.ChangeMarker, crypto *service.EnvCrypto, clock clock.Clock, audit audit.Recorder) *ReplaceEnv {
	return &ReplaceEnv{tx: tx, owners: owners, repo: repo, marker: marker, crypto: crypto, clock: clock, audit: audit}
}

func (u *ReplaceEnv) Execute(ctx context.Context, in dto.ReplaceEnv) error {
	if in.Vars == nil {
		return validate.Errorf("vars", "es obligatorio")
	}
	if err := entity.CheckCount("vars", len(in.Vars)); err != nil {
		return err
	}
	now := u.clock.Now()
	vars, err := entity.NewEnvVars("vars", in.Vars, secretOrDefault(in.Secret), now)
	if err != nil {
		return err
	}
	// Un PUT reemplaza todo el set, así que sus límites no dependen de lo
	// guardado: se revisan antes de tomar el lock de escritura.
	if err := entity.CheckLimits("vars", vars); err != nil {
		return err
	}
	// Las variables, la marca de cambios pendientes y la auditoría van juntas:
	// si una falla, no queda ninguna.
	return u.tx.InTx(ctx, func(ctx context.Context) error {
		owner, err := resolveOwner(ctx, u.owners, in.Target)
		if err != nil {
			return err
		}
		sealed, err := u.crypto.Seal(owner, vars)
		if err != nil {
			return err
		}
		if err := u.repo.Replace(ctx, owner, sealed); err != nil {
			return err
		}
		if err := u.marker.Mark(ctx, owner, now); err != nil {
			return err
		}
		u.audit.Record(ctx, "env.replace", auditTarget(in.Target), map[string][]string{"keys": keysOf(vars)})
		return nil
	})
}
