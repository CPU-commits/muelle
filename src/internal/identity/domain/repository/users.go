// Package repository define cómo identity guarda usuarios, sesiones,
// invitaciones y tokens. Los métodos devuelven los errores comunes de
// shared/domain/repository (ErrNotFound, ErrConflict) y participan en la
// transacción del ctx si hay una (repository.Transactor).
package repository

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
)

// UserRepository guarda los usuarios. Los métodos que reciben un id devuelven
// ErrNotFound si el usuario no existe.
type UserRepository interface {
	// Create guarda u (sin id) y devuelve el usuario guardado. Si el nombre
	// ya existe devuelve ErrConflict.
	Create(ctx context.Context, u entity.User) (entity.User, error)
	GetByID(ctx context.Context, id int64) (entity.User, error)
	GetByUsername(ctx context.Context, username string) (entity.User, error)
	// List devuelve todos los usuarios ordenados por nombre.
	List(ctx context.Context) ([]entity.User, error)
	Count(ctx context.Context) (int, error)
	// Delete borra el usuario; sus sesiones e invitaciones se borran en
	// cascada y el created_by de quienes creó queda en NULL.
	Delete(ctx context.Context, id int64) error

	// SetTOTPPending guarda un secreto TOTP recién generado, ya cifrado.
	SetTOTPPending(ctx context.Context, id int64, encrypted []byte) error
	// ActivateTOTP guarda el secreto activo (ya cifrado) y borra el
	// pendiente. Si no había uno pendiente devuelve ErrNotFound.
	ActivateTOTP(ctx context.Context, id int64, encrypted []byte) error
	// ClearTOTP borra el secreto activo y el pendiente.
	ClearTOTP(ctx context.Context, id int64) error
}
