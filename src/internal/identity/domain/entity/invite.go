package entity

import "time"

// Invite es una invitación de un solo uso. Se guarda solo el hash del token.
// Sin creador (CreatedBy nil) es el link de setup que genera el sistema
// cuando todavía no hay usuarios.
type Invite struct {
	ID        int64
	TokenHash string
	CreatedBy *int64
	ExpiresAt time.Time
	UsedAt    *time.Time
}

// NewInvite arma una invitación que vence ttl después de now.
func NewInvite(tokenHash string, createdBy *int64, now time.Time, ttl time.Duration) Invite {
	return Invite{TokenHash: tokenHash, CreatedBy: createdBy, ExpiresAt: now.Add(ttl)}
}

// IsSystem dice si es un link de setup del sistema.
func (i Invite) IsSystem() bool { return i.CreatedBy == nil }

// Usable dice si en now la invitación todavía se puede aceptar.
func (i Invite) Usable(now time.Time) bool {
	return i.UsedAt == nil && now.Before(i.ExpiresAt)
}
