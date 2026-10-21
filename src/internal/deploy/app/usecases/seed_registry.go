package usecases

import (
	"context"
	"errors"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/clock"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// SeedInitRegistry guarda el registry inicial de install
// (MUELLE_INIT_REGISTRY_FILE, contrato §2.10) solo en el primer arranque de
// la base.
type SeedInitRegistry struct {
	tx     srepo.Transactor
	marks  service.Marks
	create *CreateRegistry
}

func NewSeedInitRegistry(tx srepo.Transactor, marks service.Marks, regs repository.RegistryRepository, c clock.Clock, rec audit.Recorder) *SeedInitRegistry {
	return &SeedInitRegistry{tx: tx, marks: marks, create: NewCreateRegistry(tx, regs, c, rec)}
}

// Execute, en un InTx: Marks.MarkOnce(SettingInitRegistryApplied), siempre,
// haya o no un registry inicial. Si la marca ya estaba, no hace nada más: un
// registry que el admin borró en el panel no vuelve, aunque el secreto siga
// montado. Si la insertó y r no es nil, lo crea como POST /registries
// (cifrado con registries:password:<id>), con registry.create, target
// registry:<host>, details {"username", "source": "install"} y actor system
// (el ctx del arranque no trae un principal). Si ese host ya tiene un
// registry, no lo toca.
//
// r ya viene validado (settings.Load); se vuelve a validar como última
// barrera. Si algo falla, el InTx deshace la marca y el próximo arranque
// reintenta. Los errores no traen la contraseña: los de validate no llevan el
// valor, y la contraseña va cifrada antes de llegar a la base.
func (u *SeedInitRegistry) Execute(ctx context.Context, r *dto.InitRegistry) error {
	if r != nil {
		if err := validate.RegistryHost("host", r.Host); err != nil {
			return err
		}
		if err := validateCredentials(*r); err != nil {
			return err
		}
	}
	return u.tx.InTx(ctx, func(ctx context.Context) error {
		first, err := u.marks.MarkOnce(ctx, entity.SettingInitRegistryApplied)
		if err != nil || !first || r == nil {
			return err
		}
		_, err = u.create.create(ctx, *r, map[string]string{"username": r.Username, "source": "install"})
		if errors.Is(err, srepo.ErrConflict) {
			return nil // un INSERT que falla por UNIQUE no deshace la transacción en SQLite
		}
		return err
	})
}
