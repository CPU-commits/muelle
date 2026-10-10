package repository

import (
	"context"
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
)

// TokenRepository guarda los tokens de API. Los tokens que devuelve traen
// los slugs de su proyecto y stage. Un token se borra en cascada con su
// proyecto o su stage.
type TokenRepository interface {
	// Create guarda t (sin id, con ProjectID y StageID) y devuelve el token
	// guardado. Si el proyecto o el stage ya no existen devuelve ErrNotFound.
	Create(ctx context.Context, t entity.APIToken) (entity.APIToken, error)
	GetByHash(ctx context.Context, tokenHash string) (entity.APIToken, error)
	// List devuelve todos los tokens en orden de creación.
	List(ctx context.Context) ([]entity.APIToken, error)
	Delete(ctx context.Context, id int64) error
	// Touch actualiza last_used_at.
	Touch(ctx context.Context, id int64, at time.Time) error
}
