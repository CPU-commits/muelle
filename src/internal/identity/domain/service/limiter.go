package service

import (
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/package/clock"
)

// maxTrackedIPs: pasado este tamaño se borran las ventanas viejas, para que
// el mapa no crezca sin límite.
const maxTrackedIPs = 10000

// LoginLimiter cuenta intentos de login por IP en ventanas fijas
// (LoginMaxPerIP cada LoginWindow en producción). Es seguro usarlo desde
// varias goroutines.
type LoginLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	clock  clock.Clock
	hits   map[string]ipWindow
}

type ipWindow struct {
	start time.Time
	count int
}

func NewLoginLimiter(max int, window time.Duration, c clock.Clock) *LoginLimiter {
	return &LoginLimiter{max: max, window: window, clock: c, hits: map[string]ipWindow{}}
}

// Allow registra un intento desde ip y dice si está permitido.
func (l *LoginLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.clock.Now()
	w := l.hits[ip]
	if now.Sub(w.start) >= l.window {
		w = ipWindow{start: now}
	}
	if w.count >= l.max {
		l.hits[ip] = w
		return false
	}
	w.count++
	l.hits[ip] = w
	if len(l.hits) > maxTrackedIPs {
		for k, v := range l.hits {
			if now.Sub(v.start) >= l.window {
				delete(l.hits, k)
			}
		}
	}
	return true
}
