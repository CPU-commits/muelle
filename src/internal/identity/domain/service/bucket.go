package service

import (
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/package/clock"
)

// Límite global de los eventos de auditoría que puede provocar cualquiera sin
// autenticarse (auth.login_failed, auth.locked): sin él, muchas IPs podrían
// llenar el disco con filas de auditoría.
const (
	AuditEventsPerSecond = 20
	AuditBurst           = 200
)

// TokenBucket es un balde de tokens: permite una ráfaga de burst y después
// rate por segundo. Es seguro usarlo desde varias goroutines.
type TokenBucket struct {
	mu     sync.Mutex
	clock  clock.Clock
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
}

// NewTokenBucket arma un balde lleno.
func NewTokenBucket(perSecond float64, burst int, c clock.Clock) *TokenBucket {
	return &TokenBucket{clock: c, rate: perSecond, burst: float64(burst), tokens: float64(burst), last: c.Now()}
}

// Allow gasta un token si hay, y dice si lo hubo.
func (b *TokenBucket) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.clock.Now()
	if elapsed := now.Sub(b.last); elapsed > 0 { // un reloj que retrocede no recarga ni corre last
		b.tokens = min(b.burst, b.tokens+elapsed.Seconds()*b.rate)
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
