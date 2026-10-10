package usecases

import (
	"context"
	"errors"
	"fmt"

	"github.com/CPU-commits/muelle/src/internal/admin/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/service"
	"github.com/CPU-commits/muelle/src/package/cipher"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// masterKeyCheck es el texto de control que se cifra con la clave maestra.
const masterKeyCheck = "muelle"

type VerifyMasterKey struct {
	settings repository.SettingsRepository
	cipher   cipher.Cipher
}

func NewVerifyMasterKey(settings repository.SettingsRepository, c cipher.Cipher) *VerifyMasterKey {
	return &VerifyMasterKey{settings: settings, cipher: c}
}

// Execute guarda el valor de control en el primer arranque y en los siguientes
// lo descifra. Con otra clave maestra devuelve service.ErrWrongMasterKey: sin
// esta comprobación muelle arrancaría y fallaría recién al leer un secreto.
func (u *VerifyMasterKey) Execute(ctx context.Context) error {
	stored, encrypted, err := u.settings.Get(ctx, entity.KeyMasterKeyCheck)
	if errors.Is(err, sharedrepo.ErrNotFound) {
		ct := u.cipher.Encrypt([]byte(masterKeyCheck), entity.MasterKeyCheckAAD)
		return u.settings.Set(ctx, entity.KeyMasterKeyCheck, ct, true)
	}
	if err != nil {
		return err
	}
	if !encrypted {
		return fmt.Errorf("%w: el valor de control no está cifrado", service.ErrWrongMasterKey)
	}
	pt, err := u.cipher.Decrypt(stored, entity.MasterKeyCheckAAD)
	if err != nil || string(pt) != masterKeyCheck {
		return service.ErrWrongMasterKey
	}
	return nil
}
