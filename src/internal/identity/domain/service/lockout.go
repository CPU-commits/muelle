package service

import (
	"strings"
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/package/clock"
)

const (
	// maxLockoutKeys acota el mapa del bloqueo, como el del límite por IP.
	maxLockoutKeys = 10000
	// maxUsernameKey es el largo máximo del usuario dentro de una clave: el
	// nombre del login puede ser cualquier texto (exista o no).
	maxUsernameKey = 64
	// lockoutForget: un par sin bloqueo vigente y sin fallos nuevos por este
	// tiempo se olvida cuando hace falta lugar en el mapa.
	lockoutForget = 24 * time.Hour
)

// LoginLockout es el bloqueo progresivo de login (§10.1 del spec), en
// memoria, por usuario e IP: la clave es el nombre en minúsculas más la IPv4,
// o el /64 de la IPv6 (ipKey). Así un atacante solo se bloquea a sí mismo y
// no puede dejar afuera al admin desde otra IP (OWASP). Se aplica igual a
// nombres que no existen, para que un bloqueo no revele qué usuarios hay. El
// calendario es LockoutFor. Es seguro usarlo desde varias goroutines.
type LoginLockout struct {
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

func NewLoginLockout(c clock.Clock) *LoginLockout {
	return &LoginLockout{clock: c, maxKeys: maxLockoutKeys, entries: map[string]lockEntry{}}
}

func lockoutKey(username, ip string) string {
	u := strings.ToLower(username)
	if len(u) > maxUsernameKey {
		u = u[:maxUsernameKey]
	}
	return ipKey(ip) + "\x00" + u
}

// Locked dice si el par (usuario, IP) está bloqueado ahora.
func (l *LoginLockout) Locked(username, ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.clock.Now().Before(l.entries[lockoutKey(username, ip)].lockedUntil)
}

// Fail registra un intento fallido. Si el par ya estaba bloqueado no cambia
// nada y devuelve true: un intento que esperó al hasher mientras otro
// bloqueaba el par responde "bloqueado", igual que si hubiera acertado.
func (l *LoginLockout) Fail(username, ip string) (wasLocked bool) {
	key := lockoutKey(username, ip)
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock.Now()
	e, known := l.entries[key]
	if now.Before(e.lockedUntil) {
		return true
	}
	if !known && len(l.entries) >= l.maxKeys {
		l.makeRoom(now)
	}
	e.failures++
	e.lastFailure = now
	if d := LockoutFor(e.failures); d > 0 {
		e.lockedUntil = now.Add(d)
	}
	l.entries[key] = e
	return false
}

// Succeed decide un intento correcto: si el par está bloqueado devuelve false
// sin cambiar nada (el que esperó al hasher responde "bloqueado"). Si no,
// devuelve true y, con reset, olvida los fallos. Con 2FA se llama sin reset
// al validar la contraseña y con reset recién al validar el código.
func (l *LoginLockout) Succeed(username, ip string, reset bool) bool {
	key := lockoutKey(username, ip)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.clock.Now().Before(l.entries[key].lockedUntil) {
		return false
	}
	if reset {
		delete(l.entries, key)
	}
	return true
}

// makeRoom se llama con el mapa lleno. Primero olvida los pares viejos (sin
// bloqueo vigente ni fallos recientes); si no alcanza, desaloja pares que no
// están bloqueados y, en último caso, cualquiera, hasta dejar un cuarto libre
// para que el barrido no se repita en cada clave nueva.
func (l *LoginLockout) makeRoom(now time.Time) {
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
