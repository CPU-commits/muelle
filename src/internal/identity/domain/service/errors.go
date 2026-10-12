// Package service tiene las reglas de dominio de identity (sesiones, bloqueo
// progresivo, límite por IP, tokens aleatorios) y sus puertos: el hasher de
// contraseñas, el TOTP y lo que identity necesita de otros módulos.
package service

import "errors"

// Errores de identity. infra/in/http los traduce a HTTP.
var (
	ErrInvalidCredentials = errors.New("identity: usuario o contraseña incorrectos")
	ErrLocked             = errors.New("identity: usuario bloqueado temporalmente")
	ErrRateLimited        = errors.New("identity: demasiados intentos desde esta IP")
	ErrInvalidCode        = errors.New("identity: código TOTP inválido")
	ErrNoPendingTOTP      = errors.New("identity: no hay configuración de 2FA pendiente")
	ErrInviteInvalid      = errors.New("identity: invitación inválida, usada o vencida")
	ErrAlreadySetUp       = errors.New("identity: muelle ya tiene usuarios")
	ErrCannotDeleteSelf   = errors.New("identity: no puedes borrar tu propio usuario")
)
