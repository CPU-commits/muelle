// Package entity tiene las entidades de identity: usuarios, sesiones,
// invitaciones y tokens de API.
package entity

import (
	"time"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// User es una cuenta del panel. Los secretos TOTP van cifrados: TOTPSecret
// con la AAD de service.TOTPSecretAAD y TOTPPending con la de
// service.TOTPPendingAAD.
type User struct {
	ID           int64
	Username     string
	PasswordHash string // argon2id en formato PHC
	TOTPSecret   []byte // cifrado; vacío si no tiene 2FA
	TOTPPending  []byte // cifrado; generado y esperando que lo confirme con un código
	CreatedBy    *int64 // nil: el primer usuario, o su creador ya no existe
	CreatedAt    time.Time
}

// NewUser valida el nombre y arma un usuario nuevo, todavía sin id.
func NewUser(username, passwordHash string, createdBy *int64, now time.Time) (User, error) {
	if err := validate.Username(username); err != nil {
		return User{}, err
	}
	return User{Username: username, PasswordHash: passwordHash, CreatedBy: createdBy, CreatedAt: now}, nil
}

// HasTOTP dice si el usuario tiene 2FA activo.
func (u User) HasTOTP() bool { return len(u.TOTPSecret) > 0 }

// HasPendingTOTP dice si hay un secreto generado que todavía no confirmó.
func (u User) HasPendingTOTP() bool { return len(u.TOTPPending) > 0 }
