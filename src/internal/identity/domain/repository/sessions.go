package repository

import (
	"context"
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
)

type SessionRepository interface {
	Create(ctx context.Context, s entity.Session) error
	Get(ctx context.Context, idHash string) (entity.Session, error)
	// Touch actualiza last_seen_at.
	Touch(ctx context.Context, idHash string, at time.Time) error
	// Delete no falla si la sesión ya no existe.
	Delete(ctx context.Context, idHash string) error
	// DeleteExpired borra las sesiones vencidas en now y las completas que
	// llevan más de idle sin uso. Devuelve cuántas borró.
	DeleteExpired(ctx context.Context, now time.Time, idle time.Duration) (int64, error)
}
