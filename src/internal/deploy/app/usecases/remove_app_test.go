package usecases

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	outservice "github.com/CPU-commits/muelle/src/internal/deploy/infra/out/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

func (w *world) removeApp(d *Dispatcher) *RemoveApp {
	return NewRemoveApp(w.apps, w.deploys, outservice.NewDockerStacks(w.docker), d, w.hub, clock.System{})
}

// Borrar una app detiene su worker (contrato §1.1): el deploy en curso (acá,
// retenido en el paso 6, donde el ctx sí llega) cierra failed "la app se
// borró durante el deploy", el queued queda failed, y la app no acepta
// deploys mientras se borra. Si el stack rm falla, vuelve a aceptar enseguida.
func TestRemoveAppStopsWorker(t *testing.T) {
	w := newWorld(t)
	exec(t, w.db, `UPDATE apps SET deploy_timeout_s = 60`)
	w.sim.set(simOpts{Running: new(uint64(0))})
	d, _ := w.startDispatcher(t, w.run)
	first := w.deploy(t, apiRef, nil, nil)
	waitFor(t, "esperando convergencia", func() bool { return strings.Contains(w.log(t, first), "esperando convergencia") })
	queued := w.deploy(t, apiRef, nil, nil)
	waitQueued := w.hub.Wait(queued)

	if err := w.removeApp(d).Execute(context.Background(), apiRef); err != nil {
		t.Fatal(err)
	}
	got := w.get(t, first)
	if got.Status != entity.StatusFailed || *got.Error != "la app se borró durante el deploy" {
		t.Fatalf("el que corría: %s %v\n%s", got.Status, got.Error, w.log(t, first))
	}
	if got := w.get(t, queued); got.Status != entity.StatusFailed || *got.Error != "la app se borró" {
		t.Fatalf("el queued: %s %v", got.Status, got.Error)
	}
	select {
	case <-waitQueued:
	default:
		t.Fatal("no avisó a los streams del queued")
	}
	if !slices.Equal(w.docker.RemovedStacks(), []string{"auditfy_staging_api"}) {
		t.Fatalf("stack rm: %v", w.docker.RemovedStacks())
	}
	if _, err := NewCreateDeploy(w.deps).Execute(tokenCtx(), dto.CreateDeploy{AppRef: apiRef}); !errors.Is(err, service.ErrAppRemoving) {
		t.Fatalf("POST …/deploys mientras se borra: %v", err)
	}
	if _, err := NewRollback(w.deps).Execute(tokenCtx(), apiRef); !errors.Is(err, service.ErrAppRemoving) {
		t.Fatalf("POST …/rollback mientras se borra: %v", err)
	}
	// Las otras apps siguen.
	if !d.Accepting(w.infraID) {
		t.Fatal("bloqueó otra app")
	}

	// Si el stack rm falla, la app vuelve a aceptar deploys enseguida.
	d.Unblock(w.apiID)
	w.docker.FailNext("RemoveStack", fmt.Errorf("caído: %w", docker.ErrUnavailable))
	if err := w.removeApp(d).Execute(context.Background(), apiRef); err == nil {
		t.Fatal("el stack rm falló")
	}
	if !d.Accepting(w.apiID) {
		t.Fatal("no volvió a aceptar deploys")
	}
	w.sim.set(simOpts{})
	id := w.deploy(t, apiRef, nil, nil)
	w.waitStatus(t, id, entity.StatusSucceeded)
}

// El orden que dejaría un stack huérfano sin el bloqueo en el despachador
// (decisión 8): un CreateDeploy que pasó Accepting antes del Block inserta su
// queued después de que RemoveApp terminó. El worker nunca lo reclama: queda
// failed y no hay ningún docker stack deploy después del stack rm.
func TestRemoveAppRaceWithCreate(t *testing.T) {
	w := newWorld(t)
	d, _ := w.startDispatcher(t, w.run)
	ctx := context.Background()
	app, err := w.deps.acceptingApp(ctx, apiRef) // la primera mitad de CreateDeploy
	if err != nil {
		t.Fatal(err)
	}
	if err := w.removeApp(d).Execute(ctx, apiRef); err != nil {
		t.Fatal(err)
	}
	a, err := actorOf(tokenCtx())
	if err != nil {
		t.Fatal(err)
	}
	q, err := w.deps.enqueue(ctx, app, a, func(context.Context) (map[string]string, error) { return nil, nil }) // la segunda
	if err != nil {
		t.Fatal(err)
	}
	got := w.waitStatus(t, q.ID, entity.StatusFailed)
	if *got.Error != "la app se borró" || got.StartedAt != nil {
		t.Fatalf("deploy: %+v", got)
	}
	time.Sleep(50 * time.Millisecond)
	calls := w.docker.Calls()
	rm := slices.Index(calls, "RemoveStack auditfy_staging_api")
	if rm < 0 || slices.ContainsFunc(calls[rm:], func(c string) bool { return strings.HasPrefix(c, "DeployStack") }) {
		t.Fatalf("llamadas: %v", calls)
	}
}

// Si el worker no se detiene en el plazo de Stop (retenido donde el ctx no
// llega), RemoveApp no sigue: ErrWorkerBusy, la app vuelve a aceptar
// deploys, no hay stack rm y el queued sigue queued. Al soltarlo, el worker
// termina y el queued corre.
func TestRemoveAppStopTimeout(t *testing.T) {
	w := newWorld(t)
	block := make(chan struct{})
	w.sim.set(simOpts{Block: block})
	d, _ := w.startDispatcher(t, w.run)
	d.stopTimeout = 100 * time.Millisecond
	first := w.deploy(t, apiRef, nil, nil)
	waitFor(t, "el primero en docker stack deploy", func() bool { return len(w.docker.StackDeploys()) == 1 })
	queued := w.deploy(t, apiRef, nil, nil)

	err := w.removeApp(d).Execute(context.Background(), apiRef)
	if !errors.Is(err, service.ErrWorkerBusy) {
		t.Fatalf("RemoveApp: %v", err)
	}
	if !d.Accepting(w.apiID) || len(w.docker.RemovedStacks()) != 0 {
		t.Fatalf("Accepting %v, stack rm %v", d.Accepting(w.apiID), w.docker.RemovedStacks())
	}
	if got := w.get(t, queued); got.Status != entity.StatusQueued {
		t.Fatalf("el queued: %s", got.Status)
	}

	close(block)
	got := w.waitStatus(t, first, entity.StatusFailed)
	if *got.Error != "la app se borró durante el deploy" {
		t.Fatalf("el primero: %v", *got.Error)
	}
	w.waitStatus(t, queued, entity.StatusSucceeded)
}

// Una app que no está en la base (catalog ya la borró, o nunca existió):
// solo el stack rm.
func TestRemoveAppUnknownApp(t *testing.T) {
	w := newWorld(t)
	d, _ := w.startDispatcher(t, w.run)
	if err := w.removeApp(d).Execute(context.Background(), dto.AppRef{Project: "auditfy", Stage: "staging", App: "nada"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(w.docker.RemovedStacks(), []string{"auditfy_staging_nada"}) {
		t.Fatalf("stack rm: %v", w.docker.RemovedStacks())
	}
	w.docker.SetUnavailable(true)
	if err := w.removeApp(d).Execute(context.Background(), dto.AppRef{Project: "auditfy", Stage: "staging", App: "nada"}); !errors.Is(err, shared.ErrDockerUnavailable) {
		t.Fatalf("Docker caído: %v", err)
	}
}
