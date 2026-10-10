package dto

import "time"

type UserOutput struct {
	Username    string
	TOTPEnabled bool
	CreatedAt   time.Time
}

type DeleteUserInput struct {
	Username      string // a quién se borra
	ActorUsername string // quién lo pide: no puede borrarse a sí mismo
}

type CreateInviteInput struct {
	CreatedBy       int64
	CreatorUsername string
}

// InviteOutput es una invitación recién creada. Token va en claro: se
// muestra una sola vez.
type InviteOutput struct {
	Token     string
	ExpiresAt time.Time
}
