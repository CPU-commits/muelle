package usecases

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
)

// defaultStopTimeout es cuánto espera Stop a que el worker de una app
// termine (contrato §1.1).
const defaultStopTimeout = 15 * time.Second

// Dispatcher es la cola por app (§7 paso 1, contrato §1.2): un worker por
// app con trabajo, un deploy a la vez por app, apps distintas en paralelo.
// Implementa service.Queue.
//
// Mientras una app está bloqueada (Block, la usa RemoveApp: decisión 8 del
// plan), ningún worker reclama ni retoma sus deploys: los queued que lleguen
// quedan failed "la app se borró". La mirada a blocked va bajo el mismo mu
// que Block, antes de cada Claim y de cada Resume.
//
// Qué registra en slog (decisión 18): "deploy cerrado" con app, deploy,
// status y, si falló, el error ya guardado (enmascarado); un error interno,
// solo "deploy: error interno" con app y deploy. Nunca el texto de un error
// del worker ni la salida del CLI: pueden traer valores de variables.
type Dispatcher struct {
	run         *RunDeploy
	log         *slog.Logger
	stopTimeout time.Duration

	mu       sync.Mutex
	ctx      context.Context // el de Start
	started  bool
	stopping bool                // Wait ya empezó: no se arranca nada
	pending  map[int64]bool      // los Wake antes de Start
	workers  map[int64]*worker   // por app
	blocked  map[int64]time.Time // app → hasta cuándo
	wg       sync.WaitGroup
}

type worker struct {
	cancel  context.CancelCauseFunc
	done    chan struct{}
	woken   bool // llegó un Wake mientras trabajaba
	stopped bool // Stop lo canceló
}

var _ service.Queue = (*Dispatcher)(nil)

// NewDispatcher arma el despachador de run y lo fija como su Gate.
func NewDispatcher(run *RunDeploy, log *slog.Logger) *Dispatcher {
	d := &Dispatcher{
		run: run, log: log, stopTimeout: defaultStopTimeout,
		pending: map[int64]bool{}, workers: map[int64]*worker{}, blocked: map[int64]time.Time{},
	}
	run.Gate = d
	return d
}

// Start arranca un worker por cada app de apps (las que tienen un deploy
// queued o running, ActiveApps) y por cada Wake anterior. Los workers corren
// hasta que ctx se cancela; después hay que llamar a Wait.
func (d *Dispatcher) Start(ctx context.Context, apps []int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started {
		return
	}
	d.started, d.ctx = true, ctx
	for _, id := range apps {
		d.pending[id] = true
	}
	for _, id := range slices.Sorted(maps.Keys(d.pending)) {
		d.wakeLocked(id)
	}
	d.pending = nil
}

// Wake avisa que la app tiene un deploy queued nuevo. Antes de Start solo lo
// anota. Con la app bloqueada no arranca un worker: marca failed sus queued
// ("la app se borró", decisión 8) y avisa a los streams, en una goroutine.
func (d *Dispatcher) Wake(appID int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.started {
		d.pending[appID] = true
		return
	}
	d.wakeLocked(appID)
}

func (d *Dispatcher) wakeLocked(appID int64) {
	if d.stopping || d.ctx.Err() != nil {
		return
	}
	if w := d.workers[appID]; w != nil {
		w.woken = true
		return
	}
	if d.blockedLocked(appID) {
		ctx := d.ctx
		d.wg.Go(func() { d.failQueued(ctx, appID) })
		return
	}
	d.spawnLocked(appID)
}

func (d *Dispatcher) spawnLocked(appID int64) {
	ctx, cancel := context.WithCancelCause(d.ctx)
	w := &worker{cancel: cancel, done: make(chan struct{})}
	d.workers[appID] = w
	d.wg.Go(func() { d.work(ctx, appID, w) })
}

// Accepting es false mientras la app está bloqueada.
func (d *Dispatcher) Accepting(appID int64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return !d.blockedLocked(appID)
}

func (d *Dispatcher) blockedLocked(appID int64) bool {
	until, ok := d.blocked[appID]
	if !ok {
		return false
	}
	if !d.run.Clock.Now().Before(until) {
		delete(d.blocked, appID)
		return false
	}
	return true
}

func (d *Dispatcher) isBlocked(appID int64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.blockedLocked(appID)
}

// Block deja de aceptar deploys de la app por dur (RemoveApp: 2 min, lo que
// tarda catalog en borrar la fila).
func (d *Dispatcher) Block(appID int64, dur time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.blocked[appID] = d.run.Clock.Now().Add(dur)
}

// Unblock vuelve a aceptar deploys de la app (el borrado falló).
func (d *Dispatcher) Unblock(appID int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.blocked, appID)
}

// Stop cancela el worker de la app con la causa service.ErrAppRemoved y
// espera a que termine, a lo más 15 s: si no terminó, devuelve
// service.ErrWorkerBusy (el worker sigue cancelado, cierra solo y sale del
// mapa al terminar). Sin worker, vuelve enseguida.
func (d *Dispatcher) Stop(ctx context.Context, appID int64) error {
	d.mu.Lock()
	w := d.workers[appID]
	if w == nil {
		d.mu.Unlock()
		return nil
	}
	w.stopped = true
	w.cancel(service.ErrAppRemoved)
	d.mu.Unlock()
	t := time.NewTimer(d.stopTimeout)
	defer t.Stop()
	select {
	case <-w.done:
		return nil
	case <-t.C:
		return service.ErrWorkerBusy
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Wait espera a que terminen todos los workers. Se llama después de cancelar
// el ctx de Start; desde ahí no se arranca ninguno.
func (d *Dispatcher) Wait() {
	d.mu.Lock()
	d.stopping = true
	d.mu.Unlock()
	d.wg.Wait()
}

// work es el worker de una app: retoma su running (Resume) o reclama su
// queued más viejo y lo corre, uno por vez, hasta que no queda nada y no
// llegó un Wake. Con la app bloqueada, marca failed sus queued y termina.
func (d *Dispatcher) work(ctx context.Context, appID int64, w *worker) {
	defer func() {
		w.cancel(nil)
		d.mu.Lock()
		if d.workers[appID] == w {
			delete(d.workers, appID)
		}
		// Stop lo canceló pero RemoveApp no siguió (ErrWorkerBusy): la app
		// no se borró y puede tener queued esperando.
		if w.stopped && !d.blockedLocked(appID) && !d.stopping && d.ctx.Err() == nil {
			d.spawnLocked(appID)
		}
		d.mu.Unlock()
		close(w.done)
	}()
	for ctx.Err() == nil {
		d.mu.Lock()
		w.woken = false
		blocked := d.blockedLocked(appID)
		d.mu.Unlock()
		if blocked {
			d.failQueued(ctx, appID)
			return
		}
		if d.next(ctx, appID) {
			continue
		}
		d.mu.Lock()
		if w.woken && ctx.Err() == nil {
			d.mu.Unlock()
			continue
		}
		if d.workers[appID] == w {
			delete(d.workers, appID)
		}
		d.mu.Unlock()
		return
	}
}

// next hace una unidad de trabajo de la app y dice si hay que volver a mirar.
func (d *Dispatcher) next(ctx context.Context, appID int64) bool {
	deploys := d.run.Deploys
	running, err := deploys.Running(ctx, appID)
	if err != nil {
		return d.internal(ctx, appID, 0)
	}
	if running != nil {
		if d.isBlocked(appID) {
			return true // la vuelta siguiente lo ve y termina
		}
		err := d.run.Resume(ctx, *running)
		d.after(ctx, appID, running.ID, err)
		return true
	}
	q, err := deploys.NextQueued(ctx, appID)
	if err != nil {
		return d.internal(ctx, appID, 0)
	}
	if q == nil {
		return false
	}
	if d.isBlocked(appID) {
		return true
	}
	at := d.run.Clock.Now()
	ok, err := deploys.Claim(ctx, q.ID, at)
	if err != nil {
		return d.internal(ctx, appID, q.ID)
	}
	if !ok {
		return true // lo reemplazó otro POST entre NextQueued y Claim
	}
	d.run.Hub.Notify(q.ID)
	q.Status, q.StartedAt = entity.StatusRunning, &at
	err = d.run.Execute(ctx, *q)
	d.after(ctx, appID, q.ID, err)
	return true
}

// after registra cómo terminó un deploy y aplica la retención (contrato
// §2.3). Si quedó running por un error interno, espera RetryEvery antes de
// que el worker lo retome; si quedó running por el apagado, nada.
func (d *Dispatcher) after(ctx context.Context, appID, id int64, err error) {
	if err != nil && ctx.Err() != nil && !errors.Is(context.Cause(ctx), service.ErrAppRemoved) {
		return
	}
	if err != nil {
		d.log.Error("deploy: error interno", "app", appID, "deploy", id)
	}
	bg := context.WithoutCancel(ctx)
	dep, gerr := d.run.Deploys.GetByID(bg, id)
	if gerr != nil {
		d.internal(ctx, appID, id)
		return
	}
	if !dep.Status.Final() {
		d.sleep(ctx, d.run.Timing.RetryEvery)
		return
	}
	d.closed(appID, dep)
	if _, err := d.run.Deploys.ApplyRetention(bg, appID); err != nil {
		d.log.Error("deploy: error interno", "app", appID, "deploy", id)
	}
}

// closed registra un deploy que cerró.
func (d *Dispatcher) closed(appID int64, dep entity.Deploy) {
	attrs := []any{"app", appID, "deploy", dep.ID, "status", string(dep.Status)}
	if dep.Error != nil {
		attrs = append(attrs, "error", *dep.Error)
	}
	d.log.Info("deploy cerrado", attrs...)
}

// internal registra un error de la base del worker (sin su texto) y espera
// RetryEvery antes de volver a intentar.
func (d *Dispatcher) internal(ctx context.Context, appID, id int64) bool {
	if ctx.Err() != nil {
		return true
	}
	if id != 0 {
		d.log.Error("deploy: error interno", "app", appID, "deploy", id)
	} else {
		d.log.Error("deploy: error interno", "app", appID)
	}
	d.sleep(ctx, d.run.Timing.RetryEvery)
	return true
}

func (d *Dispatcher) sleep(ctx context.Context, dur time.Duration) {
	t := time.NewTimer(dur)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// failQueued marca failed los queued de una app bloqueada y avisa a sus
// streams.
func (d *Dispatcher) failQueued(ctx context.Context, appID int64) {
	ids, err := d.run.Deploys.FailQueued(context.WithoutCancel(ctx), appID, reasonQueuedRemoved, d.run.Clock.Now())
	if err != nil {
		d.log.Error("deploy: error interno", "app", appID)
		return
	}
	for _, id := range ids {
		d.run.Hub.Notify(id)
		d.log.Info("deploy cerrado", "app", appID, "deploy", id, "status", string(entity.StatusFailed), "error", reasonQueuedRemoved)
	}
}
