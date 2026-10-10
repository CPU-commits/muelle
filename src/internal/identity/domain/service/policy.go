package service

import (
	"fmt"
	"time"
)

// Duraciones de §10.1 del spec.
const (
	SessionIdle   = 12 * time.Hour     // una sesión sin uso se cierra
	SessionMax    = 7 * 24 * time.Hour // duración máxima de una sesión
	MFAPendingTTL = 5 * time.Minute    // sesión intermedia del login con 2FA
	InviteTTL     = 24 * time.Hour

	// TouchEvery: last_seen_at de las sesiones y last_used_at de los tokens se
	// actualizan como mucho una vez por minuto, para no escribir en cada request.
	TouchEvery = time.Minute
)

// Límites de login (§10.1 del spec).
const (
	LoginMaxPerIP = 20               // intentos por IP…
	LoginWindow   = 15 * time.Minute // …en esta ventana

	LockoutThreshold = 5           // fallos seguidos antes del primer bloqueo
	LockoutBase      = time.Minute // primer bloqueo; se duplica con cada fallo
	LockoutMax       = time.Hour   // tope del bloqueo
)

// MaxTokenDays es el vencimiento más lejano que se puede pedir para un token de API.
const MaxTokenDays = 3650

// LockoutFor devuelve cuánto se bloquea un usuario tras failures fallos
// seguidos: nada hasta 4, 1 min con 5, y se duplica con cada fallo hasta 1 h.
func LockoutFor(failures int) time.Duration {
	if failures < LockoutThreshold {
		return 0
	}
	d := LockoutBase
	for i := LockoutThreshold; i < failures && d < LockoutMax; i++ {
		d *= 2
	}
	return min(d, LockoutMax)
}

// TOTPSecretAAD es la AAD del secreto TOTP activo (users.totp_secret).
func TOTPSecretAAD(userID int64) string { return fmt.Sprintf("users:totp:%d", userID) }

// TOTPPendingAAD es la AAD del secreto TOTP pendiente (users.totp_pending).
// Es distinta de la del activo: activar el 2FA descifra el pendiente y lo
// vuelve a cifrar, así un valor pendiente nunca descifra como activo.
func TOTPPendingAAD(userID int64) string { return fmt.Sprintf("users:totp_pending:%d", userID) }
