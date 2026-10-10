package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/package/audit"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// ResetUserTOTP borra el 2FA de un usuario, para quien perdió su teléfono, y
// cierra todas sus sesiones: si el teléfono lo tiene otra persona, una sesión
// abierta con él deja de servir. Todo va en la misma transacción.
type ResetUserTOTP struct {
	tx       sharedrepo.Transactor
	users    repository.UserRepository
	sessions repository.SessionRepository
	audit    audit.Recorder
}

func NewResetUserTOTP(tx sharedrepo.Transactor, users repository.UserRepository, sessions repository.SessionRepository,
	rec audit.Recorder) *ResetUserTOTP {
	return &ResetUserTOTP{tx: tx, users: users, sessions: sessions, audit: rec}
}

func (uc *ResetUserTOTP) Execute(ctx context.Context, username string) error {
	return uc.tx.InTx(ctx, func(ctx context.Context) error {
		u, err := uc.users.GetByUsername(ctx, username)
		if err != nil {
			return err
		}
		if err := uc.users.ClearTOTP(ctx, u.ID); err != nil {
			return err
		}
		if _, err := uc.sessions.DeleteByUser(ctx, u.ID); err != nil {
			return err
		}
		uc.audit.Record(ctx, "user.totp_reset", "user:"+u.Username, nil)
		return nil
	})
}
