package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/package/cipher"
)

// BeginTOTPSetup genera un secreto TOTP nuevo y lo deja pendiente, cifrado
// con la AAD de pendiente, hasta que EnableTOTP lo confirme con un código.
type BeginTOTPSetup struct {
	users  repository.UserRepository
	cipher cipher.Cipher
	totp   service.TOTP
}

func NewBeginTOTPSetup(users repository.UserRepository, c cipher.Cipher, totp service.TOTP) *BeginTOTPSetup {
	return &BeginTOTPSetup{users: users, cipher: c, totp: totp}
}

func (uc *BeginTOTPSetup) Execute(ctx context.Context, userID int64) (dto.TOTPSetupOutput, error) {
	u, err := uc.users.GetByID(ctx, userID)
	if err != nil {
		return dto.TOTPSetupOutput{}, err
	}
	secret, url, err := uc.totp.Generate(u.Username)
	if err != nil {
		return dto.TOTPSetupOutput{}, err
	}
	encrypted := uc.cipher.Encrypt([]byte(secret), service.TOTPPendingAAD(u.ID))
	if err := uc.users.SetTOTPPending(ctx, u.ID, encrypted); err != nil {
		return dto.TOTPSetupOutput{}, err
	}
	return dto.TOTPSetupOutput{Secret: secret, URL: url}, nil
}
