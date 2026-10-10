package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// ResetUserTOTP borra el 2FA de un usuario, para quien perdió su teléfono, y
// cierra todas sus sesiones: si el teléfono lo tiene otra persona, una sesión
// abierta con él deja de servir. Todo va en la misma transacción. Hecho el
// cambio, también olvida lo que había en memoria del 2FA anterior: los
// fallos y el bloqueo por códigos malos (así no queda bloqueado con el
// secreto nuevo) y los códigos usados.
type ResetUserTOTP struct {
	tx       sharedrepo.Transactor
	users    repository.UserRepository
	sessions repository.SessionRepository
	lockout  *service.TOTPLockout
	replay   *service.TOTPReplayGuard
	audit    audit.Recorder
}

func NewResetUserTOTP(tx sharedrepo.Transactor, users repository.UserRepository, sessions repository.SessionRepository,
	lockout *service.TOTPLockout, replay *service.TOTPReplayGuard, rec audit.Recorder) *ResetUserTOTP {
	return &ResetUserTOTP{tx: tx, users: users, sessions: sessions, lockout: lockout, replay: replay, audit: rec}
}

func (uc *ResetUserTOTP) Execute(ctx context.Context, username string) error {
	var userID int64
	err := uc.tx.InTx(ctx, func(ctx context.Context) error {
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
		userID = u.ID
		return nil
	})
	if err != nil {
		return err
	}
	uc.lockout.Reset(userID)
	uc.replay.Forget(userID)
	return nil
}
