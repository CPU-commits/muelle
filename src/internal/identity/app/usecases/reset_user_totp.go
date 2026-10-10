package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/package/audit"
)

// ResetUserTOTP borra el 2FA de un usuario, para quien perdió su teléfono.
type ResetUserTOTP struct {
	users repository.UserRepository
	audit audit.Recorder
}

func NewResetUserTOTP(users repository.UserRepository, rec audit.Recorder) *ResetUserTOTP {
	return &ResetUserTOTP{users: users, audit: rec}
}

func (uc *ResetUserTOTP) Execute(ctx context.Context, username string) error {
	u, err := uc.users.GetByUsername(ctx, username)
	if err != nil {
		return err
	}
	if err := uc.users.ClearTOTP(ctx, u.ID); err != nil {
		return err
	}
	uc.audit.Record(ctx, "user.totp_reset", "user:"+u.Username, nil)
	return nil
}
