package usecases

import (
	"context"
	"errors"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/cipher"
	"github.com/CPU-commits/muelle/src/package/clock"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// EnableTOTP confirma el secreto pendiente con un código y lo activa. El
// secreto se descifra con la AAD de pendiente y se vuelve a cifrar con la del
// activo. Leer, validar y activar van en una transacción (BEGIN IMMEDIATE):
// un BeginTOTPSetup a la vez espera a que termine, así se activa exactamente
// el secreto que se validó y el nuevo queda pendiente en vez de perderse.
type EnableTOTP struct {
	tx     sharedrepo.Transactor
	users  repository.UserRepository
	cipher cipher.Cipher
	totp   service.TOTP
	audit  audit.Recorder
	clock  clock.Clock
}

func NewEnableTOTP(tx sharedrepo.Transactor, users repository.UserRepository, c cipher.Cipher, totp service.TOTP,
	rec audit.Recorder, clk clock.Clock) *EnableTOTP {
	return &EnableTOTP{tx: tx, users: users, cipher: c, totp: totp, audit: rec, clock: clk}
}

func (uc *EnableTOTP) Execute(ctx context.Context, in dto.EnableTOTPInput) error {
	return uc.tx.InTx(ctx, func(ctx context.Context) error {
		u, err := uc.users.GetByID(ctx, in.UserID)
		if err != nil {
			return err
		}
		if !u.HasPendingTOTP() {
			return service.ErrNoPendingTOTP
		}
		secret, err := uc.cipher.Decrypt(u.TOTPPending, service.TOTPPendingAAD(u.ID))
		if err != nil {
			return err
		}
		if !uc.totp.Validate(in.Code, string(secret), uc.clock.Now()) {
			return service.ErrInvalidCode
		}
		encrypted := uc.cipher.Encrypt(secret, service.TOTPSecretAAD(u.ID))
		if err := uc.users.ActivateTOTP(ctx, u.ID, encrypted); err != nil {
			if errors.Is(err, sharedrepo.ErrNotFound) {
				return service.ErrNoPendingTOTP
			}
			return err
		}
		uc.audit.Record(ctx, "user.totp_enable", "user:"+u.Username, nil)
		return nil
	})
}
