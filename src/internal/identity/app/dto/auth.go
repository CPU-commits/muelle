// Package dto tiene las entradas y salidas de los casos de uso de identity.
package dto

import "time"

type LoginInput struct {
	Username  string
	Password  string
	IP        string
	UserAgent string
}

// SessionOutput es una sesión recién abierta. Token va en claro: es el valor
// de la cookie y no se vuelve a poder leer.
type SessionOutput struct {
	Token       string
	UserID      int64
	Username    string
	ExpiresAt   time.Time
	MFARequired bool // sesión intermedia: falta el código TOTP
}

// CompleteTOTPInput cambia una sesión pendiente de MFA por una completa.
// SessionHash es el de la sesión pendiente (Principal.SessionHash).
type CompleteTOTPInput struct {
	SessionHash string
	Code        string
	IP          string
	UserAgent   string
}

// AcceptInviteInput crea un usuario con una invitación (o con el link de
// setup) y le abre una sesión.
type AcceptInviteInput struct {
	Token     string
	Username  string
	Password  string
	IP        string
	UserAgent string
}

type MeOutput struct {
	Username       string
	TOTPEnabled    bool
	NeedsTOTPSetup bool
}

// TOTPSetupOutput es el secreto TOTP recién generado, para mostrarlo como QR.
type TOTPSetupOutput struct {
	Secret string
	URL    string
}

type EnableTOTPInput struct {
	UserID int64
	Code   string
}
