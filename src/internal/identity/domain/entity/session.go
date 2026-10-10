package entity

import "time"

// MaxUserAgent es el largo máximo (en bytes) del user agent que se guarda.
const MaxUserAgent = 256

// Session es una sesión del panel. Solo se guarda el hash del token.
// MFAPending marca la sesión intermedia del login con 2FA: la contraseña
// fue correcta y falta el código TOTP.
type Session struct {
	IDHash     string
	UserID     int64
	MFAPending bool
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	UserAgent  string
	IP         string
}

// NewSession arma una sesión que vence ttl después de now. El user agent se
// recorta a MaxUserAgent bytes sin cortar un carácter por la mitad.
func NewSession(idHash string, userID int64, mfaPending bool, now time.Time, ttl time.Duration, ip, userAgent string) Session {
	return Session{
		IDHash: idHash, UserID: userID, MFAPending: mfaPending,
		CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(ttl),
		UserAgent: truncate(userAgent, MaxUserAgent), IP: ip,
	}
}

// Expired dice si la sesión ya no sirve en now: llegó a su vencimiento o,
// si es una sesión completa, lleva más de idle sin uso. Una sesión pendiente
// de MFA solo vence por su ExpiresAt.
func (s Session) Expired(now time.Time, idle time.Duration) bool {
	return !now.Before(s.ExpiresAt) || (!s.MFAPending && now.Sub(s.LastSeenAt) > idle)
}

// truncate corta s a lo más max bytes, en el límite de un carácter.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := 0
	for i := range s { // i recorre el inicio de cada carácter
		if i > max {
			break
		}
		cut = i
	}
	return s[:cut]
}
