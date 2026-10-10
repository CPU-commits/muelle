package service

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/package/clock"
)

const (
	// maxLockoutKeys acota cada mapa de bloqueo, como el del límite por IP.
	maxLockoutKeys = 10000
	// maxUsernameKey es el largo máximo del usuario dentro de una clave: el
	// nombre del login puede ser cualquier texto (exista o no).
	maxUsernameKey = 64
	// lockoutForget: una clave sin bloqueo vigente y sin fallos nuevos por
	// este tiempo se olvida cuando hace falta lugar en el mapa.
	lockoutForget = 24 * time.Hour
)

// LoginLockout es el bloqueo progresivo del paso de la contraseña (§10.1 del
// spec), en memoria, por usuario e IP: la clave es el nombre en minúsculas
// más la IPv4, o el /64 de la IPv6 (ipKey). Así un atacante solo se bloquea a
// sí mismo y no puede dejar afuera al admin desde otra IP (OWASP). Se aplica
// igual a nombres que no existen, para que un bloqueo no revele qué usuarios
// hay. El calendario es LockoutFor. Es seguro usarlo desde varias goroutines.
type LoginLockout struct{ l *progressiveLock }

func NewLoginLockout(c clock.Clock) *LoginLockout { return &LoginLockout{newProgressiveLock(c)} }

func lockoutKey(username, ip string) string {
	u := strings.ToLower(username)
	if len(u) > maxUsernameKey {
		u = u[:maxUsernameKey]
	}
	return ipKey(ip) + "\x00" + u
}

// Locked dice si el par (usuario, IP) está bloqueado ahora.
func (l *LoginLockout) Locked(username, ip string) bool { return l.l.locked(lockoutKey(username, ip)) }

// Fail registra una contraseña incorrecta. Si el par ya estaba bloqueado no
// cambia nada y devuelve wasLocked: un intento que esperó al hasher mientras
// otro bloqueaba el par responde "bloqueado", igual que si hubiera acertado.
// locked dice si el par quedó bloqueado con este fallo.
func (l *LoginLockout) Fail(username, ip string) (wasLocked, locked bool) {
	return l.l.fail(lockoutKey(username, ip))
}

// Succeed decide una contraseña correcta: si el par está bloqueado devuelve
// false sin cambiar nada (el que esperó al hasher responde "bloqueado"); si
// no, olvida sus fallos y devuelve true.
func (l *LoginLockout) Succeed(username, ip string) bool {
	return l.l.succeed(lockoutKey(username, ip))
}

// TOTPLockout es el bloqueo progresivo del paso del código TOTP, en memoria y
// por usuario, desde cualquier IP: a este paso solo llega quien sabe la
// contraseña, así que contar por IP no frena a un atacante con muchas IPs.
// Mismo calendario que LoginLockout; solo un código correcto lo reinicia.
type TOTPLockout struct{ l *progressiveLock }

func NewTOTPLockout(c clock.Clock) *TOTPLockout { return &TOTPLockout{newProgressiveLock(c)} }

func totpKey(userID int64) string { return strconv.FormatInt(userID, 10) }

// Locked dice si el 2FA del usuario está bloqueado ahora.
func (l *TOTPLockout) Locked(userID int64) bool { return l.l.locked(totpKey(userID)) }

// Fail registra un código incorrecto; mismo contrato que LoginLockout.Fail.
func (l *TOTPLockout) Fail(userID int64) (wasLocked, locked bool) { return l.l.fail(totpKey(userID)) }

// Succeed decide un código correcto; mismo contrato que LoginLockout.Succeed.
func (l *TOTPLockout) Succeed(userID int64) bool { return l.l.succeed(totpKey(userID)) }

// progressiveLock cuenta fallos por clave y bloquea según LockoutFor, con un
// mapa acotado. Mientras una clave está bloqueada, ni un fallo ni un acierto
// cambian nada.
type progressiveLock struct {
	mu      sync.Mutex
	clock   clock.Clock
	maxKeys int
	entries map[string]lockEntry
}

type lockEntry struct {
	failures    int
	lockedUntil time.Time
	lastFailure time.Time
}

func newProgressiveLock(c clock.Clock) *progressiveLock {
	return &progressiveLock{clock: c, maxKeys: maxLockoutKeys, entries: map[string]lockEntry{}}
}

func (l *progressiveLock) locked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.clock.Now().Before(l.entries[key].lockedUntil)
}

func (l *progressiveLock) fail(key string) (wasLocked, locked bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock.Now()
	e, known := l.entries[key]
	if now.Before(e.lockedUntil) {
		return true, true
	}
	if !known && len(l.entries) >= l.maxKeys {
		l.makeRoom(now)
	}
	e.failures++
	e.lastFailure = now
	if d := LockoutFor(e.failures); d > 0 {
		e.lockedUntil = now.Add(d)
		locked = true
	}
	l.entries[key] = e
	return false, locked
}

func (l *progressiveLock) succeed(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.clock.Now().Before(l.entries[key].lockedUntil) {
		return false
	}
	delete(l.entries, key)
	return true
}

// makeRoom se llama con el mapa lleno. Primero olvida las claves viejas (sin
// bloqueo vigente ni fallos recientes); si no alcanza, desaloja claves que no
// están bloqueadas y, en último caso, cualquiera, hasta dejar un cuarto libre
// para que el barrido no se repita en cada clave nueva.
func (l *progressiveLock) makeRoom(now time.Time) {
	for k, e := range l.entries {
		if !now.Before(e.lockedUntil) && now.Sub(e.lastFailure) >= lockoutForget {
			delete(l.entries, k)
		}
	}
	target := l.maxKeys * 3 / 4
	for k, e := range l.entries {
		if len(l.entries) <= target {
			return
		}
		if !now.Before(e.lockedUntil) {
			delete(l.entries, k)
		}
	}
	for k := range l.entries {
		if len(l.entries) <= target {
			return
		}
		delete(l.entries, k)
	}
}
