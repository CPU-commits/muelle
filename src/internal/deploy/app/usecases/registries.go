package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/repository"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/clock"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// Los registries (contrato §2.10): las credenciales de los registries
// privados, una por host. La contraseña nunca vuelve: los casos de uso
// devuelven el registry sin ella, y la auditoría lleva solo el usuario.

func registryTarget(host string) string { return "registry:" + host }

// validateCredentials aplica las reglas de shared/domain/validate al usuario
// y la contraseña (los mensajes no traen el valor).
func validateCredentials(in dto.RegistryInput) error {
	if err := validate.RegistryUsername("username", in.Username); err != nil {
		return err
	}
	return validate.RegistryPassword("password", in.Password)
}

// ListRegistries lista los registries por host, sin la contraseña.
type ListRegistries struct {
	regs repository.RegistryRepository
}

func NewListRegistries(regs repository.RegistryRepository) *ListRegistries {
	return &ListRegistries{regs: regs}
}

func (u *ListRegistries) Execute(ctx context.Context) ([]entity.Registry, error) {
	return u.regs.List(ctx)
}

// CreateRegistry guarda la credencial de un host nuevo.
type CreateRegistry struct {
	tx    srepo.Transactor
	regs  repository.RegistryRepository
	clock clock.Clock
	audit audit.Recorder
}

func NewCreateRegistry(tx srepo.Transactor, regs repository.RegistryRepository, c clock.Clock, rec audit.Recorder) *CreateRegistry {
	return &CreateRegistry{tx: tx, regs: regs, clock: c, audit: rec}
}

// Execute valida host, usuario y contraseña (422) y guarda el registry
// (ErrConflict si el host ya está), con registry.create en la misma
// transacción. Devuelve el registry sin la contraseña.
func (u *CreateRegistry) Execute(ctx context.Context, in dto.RegistryInput) (entity.Registry, error) {
	if err := validate.RegistryHost("host", in.Host); err != nil {
		return entity.Registry{}, err
	}
	if err := validateCredentials(in); err != nil {
		return entity.Registry{}, err
	}
	return u.create(ctx, in, map[string]string{"username": in.Username})
}

// create guarda el registry y su evento en una transacción (la del ctx si
// trae una). details nunca lleva la contraseña.
func (u *CreateRegistry) create(ctx context.Context, in dto.RegistryInput, details map[string]string) (entity.Registry, error) {
	now := u.clock.Now()
	reg := entity.Registry{Host: in.Host, Username: in.Username, Password: in.Password, CreatedAt: now, UpdatedAt: now}
	err := u.tx.InTx(ctx, func(ctx context.Context) error {
		if err := u.regs.Create(ctx, &reg); err != nil {
			return err
		}
		u.audit.Record(ctx, "registry.create", registryTarget(reg.Host), details)
		return nil
	})
	reg.Password = ""
	if err != nil {
		return entity.Registry{}, err
	}
	return reg, nil
}

// UpdateRegistry reemplaza el usuario y la contraseña de un host (PATCH: los
// dos obligatorios).
type UpdateRegistry struct {
	tx    srepo.Transactor
	regs  repository.RegistryRepository
	clock clock.Clock
	audit audit.Recorder
}

func NewUpdateRegistry(tx srepo.Transactor, regs repository.RegistryRepository, c clock.Clock, rec audit.Recorder) *UpdateRegistry {
	return &UpdateRegistry{tx: tx, regs: regs, clock: c, audit: rec}
}

// Execute: in.Host es el de la ruta (un host que no existe es ErrNotFound,
// también uno que no tiene la forma canónica).
func (u *UpdateRegistry) Execute(ctx context.Context, in dto.RegistryInput) (entity.Registry, error) {
	if err := validateCredentials(in); err != nil {
		return entity.Registry{}, err
	}
	reg := entity.Registry{Host: in.Host, Username: in.Username, Password: in.Password, UpdatedAt: u.clock.Now()}
	err := u.tx.InTx(ctx, func(ctx context.Context) error {
		if err := u.regs.Update(ctx, &reg); err != nil {
			return err
		}
		u.audit.Record(ctx, "registry.update", registryTarget(reg.Host), map[string]string{"username": reg.Username})
		return nil
	})
	reg.Password = ""
	if err != nil {
		return entity.Registry{}, err
	}
	return reg, nil
}

// DeleteRegistry borra la credencial de un host. No redespliega nada.
type DeleteRegistry struct {
	tx    srepo.Transactor
	regs  repository.RegistryRepository
	audit audit.Recorder
}

func NewDeleteRegistry(tx srepo.Transactor, regs repository.RegistryRepository, rec audit.Recorder) *DeleteRegistry {
	return &DeleteRegistry{tx: tx, regs: regs, audit: rec}
}

func (u *DeleteRegistry) Execute(ctx context.Context, host string) error {
	return u.tx.InTx(ctx, func(ctx context.Context) error {
		if err := u.regs.Delete(ctx, host); err != nil {
			return err
		}
		u.audit.Record(ctx, "registry.delete", registryTarget(host), nil)
		return nil
	})
}
