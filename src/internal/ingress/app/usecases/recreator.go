package usecases

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Recreator recrea muelle-caddy cuando cambian los puertos
// (contrato §3.5): delay después del último Schedule (2 s), y nunca mientras
// un hook lo frena (Hold, D15 del plan de la Fase 3). El PATCH /settings que
// cambia los puertos sale por ese mismo contenedor: recrearlo antes de que
// el hook vuelva cortaría su respuesta. Corre en ingress.Run.
type Recreator struct {
	ch     chan struct{}
	mu     sync.Mutex
	holds  int
	state  *ProxyState
	caddy  *EnsureCaddy
	syncer *Syncer
	delay  time.Duration
	lo, hi time.Duration
	log    *slog.Logger
}

// NewRecreator recibe el delay de la recreación (Timing.RecreateDelay) y el
// backoff con que espera al Caddy nuevo (Timing.RetryMin y RetryMax).
func NewRecreator(caddy *EnsureCaddy, syncer *Syncer, state *ProxyState, delay, retryMin, retryMax time.Duration, log *slog.Logger) *Recreator {
	return &Recreator{ch: make(chan struct{}, 1), state: state, caddy: caddy, syncer: syncer,
		delay: delay, lo: retryMin, hi: retryMax, log: log}
}

// Schedule agenda la recreación (GET /proxy dice recreating desde ya) y no
// espera. Varias seguidas se juntan en una.
func (r *Recreator) Schedule() {
	r.state.BeginRecreate()
	select {
	case r.ch <- struct{}{}:
	default:
	}
}

// Hold frena la recreación hasta que se llame a release, y la marca como
// pendiente (GET /proxy dice recreating). release la agenda (Schedule): así
// el delay cuenta desde que vuelve el hook, no desde que empezó. release es
// idempotente.
func (r *Recreator) Hold() (release func()) {
	r.mu.Lock()
	r.holds++
	r.mu.Unlock()
	r.state.BeginRecreate()
	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			r.holds--
			r.mu.Unlock()
			r.Schedule()
		})
	}
}

func (r *Recreator) held() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.holds > 0
}

// Run, hasta que ctx se cancela: espera un Schedule; después espera delay, y
// vuelve a esperarlo mientras lleguen más. Si al vencer hay un Hold sin
// soltar, vuelve a esperar un Schedule (el que manda el release). Si no,
// toma la generación actual, recrea (EnsureCaddy, que lee los ajustes de ese
// momento) y le manda la configuración, y termina con EndRecreate(gen): un
// Schedule que llegó mientras recreaba sigue pendiente.
func (r *Recreator) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.ch:
		}
	debounce:
		for {
			t := time.NewTimer(r.delay)
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-r.ch:
				t.Stop()
			case <-t.C:
				break debounce
			}
		}
		if r.held() {
			continue
		}
		gen := r.state.Generation()
		r.recreate(ctx)
		r.state.EndRecreate(gen)
	}
}

// recreate asegura muelle-caddy (Recreate: si no arranca con los puertos
// nuevos, vuelve a los anteriores) y le manda la configuración; si lo creó
// de nuevo, espera a que su admin API responda (syncStarting). Los errores
// ya quedaron en el estado y en el log.
func (r *Recreator) recreate(ctx context.Context) {
	recreated, err := r.caddy.Recreate(ctx)
	if err != nil {
		r.log.Warn("ingress: la recreación de muelle-caddy falló", "error", err)
	}
	if recreated {
		_ = syncStarting(ctx, r.syncer, r.lo, r.hi)
		return
	}
	_ = r.syncer.Sync(ctx, false)
}
