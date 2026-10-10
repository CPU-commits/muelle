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

// PatchEnv agrega o reemplaza variables (set) y borra otras (unset) de un
// stage o una app (PATCH), y marca con cambios pendientes las apps afectadas.
type PatchEnv struct {
	tx     repository.Transactor
	owners service.OwnerResolver
	repo   secretsrepo.EnvRepository
	marker service.ChangeMarker
	crypto *service.EnvCrypto
	clock  clock.Clock
	audit  audit.Recorder
}

func NewPatchEnv(tx repository.Transactor, owners service.OwnerResolver, repo secretsrepo.EnvRepository,
	marker service.ChangeMarker, crypto *service.EnvCrypto, clock clock.Clock, audit audit.Recorder) *PatchEnv {
	return &PatchEnv{tx: tx, owners: owners, repo: repo, marker: marker, crypto: crypto, clock: clock, audit: audit}
}

func (u *PatchEnv) Execute(ctx context.Context, in dto.PatchEnv) error {
	for _, k := range in.Unset {
		if err := validate.EnvKey("unset", k); err != nil {
			return err
		}
		if _, ok := in.Set[k]; ok {
			return validate.Errorf("unset", "%q no puede estar en set y unset a la vez", k)
		}
	}
	now := u.clock.Now()
	set, err := entity.NewEnvVars("set", in.Set, secretOrDefault(in.Secret), now)
	if err != nil {
		return err
	}
	unset := sortedUnique(in.Unset)

	// El conteo, las variables, la marca de cambios pendientes y la auditoría
	// van en la misma transacción: si una falla, no queda ninguna.
	return u.tx.InTx(ctx, func(ctx context.Context) error {
		owner, err := resolveOwner(ctx, u.owners, in.Target)
		if err != nil {
			return err
		}
		current, err := u.repo.List(ctx, owner)
		if err != nil {
			return err
		}
		if err := entity.CheckCount("set", patchedCount(current, set, unset)); err != nil {
			return err
		}
		sealed, err := u.crypto.Seal(owner, set)
		if err != nil {
			return err
		}
		if err := u.repo.Patch(ctx, owner, sealed, unset); err != nil {
			return err
		}
		if err := u.marker.Mark(ctx, owner, now); err != nil {
			return err
		}
		u.audit.Record(ctx, "env.patch", auditTarget(in.Target), map[string][]string{"set": keysOf(set), "unset": unset})
		return nil
	})
}

// patchedCount es cuántas variables quedan después del patch.
func patchedCount(current []entity.EncryptedEnvVar, set []entity.EnvVar, unset []string) int {
	total := make(map[string]bool, len(current)+len(set))
	for _, v := range current {
		total[v.Key] = true
	}
	for _, v := range set {
		total[v.Key] = true
	}
	for _, k := range unset {
		delete(total, k)
	}
	return len(total)
}
