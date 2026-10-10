package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/package/audit"
)

// Logout cierra la sesión. Recibe el hash de la sesión del Principal, así no
// hace falta volver a leer la cookie.
type Logout struct {
	sessions repository.SessionRepository
	audit    loginAudit
}

func NewLogout(sessions repository.SessionRepository, rec audit.Recorder) *Logout {
	return &Logout{sessions: sessions, audit: loginAudit{rec: rec}}
}

func (uc *Logout) Execute(ctx context.Context, in dto.LogoutInput) error {
	if err := uc.sessions.Delete(ctx, in.SessionHash); err != nil {
		return err
	}
	uc.audit.logout(ctx, in.Username)
	return nil
}
