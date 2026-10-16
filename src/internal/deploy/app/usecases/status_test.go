package usecases

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	outservice "github.com/CPU-commits/muelle/src/internal/deploy/infra/out/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
)

// statusFixture es GetStatus sobre la base del fixture, el Docker en memoria
// (con su adaptador) y las variables de fakeEnv.
type statusFixture struct {
	*fixture
	docker *dockertest.Fake
	env    *fakeEnv
	u      *GetStatus
}

func newStatusFixture(t *testing.T, wrap func(service.Cluster) service.Cluster) *statusFixture {
	t.Helper()
	f := newFixture(t)
	d := dockertest.New()
	var cluster service.Cluster = outservice.NewDocker(d)
	if wrap != nil {
		cluster = wrap(cluster)
	}
	env := newFakeEnv()
	return &statusFixture{fixture: f, docker: d, env: env, u: NewGetStatus(f.apps, cluster, env, f.clock)}
}

// putAPI deja en Swarm el servicio api de auditfy/staging/api con sus
// tareas: t1 (slot 1) corre en este nodo (node-1), con su contenedor c1
// healthy; t2 (slot 2) corre en otro nodo; t0 es una vieja del slot 1 que falló.
func (f *statusFixture) putAPI() {
	two := uint64(2)
	f.docker.PutNode(docker.Node{ID: "node-2", Hostname: "worker-1"})
	f.docker.PutStackService(naming.Stack("auditfy", "staging", "api"), docker.Service{
		Name: naming.Service("auditfy", "staging", "api", "api"), Labels: naming.Labels("auditfy", "staging", "api", 7),
		Image: "ghcr.io/x/api:1@" + digestA, Replicas: &two, RunningTasks: 2, DesiredTasks: 2, MemoryLimitBytes: 512 << 20,
		Update: &docker.UpdateStatus{State: docker.UpdateCompleted},
	},
		docker.Task{ID: "t1", Slot: 1, State: docker.TaskRunning, DesiredState: docker.TaskRunning, NodeID: "node-1", ContainerID: "c1",
			CreatedAt: t0.Add(time.Minute), UpdatedAt: t0.Add(time.Minute + 123456789)},
		docker.Task{ID: "t2", Slot: 2, State: docker.TaskRunning, DesiredState: docker.TaskRunning, NodeID: "node-2", ContainerID: "c2",
			CreatedAt: t0.Add(time.Minute), UpdatedAt: t0.Add(time.Minute)},
		docker.Task{ID: "t0", Slot: 1, State: docker.TaskFailed, DesiredState: docker.TaskShutdown, NodeID: "node-1", ContainerID: "c0",
			Err: "task: non-zero exit (1)", CreatedAt: t0, UpdatedAt: t0.Add(30 * time.Second)},
	)
	f.docker.PutContainer(docker.Container{ID: "c1", Name: "auditfy_staging_api_api.1.t1", Running: true, Health: "healthy"})
	f.docker.SetStats("c1", docker.Stats{CPUPercent: 1.5, MemoryBytes: 300 << 20})
}

func (f *statusFixture) countCalls(op string) int {
	n := 0
	for _, c := range f.docker.Calls() {
		if c == op || strings.HasPrefix(c, op+" ") {
			n++
		}
	}
	return n
}

// El estado de una app (contrato §2.8): por servicio, el nombre corto, la
// imagen, las réplicas, el límite de memoria y el estado de la actualización;
// por tarea, el slot, el health solo en este nodo, el hostname del nodo, la
// hora truncada al milisegundo, y las medidas solo de lo que corre en este
// nodo. Las sumas del servicio son las de sus tareas medidas.
func TestStatusValues(t *testing.T) {
	f := newStatusFixture(t, nil)
	f.putAPI()
	// Un servicio global sin medir, sin límite y sin actualización; su
	// tarea todavía no tiene nodo.
	f.docker.PutStackService(naming.Stack("auditfy", "staging", "api"), docker.Service{
		Name: naming.Service("auditfy", "staging", "api", "cron"), Labels: naming.Labels("auditfy", "staging", "api", 7),
		Image: "ghcr.io/x/cron:1", DesiredTasks: 1,
	}, docker.Task{ID: "tg", State: docker.TaskPending, DesiredState: docker.TaskRunning, UpdatedAt: t0})
	// La tarea vieja tiene su contenedor en este nodo: una histórica no
	// lleva health. Y una tarea en un nodo que ya no está sale con su id.
	f.docker.PutContainer(docker.Container{ID: "c0", Name: "auditfy_staging_api_api.1.t0", Health: "unhealthy"})
	f.docker.SetTasks(naming.Service("auditfy", "staging", "api", "cron"),
		docker.Task{ID: "tg", State: docker.TaskPending, DesiredState: docker.TaskRunning, UpdatedAt: t0},
		docker.Task{ID: "tx", State: docker.TaskShutdown, DesiredState: docker.TaskShutdown, NodeID: "node-gone", UpdatedAt: t0.Add(-time.Hour)})
	// Otra app del stage no aparece.
	f.docker.PutStackService(naming.Stack("auditfy", "staging", "web"), docker.Service{
		Name: naming.Service("auditfy", "staging", "web", "web"), Labels: naming.Labels("auditfy", "staging", "web", 3),
	})

	st, err := f.u.Execute(context.Background(), apiRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Services) != 2 || st.Services[0].Name != "api" || st.Services[1].Name != "cron" {
		t.Fatalf("servicios: %+v", st.Services)
	}
	api, cron := st.Services[0], st.Services[1]
	if api.Image != "ghcr.io/x/api:1@"+digestA || api.ReplicasDesired != 2 || api.ReplicasRunning != 2 ||
		api.MemoryLimitBytes == nil || *api.MemoryLimitBytes != 512<<20 || api.UpdateState == nil || *api.UpdateState != "completed" ||
		api.CPUPercent == nil || *api.CPUPercent != 1.5 || api.MemoryBytes == nil || *api.MemoryBytes != 300<<20 {
		t.Fatalf("api: %+v", api)
	}
	if len(api.Tasks) != 3 {
		t.Fatalf("tareas de api: %+v", api.Tasks)
	}
	t1, t2, old := api.Tasks[0], api.Tasks[1], api.Tasks[2]
	if t1.ID != "t1" || t1.Slot == nil || *t1.Slot != 1 || t1.State != "running" || t1.DesiredState != "running" ||
		t1.Health == nil || *t1.Health != "healthy" || t1.Node == nil || *t1.Node != "manager-1" ||
		t1.Since == nil || !t1.Since.Equal(t0.Add(time.Minute+123*time.Millisecond)) || t1.Error != nil ||
		t1.CPUPercent == nil || *t1.CPUPercent != 1.5 || t1.MemoryBytes == nil || *t1.MemoryBytes != 300<<20 {
		t.Fatalf("t1: %+v", t1)
	}
	if t2.ID != "t2" || t2.Health != nil || t2.Node == nil || *t2.Node != "worker-1" || t2.CPUPercent != nil || t2.MemoryBytes != nil {
		t.Fatalf("t2 (en otro nodo): %+v", t2)
	}
	if old.ID != "t0" || old.Error == nil || *old.Error != "task: non-zero exit (1)" || old.Health != nil || old.CPUPercent != nil {
		t.Fatalf("t0 (vieja): %+v", old)
	}
	if cron.ReplicasDesired != 1 || cron.ReplicasRunning != 0 || cron.MemoryLimitBytes != nil || cron.UpdateState != nil ||
		cron.CPUPercent != nil || cron.MemoryBytes != nil || len(cron.Tasks) != 2 || cron.Tasks[0].Slot != nil || cron.Tasks[0].Node != nil {
		t.Fatalf("cron (global): %+v", cron)
	}
	if gone := cron.Tasks[1]; gone.ID != "tx" || gone.Node == nil || *gone.Node != "node-gone" {
		t.Fatalf("tarea en un nodo que ya no está: %+v", gone)
	}
	// Solo se miden las tareas que corren en este nodo.
	if n := f.countCalls("ContainerStats"); n != 1 {
		t.Fatalf("ContainerStats: %d llamadas (%v)", n, f.docker.Calls())
	}
}

// Task.Err puede traer un argumento de command ya interpolado: sale
// enmascarado con los secretos de la app, como el log (contrato §2.8).
func TestStatusMasksTaskError(t *testing.T) {
	f := newStatusFixture(t, nil)
	const secret = "s3cr3t-valor-de-la-app"
	f.env.set(f.apiID, map[string]string{"TOKEN": secret}, secret)
	f.docker.PutStackService(naming.Stack("auditfy", "staging", "api"), docker.Service{
		Name: naming.Service("auditfy", "staging", "api", "api"), Labels: naming.AppLabels("auditfy", "staging", "api"),
	}, docker.Task{ID: "t1", Slot: 1, State: docker.TaskRejected, DesiredState: docker.TaskShutdown,
		Err: `starting container failed: exec: "` + secret + `": executable file not found`})
	st, err := f.u.Execute(context.Background(), apiRef)
	if err != nil {
		t.Fatal(err)
	}
	e := st.Services[0].Tasks[0].Error
	if e == nil || strings.Contains(*e, secret) || !strings.Contains(*e, `exec: "***"`) {
		t.Fatalf("error: %v", e)
	}
}

// El orden de las tareas (contrato §2.8): primero las de desired_state
// running, después por slot y por since, de la más nueva a la más vieja. Los
// servicios, por nombre.
func TestStatusOrder(t *testing.T) {
	f := newStatusFixture(t, nil)
	stack := naming.Stack("auditfy", "staging", "api")
	labels := naming.AppLabels("auditfy", "staging", "api")
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	two := uint64(2)
	f.docker.PutStackService(stack, docker.Service{Name: naming.Service("auditfy", "staging", "api", "web"), Labels: labels, Replicas: &two},
		docker.Task{ID: "a", Slot: 2, DesiredState: "running", UpdatedAt: at(1)},
		docker.Task{ID: "c", Slot: 1, DesiredState: "shutdown", UpdatedAt: at(5)},
		docker.Task{ID: "e", Slot: 2, DesiredState: "shutdown", UpdatedAt: at(4)},
		docker.Task{ID: "b", Slot: 1, DesiredState: "running", UpdatedAt: at(0)},
		docker.Task{ID: "d", Slot: 1, DesiredState: "shutdown", UpdatedAt: at(3)},
	)
	f.docker.PutStackService(stack, docker.Service{Name: naming.Service("auditfy", "staging", "api", "api"), Labels: labels})
	st, err := f.u.Execute(context.Background(), apiRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Services) != 2 || st.Services[0].Name != "api" || st.Services[1].Name != "web" {
		t.Fatalf("servicios: %+v", st.Services)
	}
	var ids []string
	for _, task := range st.Services[1].Tasks {
		ids = append(ids, task.ID)
	}
	if want := []string{"b", "a", "c", "d", "e"}; !slices.Equal(ids, want) {
		t.Fatalf("orden %v, esperaba %v", ids, want)
	}
}

// La caché por app (contrato §2.8): lo estructural 2 s y las medidas 5 s. Una
// tarea que no estaba en la medición guardada sale sin medidas.
func TestStatusCaches(t *testing.T) {
	f := newStatusFixture(t, nil)
	f.putAPI()
	ctx := context.Background()
	get := func() dto.AppStatus {
		t.Helper()
		st, err := f.u.Execute(ctx, apiRef)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	calls := func(services, stats int) {
		t.Helper()
		if s, c := f.countCalls("ListServices"), f.countCalls("ContainerStats"); s != services || c != stats {
			t.Fatalf("ListServices %d (esperaba %d), ContainerStats %d (esperaba %d)", s, services, c, stats)
		}
	}
	get()
	get()
	calls(1, 1)

	// Una tarea nueva en este nodo, que la medición guardada no tiene.
	f.docker.SetTasks(naming.Service("auditfy", "staging", "api", "api"),
		docker.Task{ID: "t1", Slot: 1, State: docker.TaskRunning, DesiredState: docker.TaskRunning, NodeID: "node-1", ContainerID: "c1"},
		docker.Task{ID: "t3", Slot: 2, State: docker.TaskRunning, DesiredState: docker.TaskRunning, NodeID: "node-1", ContainerID: "c3"},
	)
	f.docker.PutContainer(docker.Container{ID: "c3", Name: "auditfy_staging_api_api.2.t3", Running: true})
	f.docker.SetStats("c3", docker.Stats{CPUPercent: 2, MemoryBytes: 100 << 20})
	f.clock.Advance(2 * time.Second)
	st := get()
	calls(2, 1)
	tasks := st.Services[0].Tasks
	if len(tasks) != 2 || tasks[0].CPUPercent == nil || tasks[1].ID != "t3" || tasks[1].CPUPercent != nil || tasks[1].MemoryBytes != nil {
		t.Fatalf("con la medición guardada: %+v", tasks)
	}
	if s := st.Services[0]; s.CPUPercent == nil || *s.CPUPercent != 1.5 {
		t.Fatalf("la suma es la de las medidas: %+v", s)
	}

	f.clock.Advance(3 * time.Second)
	st = get()
	calls(3, 3)
	if s := st.Services[0]; s.CPUPercent == nil || *s.CPUPercent != 3.5 || s.MemoryBytes == nil || *s.MemoryBytes != 400<<20 {
		t.Fatalf("medidas nuevas: %+v", s)
	}
}

// holdStats es un Cluster que retiene ContainerStats hasta release.
type holdStats struct {
	service.Cluster
	mu      sync.Mutex
	calls   int
	release chan struct{}
}

func (h *holdStats) ContainerStats(ctx context.Context, id string) (service.Stats, error) {
	h.mu.Lock()
	h.calls++
	h.mu.Unlock()
	select {
	case <-h.release:
	case <-ctx.Done():
		return service.Stats{}, ctx.Err()
	}
	return h.Cluster.ContainerStats(ctx, id)
}

// Con la caché fría, varios GET a la vez comparten una sola medición (y una
// sola lectura de los servicios): cinco pestañas no hacen cinco rondas de
// ContainerStats.
func TestStatusConcurrentColdRequests(t *testing.T) {
	hold := &holdStats{release: make(chan struct{})}
	f := newStatusFixture(t, func(c service.Cluster) service.Cluster { hold.Cluster = c; return hold })
	f.putAPI()
	var wg sync.WaitGroup
	results := make([]dto.AppStatus, 5)
	errs := make([]error, 5)
	for i := range 5 {
		wg.Go(func() { results[i], errs[i] = f.u.Execute(context.Background(), apiRef) })
	}
	// Espera a que la medición esté retenida y a que lleguen los demás.
	deadline := time.Now().Add(2 * time.Second)
	for {
		hold.mu.Lock()
		n := hold.calls
		hold.mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	close(hold.release)
	wg.Wait()
	if hold.calls != 1 {
		t.Fatalf("ContainerStats: %d llamadas, esperaba 1", hold.calls)
	}
	if n := f.countCalls("ListServices"); n != 1 {
		t.Fatalf("ListServices: %d llamadas, esperaba 1", n)
	}
	for i := range 5 {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		task := results[i].Services[0].Tasks[0]
		if task.CPUPercent == nil || *task.CPUPercent != 1.5 {
			t.Fatalf("resultado %d: %+v", i, task)
		}
	}
}

// Una app que nunca se desplegó (o cuyo stack se borró): services vacío, sin
// medir nada.
func TestStatusNeverDeployed(t *testing.T) {
	f := newStatusFixture(t, nil)
	st, err := f.u.Execute(context.Background(), apiRef)
	if err != nil || st.Services == nil || len(st.Services) != 0 {
		t.Fatalf("%+v, %v", st, err)
	}
	if _, err := f.u.Execute(context.Background(), dto.AppRef{Project: "auditfy", Stage: "staging", App: "nada"}); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("app que no existe: %v", err)
	}
}

// Con Docker caído, 503; el error no queda en la caché: cuando Docker vuelve,
// el próximo GET responde.
func TestStatusDockerDown(t *testing.T) {
	f := newStatusFixture(t, nil)
	f.putAPI()
	f.docker.SetUnavailable(true)
	if _, err := f.u.Execute(context.Background(), apiRef); !errors.Is(err, shared.ErrDockerUnavailable) {
		t.Fatalf("Docker caído: %v", err)
	}
	f.docker.SetUnavailable(false)
	st, err := f.u.Execute(context.Background(), apiRef)
	if err != nil || len(st.Services) != 1 {
		t.Fatalf("Docker de vuelta: %+v, %v", st, err)
	}
}
