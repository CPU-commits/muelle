package usecases

import (
	"context"
	"slices"

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
	// Un PATCH vacío ({}, o set y unset vacíos) no pide nada: se rechaza.
	if len(in.Set) == 0 && len(in.Unset) == 0 {
		return validate.Errorf("set", "indica al menos una variable en set o unset")
	}
	// El tamaño de la petición se revisa antes que cada clave y antes de tocar
	// la base: un cuerpo de 1 MiB puede traer 100k claves en unset, que serían
	// 100k DELETE con el lock de escritura tomado y 1 MiB en la auditoría.
	if len(in.Unset) > entity.MaxVars {
		return validate.Errorf("unset", "máximo %d claves", entity.MaxVars)
	}
	if err := entity.CheckCount("set", len(in.Set)); err != nil {
		return err
	}
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

	// Los límites, las variables, la marca de cambios pendientes y la
	// auditoría van en la misma transacción: si una falla, no queda ninguna.
	return u.tx.InTx(ctx, func(ctx context.Context) error {
		owner, err := resolveOwner(ctx, u.owners, in.Target)
		if err != nil {
			return err
		}
		current, err := u.repo.List(ctx, owner)
		if err != nil {
			return err
		}
		// Borrar solo claves que no existen no cambia ninguna fila: no hay
		// nada que marcar ni que auditar.
		if len(set) == 0 && !hasAnyKey(current, unset) {
			return nil
		}
		// Los límites cuentan lo que el dueño ya tiene. Solo se pueden superar
		// agregando; borrar siempre se permite, aunque el dueño ya esté pasado.
		if len(set) > 0 {
			// Los valores guardados se descifran solo para medirlos, en memoria.
			plain, err := u.crypto.Open(owner, current)
			if err != nil {
				return err
			}
			if err := entity.CheckLimits("set", entity.ApplyPatch(plain, set, unset)); err != nil {
				return err
			}
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

// hasAnyKey dice si alguna de keys está en current.
func hasAnyKey(current []entity.EncryptedEnvVar, keys []string) bool {
	for _, v := range current {
		if slices.Contains(keys, v.Key) {
			return true
		}
	}
	return false
}
