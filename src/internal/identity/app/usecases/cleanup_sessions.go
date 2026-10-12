package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
)

// CleanupSessions borra las sesiones vencidas o sin uso por más de
// SessionIdle. Lo corre un job cada hora.
type CleanupSessions struct {
	sessions repository.SessionRepository
	clock    clock.Clock
}

func NewCleanupSessions(sessions repository.SessionRepository, c clock.Clock) *CleanupSessions {
	return &CleanupSessions{sessions: sessions, clock: c}
}

// Execute devuelve cuántas sesiones borró.
func (uc *CleanupSessions) Execute(ctx context.Context) (int64, error) {
	return uc.sessions.DeleteExpired(ctx, uc.clock.Now(), service.SessionIdle)
}
