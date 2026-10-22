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
		stored := make(map[string]entity.EncryptedEnvVar, len(current))
		for _, r := range current {
			stored[r.Key] = r
		}
		if in.KeepSecrets {
			if err := u.keepSecrets(ctx, owner, stored, set); err != nil {
				return err
			}
		}
		// Una entrada de set con el mismo valor y el mismo secret que la
		// guardada no es un cambio y no se reescribe. Solo se descifran las
		// claves de set que ya existen; una que no descifra cuenta como cambiada.
		changed := make([]entity.EnvVar, 0, len(set))
		for _, v := range set {
			if r, ok := stored[v.Key]; ok && u.crypto.Unchanged(owner, r, v) {
				continue
			}
			changed = append(changed, v)
		}
		// Si nada cambia (todo set igual y unset de claves que no existen),
		// no hay nada que escribir, marcar ni auditar.
		if len(changed) == 0 && !hasAnyKey(stored, unset) {
			return nil
		}
		// Los límites cuentan lo que el dueño ya tiene, medido sin descifrar.
		// Solo se pueden superar agregando; borrar siempre se permite, aunque
		// el dueño ya esté pasado.
		if len(changed) > 0 {
			sizes := make(map[string]int, len(current))
			for _, r := range current {
				sizes[r.Key] = u.crypto.StoredSize(r)
			}
			count, size := entity.PatchTotals(sizes, changed, unset)
			if err := entity.CheckTotals("set", count, size); err != nil {
				return err
			}
		}
		sealed, err := u.crypto.Seal(owner, changed)
		if err != nil {
			return err
		}
		if err := u.repo.Patch(ctx, owner, sealed, unset); err != nil {
			return err
		}
		if err := u.marker.Mark(ctx, owner, now); err != nil {
			return err
		}
		// La auditoría lista las claves pedidas, también las que no cambiaron.
		u.audit.Record(ctx, "env.patch", auditTarget(in.Target), map[string][]string{"set": keysOf(set), "unset": unset})
		return nil
	})
}

// keepSecrets rechaza una clave de set que ya es secreta en el dueño o, con
// una app, en su stage (las variables del stage, leídas en la misma
// transacción): la de la app la taparía en el próximo deploy. Al revés no
// hace falta: una del stage no tapa una secreta de una app, porque la de la
// app gana.
func (u *PatchEnv) keepSecrets(ctx context.Context, owner entity.Owner, stored map[string]entity.EncryptedEnvVar, set []entity.EnvVar) error {
	for _, v := range set {
		if r, ok := stored[v.Key]; ok && r.Secret {
			return validate.Errorf("set", "%q es secreta: cámbiala desde el panel o la GitHub Action", v.Key)
		}
	}
	if owner.Kind != entity.OwnerApp || len(set) == 0 {
		return nil
	}
	stage, err := u.owners.StageOfApp(ctx, owner.ID)
	if err != nil {
		return err
	}
	stageVars, err := u.repo.List(ctx, stage)
	if err != nil {
		return err
	}
	secret := make(map[string]bool, len(stageVars))
	for _, r := range stageVars {
		secret[r.Key] = r.Secret
	}
	for _, v := range set {
		if secret[v.Key] {
			return validate.Errorf("set", "%q es secreta en el stage, y la de la app la taparía: cámbiala desde el panel o la GitHub Action", v.Key)
		}
	}
	return nil
}

// hasAnyKey dice si alguna de keys está guardada.
func hasAnyKey(stored map[string]entity.EncryptedEnvVar, keys []string) bool {
	for _, k := range keys {
		if _, ok := stored[k]; ok {
			return true
		}
	}
	return false
}
