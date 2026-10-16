package usecases

import (
	"context"
	"time"
)

// Retrier reintenta la configuración de Caddy con backoff (1 s, 2 s… 60 s,
// D14 del plan de la Fase 3) después de un Sync que falló: el Syncer lo
// despierta con Kick (su onFail). Corre en ingress.Run.
type Retrier struct {
	kick   chan struct{}
	syncFn func(ctx context.Context) error
	lo, hi time.Duration
}

// NewRetrier recibe el Sync que reintenta y los plazos (Timing.RetryMin y RetryMax).
func NewRetrier(syncFn func(ctx context.Context) error, retryMin, retryMax time.Duration) *Retrier {
	return &Retrier{kick: make(chan struct{}, 1), syncFn: syncFn, lo: retryMin, hi: retryMax}
}

// Kick avisa que un Sync falló. No bloquea: varios avisos seguidos son uno.
func (r *Retrier) Kick() {
	select {
	case r.kick <- struct{}{}:
	default:
	}
}

// Run, hasta que ctx se cancela: al recibir un Kick espera el backoff y llama
// a syncFn; si falla, duplica la espera hasta retryMax y vuelve a llamar. Cuando
// funciona, descarta el Kick que dejaron sus propios fallos y vuelve a
// esperar uno nuevo.
func (r *Retrier) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.kick:
		}
		for d := nextBackoff(0, r.lo, r.hi); ; d = nextBackoff(d, r.lo, r.hi) {
			if !sleep(ctx, d) {
				return
			}
			if r.syncFn(ctx) == nil {
				break
			}
		}
		select {
		case <-r.kick:
		default:
		}
	}
}

// nextBackoff es la espera que sigue a d: lo la primera vez (d == 0) y
// después el doble, hasta hi.
func nextBackoff(d, lo, hi time.Duration) time.Duration {
	if d <= 0 {
		return lo
	}
	return min(2*d, hi)
}
