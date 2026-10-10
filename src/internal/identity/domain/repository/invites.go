package repository

import (
	"context"
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
)

type InviteRepository interface {
	Create(ctx context.Context, inv entity.Invite) error
	GetByTokenHash(ctx context.Context, tokenHash string) (entity.Invite, error)
	// MarkUsed marca la invitación como usada en at. Si no existe o ya estaba
	// usada devuelve ErrNotFound.
	MarkUsed(ctx context.Context, id int64, at time.Time) error
	// DeleteUnusedSystem borra los links de setup del sistema que no se usaron.
	DeleteUnusedSystem(ctx context.Context) error
}
