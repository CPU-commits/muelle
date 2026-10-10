// Package usecases tiene los casos de uso del módulo admin.
package usecases

import (
	"context"
	"errors"

	"github.com/CPU-commits/muelle/src/internal/admin/app/dto"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// loadSettings lee los ajustes; lo que no se guardó tiene su valor por defecto.
func loadSettings(ctx context.Context, repo repository.SettingsRepository) (entity.Settings, error) {
	var s entity.Settings
	v, _, err := repo.Get(ctx, entity.KeyRequire2FA)
	switch {
	case errors.Is(err, sharedrepo.ErrNotFound):
	case err != nil:
		return s, err
	default:
		s.Require2FA = entity.ParseBool(string(v))
	}
	return s, nil
}

type GetSettings struct {
	settings repository.SettingsRepository
}

func NewGetSettings(settings repository.SettingsRepository) *GetSettings {
	return &GetSettings{settings: settings}
}

func (u *GetSettings) Execute(ctx context.Context) (dto.SettingsOutput, error) {
	s, err := loadSettings(ctx, u.settings)
	return dto.SettingsOutput{Require2FA: s.Require2FA}, err
}

type PatchSettings struct {
	settings repository.SettingsRepository
	totp     service.TOTPStatus
	audit    audit.Recorder
	tx       sharedrepo.Transactor
}

func NewPatchSettings(settings repository.SettingsRepository, totp service.TOTPStatus, rec audit.Recorder, tx sharedrepo.Transactor) *PatchSettings {
	return &PatchSettings{settings: settings, totp: totp, audit: rec, tx: tx}
}

// Execute aplica los campos presentes y devuelve los ajustes resultantes.
// Activar require_2fa exige que quien lo pide tenga su propio TOTP (§10.1):
// sin esa guarda quedaría encerrado en la pantalla de configuración.
func (u *PatchSettings) Execute(ctx context.Context, in dto.PatchSettingsInput) (dto.SettingsOutput, error) {
	p, ok := shared.PrincipalFrom(ctx)
	if !ok {
		return dto.SettingsOutput{}, shared.ErrUnauthenticated
	}
	// Los ajustes son de la sesión de un usuario, nunca de un token de API
	// (aunque el patch venga vacío).
	if p.Token != nil {
		return dto.SettingsOutput{}, shared.ErrForbidden
	}
	if in.Require2FA != nil {
		err := u.tx.InTx(ctx, func(ctx context.Context) error {
			if *in.Require2FA {
				has, err := u.totp.HasTOTP(ctx, p.UserID)
				if err != nil {
					return err
				}
				if !has {
					return service.ErrTOTPRequiredFirst
				}
			}
			if err := u.settings.Set(ctx, entity.KeyRequire2FA, []byte(entity.FormatBool(*in.Require2FA)), false); err != nil {
				return err
			}
			u.audit.Record(ctx, "settings.update", "settings", map[string]bool{entity.KeyRequire2FA: *in.Require2FA})
			return nil
		})
		if err != nil {
			return dto.SettingsOutput{}, err
		}
	}
	return NewGetSettings(u.settings).Execute(ctx)
}
