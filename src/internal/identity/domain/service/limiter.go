package service

import (
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/package/clock"
)

// maxTrackedIPs: la IP sale de X-Real-IP, así que puede ser cualquier texto;
// el mapa tiene un tamaño máximo (y ipKey un largo máximo) para que no se
// pueda llenar la memoria con IPs inventadas.
const maxTrackedIPs = 10000

// LoginLimiter cuenta intentos de login por IP en ventanas fijas
// (LoginMaxPerIP cada LoginWindow en producción). Una IPv6 cuenta por su /64
// (ipKey). Es seguro usarlo desde varias goroutines.
type LoginLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	clock  clock.Clock
	maxIPs int
	hits   map[string]ipWindow
}

type ipWindow struct {
	start time.Time
	count int
}

func NewLoginLimiter(max int, window time.Duration, c clock.Clock) *LoginLimiter {
	return &LoginLimiter{max: max, window: window, clock: c, maxIPs: maxTrackedIPs, hits: map[string]ipWindow{}}
}

// Allow registra un intento desde ip y dice si está permitido.
func (l *LoginLimiter) Allow(ip string) bool {
	ip = ipKey(ip)
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock.Now()
	w, known := l.hits[ip]
	if !known && len(l.hits) >= l.maxIPs {
		l.makeRoom(now)
	}
	if now.Sub(w.start) >= l.window {
		w = ipWindow{start: now}
	}
	if w.count >= l.max {
		l.hits[ip] = w
		return false
	}
	w.count++
	l.hits[ip] = w
	return true
}

// makeRoom se llama con el mapa lleno: borra las ventanas vencidas y, si no
// alcanza, desaloja ventanas vigentes hasta dejar un cuarto libre, así el
// barrido no se repite en cada IP nueva. Desalojar solo puede devolverle
// intentos a una IP; el bloqueo por usuario sigue protegiendo las cuentas.
func (l *LoginLimiter) makeRoom(now time.Time) {
	for k, v := range l.hits {
		if now.Sub(v.start) >= l.window {
			delete(l.hits, k)
		}
	}
	target := l.maxIPs * 3 / 4
	for k := range l.hits {
		if len(l.hits) <= target {
			break
		}
		delete(l.hits, k)
	}
}
