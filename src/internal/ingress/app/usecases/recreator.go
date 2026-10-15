package usecases

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
)

// recreateSyncWindow es cuánto se reintenta mandarle la configuración al
// muelle-caddy recién creado mientras su admin API todavía no responde.
const recreateSyncWindow = 30 * time.Second

// Recreator recrea muelle-caddy cuando cambian los puertos o la imagen
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

// recreate asegura muelle-caddy y le manda la configuración. Si lo creó de
// nuevo, Caddy tarda un momento en abrir la admin API: reintenta el Sync con
// backoff hasta recreateSyncWindow, así GET /proxy pasa de recreating a
// synced sin un out_of_sync de paso. Los errores ya quedaron en el estado y
// en el log.
func (r *Recreator) recreate(ctx context.Context) {
	recreated, err := r.caddy.Execute(ctx)
	if err != nil {
		r.log.Warn("ingress: la recreación de muelle-caddy falló", "error", err)
	}
	deadline := time.Now().Add(recreateSyncWindow)
	for d := time.Duration(0); ; {
		err := r.syncer.Sync(ctx, false)
		if err == nil || !recreated || !errors.Is(err, service.ErrCaddyUnreachable) || time.Now().After(deadline) {
			return
		}
		d = nextBackoff(d, r.lo, r.hi)
		if !sleep(ctx, d) {
			return
		}
	}
}
