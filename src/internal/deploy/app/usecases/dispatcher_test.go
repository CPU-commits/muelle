package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

var webRef = dto.AppRef{Project: "auditfy", Stage: "staging", App: "web"}

// startDispatcher arma el despachador sobre run (con su Hub), lo pone como
// cola de w.deps y lo arranca como Module.Run: con las apps que tienen un
// deploy queued o running. cancel lo detiene y espera a sus workers (también
// al terminar el test).
func (w *world) startDispatcher(t *testing.T, run *RunDeploy) (*Dispatcher, func()) {
	t.Helper()
	d := NewDispatcher(run, w.slog)
	w.deps.Queue = d
	ctx, cancel := context.WithCancel(context.Background())
	apps, err := w.deploys.ActiveApps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	d.Start(ctx, apps)
	stop := func() { cancel(); d.Wait() }
	t.Cleanup(stop)
	return d, stop
}

// waitStatus espera a que el deploy llegue a status.
func (w *world) waitStatus(t *testing.T, id int64, status entity.Status) entity.Deploy {
	t.Helper()
	var d entity.Deploy
	waitFor(t, fmt.Sprintf("deploy %d %s", id, status), func() bool {
		d = w.get(t, id)
		return d.Status == status
	})
	return d
}

// Un deploy a la vez por app; apps distintas en paralelo (§7 paso 1).
func TestDispatcherOneAtATimePerApp(t *testing.T) {
	w := newWorld(t)
	block := make(chan struct{})
	w.sim.set(simOpts{Block: block})
	w.startDispatcher(t, w.run)
	api1 := w.deploy(t, apiRef, nil, nil)
	web1 := w.deploy(t, webRef, nil, nil)
	waitFor(t, "las dos apps en docker stack deploy a la vez", func() bool { return len(w.docker.StackDeploys()) == 2 })
	api2 := w.deploy(t, apiRef, nil, nil)
	time.Sleep(100 * time.Millisecond)
	if n := len(w.docker.StackDeploys()); n != 2 {
		t.Fatalf("la misma app desplegó dos veces a la vez: %d", n)
	}
	if d := w.get(t, api2); d.Status != entity.StatusQueued {
		t.Fatalf("el segundo de api: %s", d.Status)
	}
	close(block)
	for _, id := range []int64{api1, web1, api2} {
		w.waitStatus(t, id, entity.StatusSucceeded)
	}
	if n := len(w.docker.StackDeploys()); n != 3 {
		t.Fatalf("docker stack deploy: %d", n)
	}
}

// Tres POST mientras uno corre: los dos primeros quedan superseded y corre
// solo el último (contrato §2.2).
func TestDispatcherSupersede(t *testing.T) {
	w := newWorld(t)
	block := make(chan struct{})
	w.sim.set(simOpts{Block: block})
	w.startDispatcher(t, w.run)
	first := w.deploy(t, apiRef, nil, nil)
	waitFor(t, "el primero en docker stack deploy", func() bool { return len(w.docker.StackDeploys()) == 1 })
	var ids []int64
	for range 3 {
		ids = append(ids, w.deploy(t, apiRef, nil, nil))
	}
	close(block)
	w.waitStatus(t, first, entity.StatusSucceeded)
	last := w.waitStatus(t, ids[2], entity.StatusSucceeded)
	for _, id := range ids[:2] {
		if d := w.get(t, id); d.Status != entity.StatusSuperseded || d.SupersededBy == nil {
			t.Fatalf("deploy %d: %s", id, d.Status)
		}
	}
	if n := len(w.docker.StackDeploys()); n != 2 {
		t.Fatalf("docker stack deploy: %d", n)
	}
	if svc, _ := w.docker.InspectService(context.Background(), "auditfy_staging_api_api"); svc.Labels[naming.LabelDeploy] != strconv.FormatInt(last.ID, 10) {
		t.Fatalf("etiquetas: %v", svc.Labels)
	}
}

// Un Wake antes de Start se anota y corre al arrancar.
func TestDispatcherWakeBeforeStart(t *testing.T) {
	w := newWorld(t)
	d := NewDispatcher(w.run, w.slog)
	w.deps.Queue = d
	id := w.deploy(t, apiRef, nil, nil)
	time.Sleep(50 * time.Millisecond)
	if got := w.get(t, id); got.Status != entity.StatusQueued {
		t.Fatalf("antes de Start: %s", got.Status)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer func() { cancel(); d.Wait() }()
	d.Start(ctx, nil)
	w.waitStatus(t, id, entity.StatusSucceeded)
}

// Con la app bloqueada (se está borrando), un queued que llega por un
// CreateDeploy que pasó Accepting antes del Block nunca se reclama: queda
// failed "la app se borró" sin tocar Swarm (decisión 8). Después de Unblock,
// un deploy nuevo sí corre.
func TestDispatcherBlockedApp(t *testing.T) {
	w := newWorld(t)
	d, _ := w.startDispatcher(t, w.run)
	d.Block(w.apiID, time.Minute)
	if d.Accepting(w.apiID) || !d.Accepting(w.otherID) {
		t.Fatal("Accepting")
	}
	ctx := context.Background()
	q := entity.Deploy{AppID: w.apiID, Trigger: entity.TriggerAPI, ActorType: entity.ActorToken, ActorName: "ci", CreatedAt: time.Now()}
	if err := w.deploys.Create(ctx, &q); err != nil {
		t.Fatal(err)
	}
	wait := w.hub.Wait(q.ID)
	d.Wake(w.apiID)
	got := w.waitStatus(t, q.ID, entity.StatusFailed)
	if got.Error == nil || *got.Error != "la app se borró" || got.StartedAt != nil {
		t.Fatalf("deploy: %+v", got)
	}
	select {
	case <-wait:
	case <-time.After(time.Second):
		t.Fatal("no avisó a los streams")
	}
	if n := len(w.docker.StackDeploys()); n != 0 {
		t.Fatalf("docker stack deploy: %d", n)
	}
	if _, err := NewCreateDeploy(w.deps).Execute(tokenCtx(), dto.CreateDeploy{AppRef: apiRef}); !errors.Is(err, service.ErrAppRemoving) {
		t.Fatalf("CreateDeploy con la app bloqueada: %v", err)
	}

	d.Unblock(w.apiID)
	id := w.deploy(t, apiRef, nil, nil)
	w.waitStatus(t, id, entity.StatusSucceeded)
}

// El bloqueo vence solo.
func TestDispatcherBlockExpires(t *testing.T) {
	w := newWorld(t)
	clk := clock.NewManual(time.Now())
	run := *w.run
	run.Clock = clk
	d := NewDispatcher(&run, w.slog)
	d.Block(w.apiID, 2*time.Minute)
	clk.Advance(2*time.Minute - time.Millisecond)
	if d.Accepting(w.apiID) {
		t.Fatal("todavía está bloqueada")
	}
	clk.Advance(time.Millisecond)
	if !d.Accepting(w.apiID) {
		t.Fatal("el bloqueo venció")
	}
}

// El apagado durante el paso 6 deja el deploy running y Run vuelve; el
// próximo arranque (otro despachador sobre la misma base, con los servicios
// ya etiquetados) lo cierra succeeded con "muelle se reinició durante el
// deploy" (contrato §1.2).
func TestRunShutdownLeavesRunning(t *testing.T) {
	w := newWorld(t)
	exec(t, w.db, `UPDATE apps SET deploy_timeout_s = 60`)
	w.sim.set(simOpts{Running: new(uint64(0))})
	_, stop := w.startDispatcher(t, w.run)
	id := w.deploy(t, apiRef, nil, nil)
	waitFor(t, "esperando convergencia", func() bool { return strings.Contains(w.log(t, id), "esperando convergencia") })
	stop()
	if d := w.get(t, id); d.Status != entity.StatusRunning || d.FinishedAt != nil {
		t.Fatalf("después del apagado: %s", d.Status)
	}

	// Las tareas ya corren.
	svc, _ := w.docker.InspectService(context.Background(), "auditfy_staging_api_api")
	svc.RunningTasks = 1
	w.docker.PutService(svc)
	w2 := *w.run
	w2.Hub = NewHub()
	w2.Log = NewLogWriter(w.deploys, w2.Hub, clock.System{})
	w.startDispatcher(t, &w2)
	d := w.waitStatus(t, id, entity.StatusSucceeded)
	if err := containsInOrder(w.texts(t, d.ID), "esperando convergencia", "muelle se reinició durante el deploy",
		"esperando convergencia", "api: 1/1 réplicas corriendo", "advertencia: no se pudo actualizar Caddy", "succeeded en "); err != nil {
		t.Fatal(err)
	}
	if n := len(w.docker.StackDeploys()); n != 1 {
		t.Fatalf("el reinicio no vuelve a aplicar el stack: %d", n)
	}
}

// running deja un deploy de la app como si muelle se hubiera cortado: running
// desde startedAt, con requested_images (si no son nil).
func (w *world) running(t *testing.T, appID int64, startedAt time.Time, requested map[string]string) int64 {
	t.Helper()
	ctx := context.Background()
	d := entity.Deploy{AppID: appID, Trigger: entity.TriggerAPI, ActorType: entity.ActorToken, ActorName: "ci", CreatedAt: startedAt}
	if err := w.deploys.Create(ctx, &d); err != nil {
		t.Fatal(err)
	}
	if ok, err := w.deploys.Claim(ctx, d.ID, startedAt); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if requested != nil {
		if err := w.deploys.SetImages(ctx, d.ID, requested, requested, requested); err != nil {
			t.Fatal(err)
		}
	}
	return d.ID
}

// putService deja en Swarm el servicio svc de la app con la etiqueta del deploy.
func (w *world) putService(app, svc string, deployID int64, s docker.Service) {
	s.Name = naming.Service("auditfy", "staging", app, svc)
	s.Labels = naming.Labels("auditfy", "staging", app, deployID)
	if s.Replicas == nil {
		s.Replicas = new(uint64(1))
	}
	w.docker.PutStackService(naming.Stack("auditfy", "staging", app), s)
}

// Un running sin requested_images, o cuyos servicios no llevan su
// muelle.deploy, nunca llegó a Swarm.
func TestResumeNeverApplied(t *testing.T) {
	w := newWorld(t)
	started := time.Now().Add(-time.Minute)
	a := w.running(t, w.apiID, started, nil)
	b := w.running(t, w.infraID, started, map[string]string{"web": "ghcr.io/x/web:1", "worker": "ghcr.io/x/worker:1"})
	w.putService("infra", "web", 1, docker.Service{RunningTasks: 1, CreatedAt: started.Add(-time.Hour)})
	w.startDispatcher(t, w.run)
	for _, id := range []int64{a, b} {
		d := w.waitStatus(t, id, entity.StatusFailed)
		if *d.Error != reasonNeverApplied {
			t.Fatalf("error: %s", *d.Error)
		}
		if err := containsInOrder(w.texts(t, id), "muelle se reinició durante el deploy", "failed: "+reasonNeverApplied); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(w.docker.StackDeploys()); n != 0 {
		t.Fatalf("docker stack deploy: %d", n)
	}
}

// Solo algunos servicios llevan su muelle.deploy: el CLI se cortó a mitad.
func TestResumePartial(t *testing.T) {
	w := newWorld(t)
	started := time.Now().Add(-time.Minute)
	id := w.running(t, w.infraID, started, map[string]string{"web": "ghcr.io/x/web:1", "worker": "ghcr.io/x/worker:1"})
	w.putService("infra", "web", id, docker.Service{RunningTasks: 1, CreatedAt: started})
	w.putService("infra", "worker", id-1, docker.Service{RunningTasks: 1, CreatedAt: started.Add(-time.Hour)})
	w.startDispatcher(t, w.run)
	d := w.waitStatus(t, id, entity.StatusFailed)
	if *d.Error != reasonPartialApplied {
		t.Fatalf("error: %s", *d.Error)
	}
}

// Un rollback de Swarm devuelve las etiquetas anteriores: si su UpdateStatus
// es de este deploy, cierra con el rollback, no con "Swarm no cambió". Uno de
// antes de started_at no cuenta.
func TestResumeAfterSwarmRollback(t *testing.T) {
	for _, c := range []struct {
		name  string
		delta time.Duration // del UpdateStatus respecto de started_at
		want  string
	}{
		{"rollback de este deploy", 10 * time.Second, "api: Swarm hizo rollback (rollback_completed): update rolled back"},
		{"rollback anterior", -time.Minute, reasonNeverApplied},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := newWorld(t)
			started := time.Now().Add(-time.Minute)
			id := w.running(t, w.apiID, started, map[string]string{"api": "ghcr.io/x/api:2"})
			at := started.Add(c.delta)
			w.putService("api", "api", id-1, docker.Service{RunningTasks: 1, CreatedAt: started.Add(-time.Hour),
				Update: &docker.UpdateStatus{State: docker.UpdateRollbackCompleted, Message: "update rolled back", StartedAt: &at}})
			w.startDispatcher(t, w.run)
			d := w.waitStatus(t, id, entity.StatusFailed)
			if *d.Error != c.want {
				t.Fatalf("error %q, esperaba %q\n%s", *d.Error, c.want, w.log(t, id))
			}
		})
	}
}

// Con Docker caído, el deploy queda running y se reintenta; lo cierra cuando
// Docker vuelve.
func TestResumeDockerDown(t *testing.T) {
	w := newWorld(t)
	started := time.Now()
	id := w.running(t, w.apiID, started, map[string]string{"api": "ghcr.io/x/api:1"})
	w.putService("api", "api", id, docker.Service{RunningTasks: 1, CreatedAt: started})
	w.docker.SetUnavailable(true)
	w.startDispatcher(t, w.run)
	waitFor(t, "el aviso de Docker caído", func() bool { return strings.Contains(w.log(t, id), "Docker no responde") })
	time.Sleep(3 * testTiming.RetryEvery)
	if d := w.get(t, id); d.Status != entity.StatusRunning {
		t.Fatalf("con Docker caído: %s", d.Status)
	}
	w.docker.SetUnavailable(false)
	w.waitStatus(t, id, entity.StatusSucceeded)
	if n := strings.Count(w.log(t, id), "muelle se reinició durante el deploy"); n != 1 {
		t.Fatalf("la línea del reinicio %d veces:\n%s", n, w.log(t, id))
	}
}

// La retención (contrato §2.3) se aplica al cerrar cada deploy.
func TestRetentionAfterClose(t *testing.T) {
	w := newWorld(t)
	for range 25 {
		w.succeed(t, w.apiID, map[string]string{"api": "ghcr.io/x/api:1@" + digestA})
	}
	w.startDispatcher(t, w.run)
	id := w.deploy(t, apiRef, nil, nil)
	w.waitStatus(t, id, entity.StatusSucceeded)
	waitFor(t, "la retención", func() bool {
		list, err := w.deploys.List(context.Background(), w.apiID)
		return err == nil && len(list) == 20 && list[0].ID == id
	})
}

// slog nunca recibe un secreto (decisión 18): ni de un error de render (el
// valor interpolado va en el error de compose-go) ni de la salida del CLI.
func TestDispatcherNeverLogsSecrets(t *testing.T) {
	const secret = "no-es-un-numero-secreto"
	w := newWorld(t)
	exec(t, w.db, `UPDATE apps SET compose_yaml = ? WHERE id = ?`, `services:
  web:
    image: ghcr.io/x/web:${TAG}
    deploy:
      replicas: ${N}
  worker:
    image: ghcr.io/x/worker:1
`, w.infraID)
	w.env.set(w.infraID, map[string]string{"TAG": "1", "N": secret}, secret)
	w.env.set(w.apiID, map[string]string{"TOKEN": secret}, secret)
	w.sim.set(simOpts{Fail: fmt.Errorf("el CLI dijo %s", secret)})
	w.startDispatcher(t, w.run)
	a := w.deploy(t, infraRef, nil, nil)
	b := w.deploy(t, apiRef, nil, nil)
	da := w.waitStatus(t, a, entity.StatusFailed)
	db := w.waitStatus(t, b, entity.StatusFailed)
	if !strings.HasPrefix(*da.Error, "render: ") || !strings.HasPrefix(*db.Error, "docker stack deploy falló") {
		t.Fatalf("errores: %q, %q", *da.Error, *db.Error)
	}
	waitFor(t, "los dos en slog", func() bool { return strings.Count(w.logs.String(), `"msg":"deploy cerrado"`) == 2 })
	out := w.logs.String()
	if strings.Contains(out, secret) || strings.Contains(w.log(t, a), secret) || strings.Contains(w.log(t, b), secret) {
		t.Fatalf("filtra el secreto:\n%s", out)
	}
	for line := range strings.Lines(out) {
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatal(err)
		}
		if rec["msg"] != "deploy cerrado" {
			continue
		}
		if rec["app"] == nil || rec["deploy"] == nil || rec["status"] != "failed" || rec["error"] == nil {
			t.Fatalf("registro: %v", rec)
		}
	}
	if !slices.ContainsFunc(strings.Split(out, "\n"), func(l string) bool { return strings.Contains(l, `"deploy":`+strconv.FormatInt(a, 10)) }) {
		t.Fatalf("falta el deploy %d:\n%s", a, out)
	}
}
