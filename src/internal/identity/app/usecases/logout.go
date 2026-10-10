package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
)

// Logout cierra la sesión. Recibe el hash de la sesión del Principal, así no
// hace falta volver a leer la cookie.
type Logout struct {
	sessions repository.SessionRepository
}

func NewLogout(sessions repository.SessionRepository) *Logout { return &Logout{sessions: sessions} }

func (uc *Logout) Execute(ctx context.Context, sessionHash string) error {
	return uc.sessions.Delete(ctx, sessionHash)
}
