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
	tx       sharedrepo.Transactor
}

func NewVerifyMasterKey(settings repository.SettingsRepository, c cipher.Cipher, tx sharedrepo.Transactor) *VerifyMasterKey {
	return &VerifyMasterKey{settings: settings, cipher: c, tx: tx}
}

// Execute guarda el valor de control en el primer arranque y en los siguientes
// lo descifra. Con otra clave maestra devuelve service.ErrWrongMasterKey: sin
// esta comprobación muelle arrancaría y fallaría recién al leer un secreto.
//
// Todo corre en una transacción y el primer guardado no pisa un valor
// existente, así que dos primeros arranques concurrentes con claves distintas
// no pasan los dos: el que pierde lee el valor del otro y falla.
func (u *VerifyMasterKey) Execute(ctx context.Context) error {
	return u.tx.InTx(ctx, func(ctx context.Context) error {
		_, _, err := u.settings.Get(ctx, entity.KeyMasterKeyCheck)
		if errors.Is(err, sharedrepo.ErrNotFound) {
			ct := u.cipher.Encrypt([]byte(masterKeyCheck), entity.MasterKeyCheckAAD)
			err = u.settings.Insert(ctx, entity.KeyMasterKeyCheck, ct, true)
			if err != nil && !errors.Is(err, sharedrepo.ErrConflict) {
				return err
			}
		} else if err != nil {
			return err
		}
		// Se relee: vale lo que quedó guardado, sea lo nuestro o lo de otro.
		stored, encrypted, err := u.settings.Get(ctx, entity.KeyMasterKeyCheck)
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
	})
}
