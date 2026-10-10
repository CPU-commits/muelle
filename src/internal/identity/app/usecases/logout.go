package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
)

// Logout cierra la sesión. Recibe el hash de la sesión del Principal, así no
// hace falta volver a leer la cookie.
type Logout struct {
	sessions repository.SessionRepository
	audit    *LoginAudit
}

func NewLogout(sessions repository.SessionRepository, la *LoginAudit) *Logout {
	return &Logout{sessions: sessions, audit: la}
}

func (uc *Logout) Execute(ctx context.Context, in dto.LogoutInput) error {
	if err := uc.sessions.Delete(ctx, in.SessionHash); err != nil {
		return err
	}
	uc.audit.logout(ctx, in.Username)
	return nil
}
