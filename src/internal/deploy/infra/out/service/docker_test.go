package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
)

const digestA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// Digest pasa la credencial tal cual (o ninguna) y traduce los errores de
// Docker a los del puerto: no existe, sin acceso y Docker caído.
func TestDockerImagesDigest(t *testing.T) {
	f := dockertest.New()
	d := NewDocker(f)
	ctx := context.Background()
	f.SetDigest("ghcr.io/x/api:1", digestA)

	got, err := d.Digest(ctx, "ghcr.io/x/api:1", &service.Credential{Host: "ghcr.io", Username: "bot", Password: "s3cr3t-pass"})
	if err != nil || got != digestA {
		t.Fatalf("Digest: %q, %v", got, err)
	}
	if _, err := d.Digest(ctx, "nginx", nil); !errors.Is(err, service.ErrImageNotFound) {
		t.Fatalf("no existe: %v", err)
	}
	f.DenyImage("ghcr.io/x/privada:1")
	if _, err := d.Digest(ctx, "ghcr.io/x/privada:1", nil); !errors.Is(err, service.ErrImageDenied) {
		t.Fatalf("sin acceso: %v", err)
	}
	f.FailNext("DistributionDigest", fmt.Errorf("caído: %w", docker.ErrUnavailable))
	if _, err := d.Digest(ctx, "ghcr.io/x/api:1", nil); !errors.Is(err, entity.ErrDockerUnavailable) {
		t.Fatalf("Docker caído: %v", err)
	}
	calls := f.DistributionCalls()
	if len(calls) != 3 || calls[0].Auth == nil || calls[0].Auth.Host != "ghcr.io" || calls[0].Auth.Username != "bot" ||
		calls[0].Auth.Password != "s3cr3t-pass" || calls[1].Auth != nil {
		t.Fatalf("credenciales: %+v", calls)
	}
}

// Deploy ejecuta docker stack deploy con el compose y solo las credenciales
// que recibe, y devuelve la salida del CLI también si falla.
func TestDockerStacksDeploy(t *testing.T) {
	f := dockertest.New()
	s := NewDockerStacks(f)
	ctx := context.Background()
	out, err := s.Deploy(ctx, "p_s_api", []byte(`{"services":{}}`),
		[]service.Credential{{Host: "ghcr.io", Username: "bot", Password: "s3cr3t-pass"}})
	if err != nil || !strings.Contains(out, "p_s_api") {
		t.Fatalf("Deploy: %q, %v", out, err)
	}
	got := f.StackDeploys()
	if len(got) != 1 || got[0].Name != "p_s_api" || string(got[0].Compose) != `{"services":{}}` ||
		!slices.Equal(got[0].Registries, []docker.RegistryAuth{{Host: "ghcr.io", Username: "bot", Password: "s3cr3t-pass"}}) {
		t.Fatalf("StackDeploys: %+v", got)
	}
	f.FailNext("DeployStack", errors.New("boom"))
	out, err = s.Deploy(ctx, "p_s_api", []byte(`{}`), nil)
	if !errors.Is(err, docker.ErrCommand) || !strings.Contains(out, "boom") {
		t.Fatalf("falla: %q, %v", out, err)
	}
	if got := f.StackDeploys(); len(got[1].Registries) != 0 {
		t.Fatalf("sin credenciales: %+v", got[1].Registries)
	}
}

// AppServices trae solo los servicios con las etiquetas de la app, con lo que
// la convergencia necesita.
func TestDockerClusterAppServices(t *testing.T) {
	f := dockertest.New()
	d := NewDocker(f)
	ctx := context.Background()
	started := time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC)
	created := started.Add(-time.Hour)
	two := uint64(2)
	labels := naming.Labels("p", "s", "api", 7)
	f.PutStackService("p_s_api", docker.Service{
		Name: "p_s_api_api", Labels: labels, Image: "x:1@" + digestA, Replicas: &two, RunningTasks: 1, DesiredTasks: 2,
		MemoryLimitBytes: 64 << 20, CreatedAt: created,
		Update: &docker.UpdateStatus{State: docker.UpdateRollbackCompleted, Message: "rollback completed", StartedAt: &started},
	})
	f.PutStackService("p_s_web", docker.Service{Name: "p_s_web_web", Labels: naming.Labels("p", "s", "web", 8)})
	f.PutService(docker.Service{Name: "suelto"})

	got, err := d.AppServices(ctx, "p", "s", "api")
	if err != nil || len(got) != 1 {
		t.Fatalf("AppServices: %+v, %v", got, err)
	}
	s := got[0]
	if s.ID == "" || s.Name != "p_s_api_api" || s.Labels[naming.LabelDeploy] != "7" || s.Image != "x:1@"+digestA ||
		s.Replicas == nil || *s.Replicas != 2 || s.RunningTasks != 1 || s.DesiredTasks != 2 || s.MemoryLimitBytes != 64<<20 ||
		!s.CreatedAt.Equal(created) || s.Update == nil || s.Update.State != service.UpdateRollbackCompleted ||
		s.Update.Message != "rollback completed" || s.Update.StartedAt == nil || !s.Update.StartedAt.Equal(started) {
		t.Fatalf("servicio: %+v", s)
	}
	f.SetUnavailable(true)
	if _, err := d.AppServices(ctx, "p", "s", "api"); !errors.Is(err, entity.ErrDockerUnavailable) {
		t.Fatalf("Docker caído: %v", err)
	}
}

// Service: ErrSwarmNotFound si no existe; Tasks de la más nueva a la más
// vieja; TaskLogs las últimas tail líneas, con los pedazos tal cual.
func TestDockerClusterServiceTasksAndLogs(t *testing.T) {
	f := dockertest.New()
	d := NewDocker(f)
	ctx := context.Background()
	if _, err := d.Service(ctx, "p_s_api_api"); !errors.Is(err, service.ErrSwarmNotFound) {
		t.Fatalf("no existe: %v", err)
	}
	old := time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC)
	f.PutService(docker.Service{Name: "p_s_api_api"},
		docker.Task{ID: "t-old", Slot: 1, State: docker.TaskFailed, Err: "exit 1", CreatedAt: old},
		docker.Task{ID: "t-new", Slot: 1, State: docker.TaskRunning, NodeID: "node-1", ContainerID: "c1", CreatedAt: old.Add(time.Minute)})
	svc, err := d.Service(ctx, "p_s_api_api")
	if err != nil || svc.Name != "p_s_api_api" {
		t.Fatalf("Service: %+v, %v", svc, err)
	}
	tasks, err := d.Tasks(ctx, svc.ID)
	if err != nil || len(tasks) != 2 || tasks[0].ID != "t-new" || tasks[1].ID != "t-old" || tasks[1].Err != "exit 1" ||
		tasks[1].State != "failed" || tasks[0].NodeID != "node-1" || tasks[0].ContainerID != "c1" || tasks[0].Slot != 1 ||
		tasks[0].ServiceID != svc.ID {
		t.Fatalf("Tasks: %+v, %v", tasks, err)
	}

	for i := range 60 {
		f.AppendTaskLogs("t-old", docker.LogLine{Time: old.Add(time.Duration(i) * time.Second), Stream: docker.StreamStderr, TaskID: "t-old", Text: fmt.Sprintf("línea %d", i)})
	}
	f.AppendTaskLogs("t-old", docker.LogLine{Stream: docker.StreamStderr, Text: "pedazo"})
	lines, err := d.TaskLogs(ctx, "t-old", 50)
	if err != nil || len(lines) != 50 || lines[0].Text != "línea 11" || lines[48].Text != "línea 59" ||
		lines[48].TaskID != "t-old" || lines[48].Stream != docker.StreamStderr || !lines[48].Time.Equal(old.Add(59*time.Second)) ||
		lines[49].Text != "pedazo" || !lines[49].Time.IsZero() || lines[49].TaskID != "" {
		t.Fatalf("TaskLogs: %d %+v, %v", len(lines), lines, err)
	}
	if _, err := d.TaskLogs(ctx, "t-nada", 50); !errors.Is(err, service.ErrSwarmNotFound) {
		t.Fatalf("tarea que no existe: %v", err)
	}
	f.FailNext("ListTasks", fmt.Errorf("caído: %w", docker.ErrUnavailable))
	if _, err := d.Tasks(ctx, svc.ID); !errors.Is(err, entity.ErrDockerUnavailable) {
		t.Fatalf("Docker caído: %v", err)
	}
}

// LocalNodeID es el id del nodo en el Swarm; sin Swarm, Docker no está
// disponible para muelle (503).
func TestDockerClusterLocalNodeID(t *testing.T) {
	f := dockertest.New()
	d := NewDocker(f)
	ctx := context.Background()
	if id, err := d.LocalNodeID(ctx); err != nil || id != "node-1" {
		t.Fatalf("LocalNodeID: %q, %v", id, err)
	}
	f.SetInfo(docker.Info{SwarmState: docker.SwarmInactive})
	if _, err := d.LocalNodeID(ctx); !errors.Is(err, entity.ErrDockerUnavailable) {
		t.Fatalf("sin Swarm: %v", err)
	}
	f.FailNext("Info", fmt.Errorf("caído: %w", docker.ErrUnavailable))
	if _, err := d.LocalNodeID(ctx); !errors.Is(err, entity.ErrDockerUnavailable) {
		t.Fatalf("Docker caído: %v", err)
	}
}

// ServiceLogs entrega las líneas del servicio con sus opciones (Tail, Since)
// y los pedazos tal cual; ErrSwarmNotFound si el servicio no está.
func TestDockerClusterServiceLogs(t *testing.T) {
	f := dockertest.New()
	d := NewDocker(f)
	ctx := context.Background()
	if _, err := d.ServiceLogs(ctx, "svc-nada", service.LogOptions{Tail: 10}); !errors.Is(err, service.ErrSwarmNotFound) {
		t.Fatalf("no existe: %v", err)
	}
	f.PutService(docker.Service{Name: "p_s_api_api"})
	svc, _ := d.Service(ctx, "p_s_api_api")
	at := time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC)
	for i := range 5 {
		f.AppendServiceLogs("p_s_api_api", docker.LogLine{Time: at.Add(time.Duration(i) * time.Second), Stream: docker.StreamStdout, TaskID: "t1", Text: fmt.Sprintf("línea %d", i)})
	}
	f.AppendServiceLogs("p_s_api_api", docker.LogLine{Stream: docker.StreamStdout, Text: "pedazo"})
	read := func(o service.LogOptions) []service.LogLine {
		t.Helper()
		st, err := d.ServiceLogs(ctx, svc.ID, o)
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		var out []service.LogLine
		for {
			l, err := st.Next()
			if errors.Is(err, io.EOF) {
				return out
			}
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, l)
		}
	}
	if got := read(service.LogOptions{Tail: 3}); len(got) != 3 || got[0].Text != "línea 3" || got[0].TaskID != "t1" ||
		got[0].Stream != docker.StreamStdout || !got[0].Time.Equal(at.Add(3*time.Second)) || got[2].Text != "pedazo" || !got[2].Time.IsZero() {
		t.Fatalf("Tail 3: %+v", got)
	}
	// Since es inclusivo; el pedazo (sin hora) lo descarta el fake.
	if got := read(service.LogOptions{Tail: 5000, Since: at.Add(4 * time.Second)}); len(got) < 1 || got[0].Text != "línea 4" {
		t.Fatalf("Since: %+v", got)
	}
	if calls := f.Calls(); !slices.Contains(calls, "ServiceLogs "+svc.ID) {
		t.Fatalf("ServiceLogs con el id: %v", calls)
	}
}

// NodeHostname, ContainerHealth y ContainerStats, para GET …/status; lo que
// no está es ErrSwarmNotFound, y Docker caído es ErrDockerUnavailable.
func TestDockerClusterNodesAndContainers(t *testing.T) {
	f := dockertest.New()
	d := NewDocker(f)
	ctx := context.Background()
	if h, err := d.NodeHostname(ctx, "node-1"); err != nil || h != "manager-1" {
		t.Fatalf("NodeHostname: %q, %v", h, err)
	}
	if _, err := d.NodeHostname(ctx, "node-x"); !errors.Is(err, service.ErrSwarmNotFound) {
		t.Fatalf("nodo que no está: %v", err)
	}
	f.PutContainer(docker.Container{ID: "c1", Name: "p_s_api_api.1.t1", Running: true, Health: "healthy"})
	f.SetStats("c1", docker.Stats{CPUPercent: 3.5, MemoryBytes: 64 << 20, MemoryLimitBytes: 1 << 30})
	if h, err := d.ContainerHealth(ctx, "c1"); err != nil || h != "healthy" {
		t.Fatalf("ContainerHealth: %q, %v", h, err)
	}
	if s, err := d.ContainerStats(ctx, "c1"); err != nil || s.CPUPercent != 3.5 || s.MemoryBytes != 64<<20 {
		t.Fatalf("ContainerStats: %+v, %v", s, err)
	}
	if _, err := d.ContainerHealth(ctx, "c-nada"); !errors.Is(err, service.ErrSwarmNotFound) {
		t.Fatalf("contenedor que no está: %v", err)
	}
	if _, err := d.ContainerStats(ctx, "c-nada"); !errors.Is(err, service.ErrSwarmNotFound) {
		t.Fatalf("stats de uno que no está: %v", err)
	}
	f.SetUnavailable(true)
	if _, err := d.ContainerStats(ctx, "c1"); !errors.Is(err, entity.ErrDockerUnavailable) {
		t.Fatalf("Docker caído: %v", err)
	}
}
