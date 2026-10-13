package dockertest

import (
	"context"
	"errors"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

var _ docker.Engine = (*Fake)(nil)

func TestFakeNetworksAndCaddy(t *testing.T) {
	f := New()
	ctx := context.Background()

	if _, err := f.CreateNetwork(ctx, docker.NetworkSpec{Name: "auditfy_staging", Driver: "overlay", Attachable: true,
		Labels: naming.StageLabels("auditfy", "staging")}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.CreateNetwork(ctx, docker.NetworkSpec{Name: "auditfy_staging", Driver: "overlay"}); !errors.Is(err, docker.ErrConflict) {
		t.Fatalf("red repetida: %v", err)
	}
	n, err := f.InspectNetwork(ctx, "auditfy_staging")
	if err != nil || n.Scope != "swarm" || !n.Attachable || len(n.Subnets) != 1 || n.Labels[naming.LabelStage] != "staging" {
		t.Fatalf("red: %+v %v", n, err)
	}

	// Crear un contenedor exige la imagen en el nodo, como Docker.
	spec := docker.ContainerSpec{
		Name: naming.CaddyContainer, Image: "ghcr.io/cpu-commits/muelle-caddy:v1", Network: naming.PanelNetwork,
		Ports:         []docker.PortBinding{{HostPort: 80, ContainerPort: 80}, {HostPort: 443, ContainerPort: 443}},
		RestartPolicy: "unless-stopped",
	}
	if _, err := f.CreateContainer(ctx, spec); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("sin la imagen: %v", err)
	}
	if err := f.PullImage(ctx, spec.Image, nil); err != nil {
		t.Fatal(err)
	}
	// La red inicial también tiene que existir.
	if _, err := f.CreateContainer(ctx, spec); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("sin la red inicial: %v", err)
	}
	if _, err := f.CreateNetwork(ctx, docker.NetworkSpec{Name: naming.PanelNetwork, Driver: "overlay", Attachable: true}); err != nil {
		t.Fatal(err)
	}
	id, err := f.CreateContainer(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.StartContainer(ctx, naming.CaddyContainer); err != nil {
		t.Fatal(err)
	}
	// Conectar en caliente no cambia el id ni reinicia el contenedor.
	before, _ := f.InspectContainer(ctx, id)
	if err := f.ConnectNetwork(ctx, "auditfy_staging", naming.CaddyContainer, nil); err != nil {
		t.Fatal(err)
	}
	if err := f.ConnectNetwork(ctx, "auditfy_staging", naming.CaddyContainer, nil); !errors.Is(err, docker.ErrConflict) {
		t.Fatalf("conectar dos veces: %v", err)
	}
	after, _ := f.InspectContainer(ctx, naming.CaddyContainer)
	if after.ID != before.ID || !after.StartedAt.Equal(before.StartedAt) || !after.Running ||
		!slices.Equal(after.Networks, []string{"auditfy_staging", naming.PanelNetwork}) {
		t.Fatalf("antes %+v\ndespués %+v", before, after)
	}
	// Una red con contenedores conectados no se borra.
	if err := f.RemoveNetwork(ctx, "auditfy_staging"); !errors.Is(err, docker.ErrConflict) {
		t.Fatalf("borrar red en uso: %v", err)
	}
	if err := f.DisconnectNetwork(ctx, "auditfy_staging", naming.CaddyContainer); err != nil {
		t.Fatal(err)
	}
	if err := f.DisconnectNetwork(ctx, "auditfy_staging", naming.CaddyContainer); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("desconectar dos veces: %v", err)
	}
	if err := f.RemoveNetwork(ctx, "auditfy_staging"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.InspectNetwork(ctx, "auditfy_staging"); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("red borrada: %v", err)
	}
	if !slices.Contains(f.Calls(), "ConnectNetwork auditfy_staging muelle-caddy") {
		t.Fatalf("las llamadas quedan registradas: %v", f.Calls())
	}
}

// Un puerto del host ocupado (el Traefik de Dokploy, §16) hace fallar el
// arranque, como en Docker.
func TestFakeBusyHostPort(t *testing.T) {
	f := New()
	ctx := context.Background()
	f.BusyHostPort(443)
	f.SetImagePresent("caddy:x")
	if _, err := f.CreateContainer(ctx, docker.ContainerSpec{Name: "c", Image: "caddy:x",
		Ports: []docker.PortBinding{{HostPort: 443, ContainerPort: 443}}}); err != nil {
		t.Fatal(err)
	}
	if err := f.StartContainer(ctx, "c"); err == nil {
		t.Fatal("arrancar con un puerto ocupado debió fallar")
	}
	if c, _ := f.InspectContainer(ctx, "c"); c.Running {
		t.Fatal("no debe quedar corriendo")
	}
	// Un contenedor corriendo no se borra sin force.
	f.FreeHostPort(443)
	if err := f.StartContainer(ctx, "c"); err != nil {
		t.Fatal(err)
	}
	if err := f.RemoveContainer(ctx, "c", false); !errors.Is(err, docker.ErrConflict) {
		t.Fatalf("borrar corriendo sin force: %v", err)
	}
	if err := f.RemoveContainer(ctx, "c", true); err != nil {
		t.Fatal(err)
	}
}

func TestFakeStacksServicesAndTasks(t *testing.T) {
	f := New()
	ctx := context.Background()
	stack := naming.Stack("auditfy", "staging", "api")
	svcName := naming.Service("auditfy", "staging", "api", "api")
	f.OnDeployStack = func(d docker.StackDeploy) error {
		// El test decide qué crea el deploy: acá, un servicio que converge.
		one := uint64(1)
		f.PutStackService(d.Name, docker.Service{Name: svcName, Labels: naming.AppLabels("auditfy", "staging", "api"), Replicas: &one,
			RunningTasks: 1, DesiredTasks: 1, Update: &docker.UpdateStatus{State: docker.UpdateCompleted}},
			docker.Task{ID: "t1", Slot: 1, State: docker.TaskRunning, DesiredState: docker.TaskRunning, ContainerID: "c1"})
		return nil
	}
	out, err := f.DeployStack(ctx, docker.StackDeploy{Name: stack, Compose: []byte("services: {}\n"),
		Registries: []docker.RegistryAuth{{Host: "ghcr.io", Username: "bot", Password: "x"}}})
	if err != nil || out == "" {
		t.Fatalf("DeployStack: %q %v", out, err)
	}
	if d := f.StackDeploys(); len(d) != 1 || d[0].Name != stack || string(d[0].Compose) != "services: {}\n" || d[0].Registries[0].Host != "ghcr.io" {
		t.Fatalf("deploys registrados: %+v", d)
	}
	list, err := f.ListServices(ctx, naming.AppLabels("auditfy", "staging", "api"))
	if err != nil || len(list) != 1 || list[0].Name != svcName || list[0].ID == "" || list[0].Labels[naming.LabelStackNamespace] != stack {
		t.Fatalf("servicios: %+v %v", list, err)
	}
	if other, _ := f.ListServices(ctx, naming.AppLabels("auditfy", "staging", "otra")); len(other) != 0 {
		t.Fatalf("filtro por etiquetas: %+v", other)
	}
	tasks, err := f.ListTasks(ctx, docker.TaskFilter{Service: svcName, DesiredState: docker.TaskRunning})
	if err != nil || len(tasks) != 1 || tasks[0].ServiceID != list[0].ID {
		t.Fatalf("tareas: %+v %v", tasks, err)
	}

	// Un deploy que falla devuelve la salida y ErrCommand.
	f.FailNext("DeployStack", errors.New("network auditfy_staging not found"))
	if out, err := f.DeployStack(ctx, docker.StackDeploy{Name: stack}); !errors.Is(err, docker.ErrCommand) || out == "" {
		t.Fatalf("deploy fallido: %q %v", out, err)
	}

	if _, err := f.RemoveStack(ctx, stack); err != nil {
		t.Fatal(err)
	}
	if list, _ := f.ListServices(ctx, nil); len(list) != 0 {
		t.Fatalf("stack rm deja servicios: %+v", list)
	}
	if !slices.Equal(f.RemovedStacks(), []string{stack}) {
		t.Fatalf("RemovedStacks = %v", f.RemovedStacks())
	}
}

func TestFakeDistribution(t *testing.T) {
	f := New()
	ctx := context.Background()
	f.SetDigest("ghcr.io/x/api:sha-1", "sha256:aaa")
	f.DenyImage("ghcr.io/x/privada:1")
	auth := &docker.RegistryAuth{Host: "ghcr.io", Username: "bot", Password: "x"}
	if d, err := f.DistributionDigest(ctx, "ghcr.io/x/api:sha-1", auth); err != nil || d != "sha256:aaa" {
		t.Fatalf("%q %v", d, err)
	}
	if _, err := f.DistributionDigest(ctx, "ghcr.io/x/no:1", nil); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("inexistente: %v", err)
	}
	if _, err := f.DistributionDigest(ctx, "ghcr.io/x/privada:1", nil); !errors.Is(err, docker.ErrDenied) {
		t.Fatalf("sin permiso: %v", err)
	}
	calls := f.DistributionCalls()
	if len(calls) != 3 || calls[0].Auth == nil || calls[0].Auth.Username != "bot" || calls[1].Auth != nil {
		t.Fatalf("llamadas: %+v", calls)
	}
}

func TestFakeLogs(t *testing.T) {
	f := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.PutService(docker.Service{Name: "svc"})
	t0 := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	f.AppendServiceLogs("svc",
		docker.LogLine{Time: t0, Text: "uno", Stream: docker.StreamStdout, TaskID: "t1"},
		docker.LogLine{Time: t0.Add(time.Second), Text: "dos", Stream: docker.StreamStderr, TaskID: "t1"},
		docker.LogLine{Time: t0.Add(2 * time.Second), Text: "tres", Stream: docker.StreamStdout, TaskID: "t1"},
	)

	s, err := f.ServiceLogs(ctx, "svc", docker.LogOptions{Tail: 2})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for {
		l, err := s.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, l.Text)
	}
	s.Close()
	if !slices.Equal(got, []string{"dos", "tres"}) {
		t.Fatalf("tail 2: %v", got)
	}

	// Con Follow, las líneas nuevas llegan hasta que se cancela el ctx.
	fctx, fcancel := context.WithCancel(ctx)
	s, err = f.ServiceLogs(fctx, "svc", docker.LogOptions{Tail: 0, Follow: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	next := make(chan docker.LogLine)
	go func() {
		for {
			l, err := s.Next()
			if err != nil {
				close(next)
				return
			}
			next <- l
		}
	}()
	f.AppendServiceLogs("svc", docker.LogLine{Time: t0.Add(3 * time.Second), Text: "cuatro"})
	select {
	case l := <-next:
		if l.Text != "cuatro" {
			t.Fatalf("follow: %+v", l)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("follow no entregó la línea nueva")
	}
	fcancel()
	select {
	case _, ok := <-next:
		if ok {
			t.Fatal("después de cancelar no deben llegar líneas")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelar no cortó el stream")
	}

	if _, err := f.ServiceLogs(ctx, "no-existe", docker.LogOptions{}); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("servicio inexistente: %v", err)
	}
}

func TestFakeUnavailableAndFailNext(t *testing.T) {
	f := New()
	ctx := context.Background()
	f.SetUnavailable(true)
	if err := f.Ping(ctx); !errors.Is(err, docker.ErrUnavailable) {
		t.Fatalf("Ping: %v", err)
	}
	if _, err := f.ListServices(ctx, nil); !errors.Is(err, docker.ErrUnavailable) {
		t.Fatalf("ListServices: %v", err)
	}
	f.SetUnavailable(false)
	boom := errors.New("boom")
	f.FailNext("CreateNetwork", boom)
	if _, err := f.CreateNetwork(ctx, docker.NetworkSpec{Name: "x"}); !errors.Is(err, boom) {
		t.Fatalf("FailNext: %v", err)
	}
	if _, err := f.CreateNetwork(ctx, docker.NetworkSpec{Name: "x"}); err != nil {
		t.Fatalf("FailNext es de una sola vez: %v", err)
	}
}

func TestFakeInfoSecretsVolumes(t *testing.T) {
	f := New()
	ctx := context.Background()
	info, err := f.Info(ctx)
	if err != nil || info.NodeID == "" || info.SwarmState != docker.SwarmActive || !info.IsManager {
		t.Fatalf("Info: %+v %v", info, err)
	}
	if n, err := f.Node(ctx, info.NodeID); err != nil || n.Hostname == "" || n.Role != "manager" {
		t.Fatalf("Node: %+v %v", n, err)
	}
	if _, err := f.CreateSecret(ctx, naming.MasterKeySecret, []byte("k"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.CreateSecret(ctx, naming.MasterKeySecret, []byte("k"), nil); !errors.Is(err, docker.ErrConflict) {
		t.Fatalf("secreto repetido: %v", err)
	}
	if data, ok := f.SecretData(naming.MasterKeySecret); !ok || string(data) != "k" {
		t.Fatal("SecretData")
	}
	f.PutVolume(docker.Volume{Name: "auditfy_staging_api_data", Labels: map[string]string{naming.LabelStackNamespace: "auditfy_staging_api"}})
	if v, _ := f.ListVolumes(ctx, map[string]string{naming.LabelStackNamespace: "auditfy_staging_api"}); len(v) != 1 {
		t.Fatalf("volúmenes: %+v", v)
	}
	if err := f.RemoveVolume(ctx, "auditfy_staging_api_data", false); err != nil {
		t.Fatal(err)
	}
}

// TaskFilter.Labels son las etiquetas del servicio, como en el cliente real.
func TestFakeListTasksByServiceLabels(t *testing.T) {
	f := New()
	ctx := context.Background()
	f.PutService(docker.Service{Name: "auditfy_staging_api_api", Labels: naming.AppLabels("auditfy", "staging", "api")},
		docker.Task{ID: "t1", State: docker.TaskRunning, DesiredState: docker.TaskRunning})
	f.PutService(docker.Service{Name: "auditfy_staging_web_web", Labels: naming.AppLabels("auditfy", "staging", "web")},
		docker.Task{ID: "t2", State: docker.TaskRunning, DesiredState: docker.TaskRunning})
	got, err := f.ListTasks(ctx, docker.TaskFilter{Labels: naming.AppLabels("auditfy", "staging", "api")})
	if err != nil || len(got) != 1 || got[0].ID != "t1" {
		t.Fatalf("tareas de api: %+v %v", got, err)
	}
	got, _ = f.ListTasks(ctx, docker.TaskFilter{Labels: naming.StageLabels("auditfy", "staging")})
	if len(got) != 2 {
		t.Fatalf("tareas del stage: %+v", got)
	}
}

// Sin Swarm activo, lo que es de Swarm falla como en el daemon real (503 →
// ErrUnavailable; el CLI, ErrCommand), hasta que install llama a InitSwarm.
func TestFakeWithoutSwarm(t *testing.T) {
	f := New()
	ctx := context.Background()
	info, _ := f.Info(ctx)
	info.SwarmState, info.NodeID, info.IsManager = docker.SwarmInactive, "", false
	f.SetInfo(info)

	if _, err := f.ListServices(ctx, nil); !errors.Is(err, docker.ErrUnavailable) {
		t.Fatalf("ListServices sin swarm: %v", err)
	}
	if _, err := f.CreateNetwork(ctx, docker.NetworkSpec{Name: naming.PanelNetwork, Driver: "overlay", Attachable: true}); !errors.Is(err, docker.ErrUnavailable) {
		t.Fatalf("red overlay sin swarm: %v", err)
	}
	if _, err := f.CreateSecret(ctx, naming.MasterKeySecret, []byte("k"), nil); !errors.Is(err, docker.ErrUnavailable) {
		t.Fatalf("secreto sin swarm: %v", err)
	}
	if _, err := f.DeployStack(ctx, docker.StackDeploy{Name: naming.MuelleStack}); !errors.Is(err, docker.ErrCommand) {
		t.Fatalf("stack sin swarm: %v", err)
	}
	// Una red bridge no necesita Swarm.
	if _, err := f.CreateNetwork(ctx, docker.NetworkSpec{Name: "local", Driver: "bridge"}); err != nil {
		t.Fatal(err)
	}

	id, err := f.InitSwarm(ctx, "")
	if err != nil || id == "" {
		t.Fatalf("InitSwarm: %q %v", id, err)
	}
	if _, err := f.InitSwarm(ctx, ""); !errors.Is(err, docker.ErrConflict) {
		t.Fatalf("InitSwarm dos veces: %v", err)
	}
	if _, err := f.CreateNetwork(ctx, docker.NetworkSpec{Name: naming.PanelNetwork, Driver: "overlay", Attachable: true}); err != nil {
		t.Fatalf("con swarm: %v", err)
	}
}

// Lo que devuelve el fake es una copia: cambiarlo no cambia su estado.
func TestFakeReturnsCopies(t *testing.T) {
	f := New()
	ctx := context.Background()
	f.PutService(docker.Service{Name: "svc", Labels: map[string]string{"a": "1"}})
	list, _ := f.ListServices(ctx, nil)
	list[0].Labels["a"] = "cambiado"
	got, _ := f.InspectService(ctx, "svc")
	if got.Labels["a"] != "1" {
		t.Fatalf("ListServices devolvió el mapa interno: %v", got.Labels)
	}
}

func TestFakeInspectImage(t *testing.T) {
	f := New()
	ctx := context.Background()
	if _, err := f.InspectImage(ctx, "caddy:x"); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("imagen que falta: %v", err)
	}
	if err := f.PullImage(ctx, "caddy:x", nil); err != nil {
		t.Fatal(err)
	}
	if img, err := f.InspectImage(ctx, "caddy:x"); err != nil || img.ID == "" || img.RepoTags[0] != "caddy:x" {
		t.Fatalf("imagen bajada: %+v %v", img, err)
	}
}
