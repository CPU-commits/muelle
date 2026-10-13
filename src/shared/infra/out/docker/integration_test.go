//go:build integration

// Tests contra el Docker real (un Swarm activo en el que este nodo es
// manager): go test -tags integration ./src/shared/infra/out/docker/
//
// Todo lo que crean empieza con "ctr23" y lleva la etiqueta muelle.test=ctr23;
// se borra al terminar, también si el test falla. No tocan nada más. Usan
// imágenes que ya estén en el nodo (alpine/socat y caddy:2-alpine): si
// faltan, el test se salta en vez de bajarlas.
package docker_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

const (
	prefix        = "ctr23"
	serviceImage  = "alpine/socat:latest"
	containerImg  = "caddy:2-alpine"
	testLabel     = "muelle.test"
	waitConverge  = 90 * time.Second
	waitReleasing = 60 * time.Second
)

func uniqueName(t *testing.T) string {
	t.Helper()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return prefix + "_" + hex.EncodeToString(b)
}

// realClient arma el cliente contra el Docker del entorno y se salta el test
// si no hay un Swarm activo con este nodo como manager.
func realClient(t *testing.T) *docker.Client {
	t.Helper()
	c, err := docker.New(docker.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	ctx := context.Background()
	info, err := c.Info(ctx)
	if err != nil {
		t.Skipf("sin Docker: %v", err)
	}
	if info.SwarmState != docker.SwarmActive || !info.IsManager {
		t.Skipf("el nodo no es manager de un Swarm activo (%s)", info.SwarmState)
	}
	return c
}

func requireLocalImage(t *testing.T, c *docker.Client, ref string) {
	t.Helper()
	// Crear un contenedor con una imagen que falta da ErrNotFound; acá basta
	// con saber si docker la tiene, sin bajarla.
	if _, err := c.InspectImage(context.Background(), ref); err != nil {
		t.Skipf("falta la imagen %s en el nodo (%v); bájala con docker pull", ref, err)
	}
}

// eventually repite check cada 500 ms hasta que devuelve true o pasa timeout.
func eventually(t *testing.T, timeout time.Duration, what string, check func() (bool, error)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ok, err := check()
		if ok {
			return
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("%s: no pasó en %s (último error: %v)", what, timeout, lastErr)
}

// removeNetworkEventually borra la red, reintentando mientras siga en uso
// (Swarm suelta los endpoints de un stack recién borrado unos segundos después).
func removeNetworkEventually(ctx context.Context, c *docker.Client, name string) error {
	deadline := time.Now().Add(waitReleasing)
	for {
		err := c.RemoveNetwork(ctx, name)
		switch {
		case err == nil, errors.Is(err, docker.ErrNotFound):
			return nil
		case !errors.Is(err, docker.ErrConflict) || time.Now().After(deadline):
			return err
		}
		time.Sleep(time.Second)
	}
}

// Una red de stage (overlay attachable) y un servicio chico desplegado con
// docker stack deploy: converge, se ven sus tareas y su log, y se borra todo.
func TestIntegrationNetworkAndStack(t *testing.T) {
	c := realClient(t)
	requireLocalImage(t, c, serviceImage)
	ctx := context.Background()
	name := uniqueName(t)
	netName, stack := name+"_net", name
	labels := map[string]string{testLabel: prefix}

	t.Cleanup(func() {
		ctx := context.Background()
		if _, err := c.RemoveStack(ctx, stack); err != nil {
			t.Errorf("limpieza: stack rm %s: %v", stack, err)
		}
		if err := removeNetworkEventually(ctx, c, netName); err != nil {
			t.Errorf("limpieza: red %s: %v", netName, err)
		}
	})

	id, err := c.CreateNetwork(ctx, docker.NetworkSpec{Name: netName, Driver: "overlay", Attachable: true, Labels: labels})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateNetwork(ctx, docker.NetworkSpec{Name: netName, Driver: "overlay", Attachable: true}); !errors.Is(err, docker.ErrConflict) {
		t.Fatalf("crear la misma red otra vez: %v", err)
	}
	// Recién creada, la red puede venir sin Driver ni Subnets: el allocator de
	// Swarm la completa en unos milisegundos.
	var n docker.Network
	eventually(t, 10*time.Second, "Swarm completa la red", func() (bool, error) {
		var err error
		n, err = c.InspectNetwork(ctx, netName)
		return err == nil && n.Driver != "" && len(n.Subnets) > 0, err
	})
	if n.ID != id || n.Driver != "overlay" || n.Scope != "swarm" || !n.Attachable || n.Labels[testLabel] != prefix || len(n.Subnets) == 0 {
		t.Fatalf("red: %+v", n)
	}
	for _, s := range n.Subnets {
		// El pool por defecto de Docker es privado (10.0.0.0/8); ingress lo verifica (§8).
		if p, err := netip.ParsePrefix(s); err != nil || !p.Addr().IsPrivate() {
			t.Errorf("subred %q no es privada", s)
		}
	}
	if list, err := c.ListNetworks(ctx, labels); err != nil || !slices.ContainsFunc(list, func(n docker.Network) bool { return n.Name == netName }) {
		t.Fatalf("ListNetworks por etiqueta: %+v %v", list, err)
	}

	compose := `services:
  app:
    image: ` + serviceImage + `
    entrypoint: ["sh", "-c", "echo hola-` + prefix + `; exec sleep 600"]
    networks: [stage]
    deploy:
      labels:
        ` + testLabel + `: ` + prefix + `
      resources:
        limits:
          memory: 32M
networks:
  stage:
    external: true
    name: ` + netName + `
`
	out, err := c.DeployStack(ctx, docker.StackDeploy{Name: stack, Compose: []byte(compose)})
	if err != nil {
		t.Fatalf("stack deploy: %v\n%s", err, out)
	}

	svcName := stack + "_app"
	var svc docker.Service
	eventually(t, waitConverge, "el servicio converge", func() (bool, error) {
		list, err := c.ListServices(ctx, labels)
		if err != nil || len(list) != 1 {
			return false, err
		}
		svc = list[0]
		return svc.RunningTasks == 1, nil
	})
	if svc.Name != svcName || svc.Replicas == nil || *svc.Replicas != 1 || svc.DesiredTasks != 1 || !strings.HasPrefix(svc.Image, serviceImage) {
		t.Fatalf("servicio: %+v", svc)
	}
	got, err := c.InspectService(ctx, svcName)
	if err != nil || got.ID != svc.ID || got.RunningTasks != 1 {
		t.Fatalf("InspectService: %+v %v", got, err)
	}

	tasks, err := c.ListTasks(ctx, docker.TaskFilter{Labels: labels, DesiredState: docker.TaskRunning})
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tareas: %+v %v", tasks, err)
	}
	task := tasks[0]
	if task.ServiceID != svc.ID || task.State != docker.TaskRunning || task.ContainerID == "" || task.Slot != 1 {
		t.Fatalf("tarea: %+v", task)
	}

	eventually(t, 30*time.Second, "el log del servicio trae la línea", func() (bool, error) {
		s, err := c.ServiceLogs(ctx, svcName, docker.LogOptions{Tail: -1})
		if err != nil {
			return false, err
		}
		defer s.Close()
		for {
			l, err := s.Next()
			if errors.Is(err, io.EOF) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			if l.Text == "hola-"+prefix {
				if l.TaskID != task.ID || l.Stream != docker.StreamStdout || l.Time.IsZero() {
					t.Errorf("línea: %+v", l)
				}
				return true, nil
			}
		}
	})

	st, err := c.ContainerStats(ctx, task.ContainerID)
	if err != nil {
		t.Fatal(err)
	}
	if st.MemoryBytes == 0 || st.MemoryLimitBytes != 32<<20 {
		t.Fatalf("stats: %+v", st)
	}

	if out, err := c.RemoveStack(ctx, stack); err != nil {
		t.Fatalf("stack rm: %v\n%s", err, out)
	}
	eventually(t, waitReleasing, "stack rm borra el servicio", func() (bool, error) {
		list, err := c.ListServices(ctx, labels)
		return err == nil && len(list) == 0, err
	})
	if err := removeNetworkEventually(ctx, c, netName); err != nil {
		t.Fatal(err)
	}
	// El nodo la sigue viendo unos segundos después de borrarla.
	eventually(t, 30*time.Second, "la red borrada desaparece", func() (bool, error) {
		_, err := c.InspectNetwork(ctx, netName)
		return errors.Is(err, docker.ErrNotFound), err
	})
	// Borrarla otra vez es ErrNotFound, no un error interno.
	if err := c.RemoveNetwork(ctx, netName); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("borrar una red que ya no existe: %v", err)
	}
	// docker stack rm de un stack que no existe no es un error.
	if _, err := c.RemoveStack(ctx, stack); err != nil {
		t.Fatalf("stack rm de un stack que ya no existe: %v", err)
	}
}

// Un contenedor standalone (como muelle-caddy) se conecta en caliente a una
// red attachable: conserva su id y no se reinicia (§3.1).
func TestIntegrationContainerHotConnect(t *testing.T) {
	c := realClient(t)
	requireLocalImage(t, c, containerImg)
	ctx := context.Background()
	name := uniqueName(t)
	ctrName := strings.ReplaceAll(name, "_", "-") + "-ctr"
	first, second := name+"_a", name+"_b"
	labels := map[string]string{testLabel: prefix}

	t.Cleanup(func() {
		ctx := context.Background()
		if err := c.RemoveContainer(ctx, ctrName, true); err != nil && !errors.Is(err, docker.ErrNotFound) {
			t.Errorf("limpieza: contenedor %s: %v", ctrName, err)
		}
		for _, n := range []string{first, second} {
			if err := removeNetworkEventually(ctx, c, n); err != nil {
				t.Errorf("limpieza: red %s: %v", n, err)
			}
		}
	})

	for _, n := range []string{first, second} {
		if _, err := c.CreateNetwork(ctx, docker.NetworkSpec{Name: n, Driver: "overlay", Attachable: true, Labels: labels}); err != nil {
			t.Fatal(err)
		}
		// Conectar un contenedor a una red que Swarm todavía no completó puede
		// quedar esperando la tarea de la conexión: se espera a que tenga subred.
		eventually(t, 10*time.Second, "Swarm completa la red "+n, func() (bool, error) {
			got, err := c.InspectNetwork(ctx, n)
			return err == nil && len(got.Subnets) > 0, err
		})
	}
	id, err := c.CreateContainer(ctx, docker.ContainerSpec{
		Name: ctrName, Image: containerImg, Cmd: []string{"sleep", "600"}, Labels: labels,
		Network: first, RestartPolicy: "unless-stopped",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateContainer(ctx, docker.ContainerSpec{Name: ctrName, Image: containerImg}); !errors.Is(err, docker.ErrConflict) {
		t.Fatalf("nombre repetido: %v", err)
	}
	if err := c.StartContainer(ctx, ctrName); err != nil {
		t.Fatal(err)
	}
	before, err := c.InspectContainer(ctx, ctrName)
	if err != nil {
		t.Fatal(err)
	}
	if before.ID != id || !before.Running || before.RestartPolicy != "unless-stopped" || !slices.Equal(before.Networks, []string{first}) {
		t.Fatalf("contenedor: %+v", before)
	}

	if err := c.ConnectNetwork(ctx, second, ctrName, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.ConnectNetwork(ctx, second, ctrName, nil); !errors.Is(err, docker.ErrConflict) {
		t.Fatalf("conectar dos veces: %v", err)
	}
	after, err := c.InspectContainer(ctx, ctrName)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{first, second}
	slices.Sort(want)
	if after.ID != before.ID || !after.StartedAt.Equal(before.StartedAt) || !after.Running || !slices.Equal(after.Networks, want) {
		t.Fatalf("conectar en caliente lo cambió:\nantes   %+v\ndespués %+v", before, after)
	}
	n, err := c.InspectNetwork(ctx, second)
	if err != nil || !slices.Contains(n.Containers, ctrName) {
		t.Fatalf("la red no lista el contenedor: %+v %v", n, err)
	}
	if err := c.RemoveNetwork(ctx, second); !errors.Is(err, docker.ErrConflict) {
		t.Fatalf("borrar una red en uso: %v", err)
	}

	if err := c.DisconnectNetwork(ctx, second, ctrName); err != nil {
		t.Fatal(err)
	}
	if err := c.DisconnectNetwork(ctx, second, ctrName); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("desconectar dos veces: %v", err)
	}
	if err := c.RemoveContainer(ctx, ctrName, false); !errors.Is(err, docker.ErrConflict) {
		t.Fatalf("borrar un contenedor corriendo sin force: %v", err)
	}
	if err := c.StopContainer(ctx, ctrName, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveContainer(ctx, ctrName, false); err != nil {
		t.Fatal(err)
	}
	if _, err := c.InspectContainer(ctx, ctrName); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("contenedor borrado: %v", err)
	}
}

// Un puerto del host ocupado hace fallar el arranque con ErrPortInUse, sea
// un proceso del host que lo escucha o un contenedor que ya lo publica (la
// prueba de puertos de install y la recreación de muelle-caddy, §3.3 y §16).
// Ojo: Docker 29 arranca sin los puertos un contenedor cuyo arranque falló
// así, si se lo vuelve a arrancar; por eso cada intento usa uno nuevo.
func TestIntegrationPortInUse(t *testing.T) {
	c := realClient(t)
	requireLocalImage(t, c, containerImg)
	ctx := context.Background()
	base := strings.ReplaceAll(uniqueName(t), "_", "-")
	names := []string{base + "-a", base + "-b", base + "-c"}
	labels := map[string]string{testLabel: prefix}
	t.Cleanup(func() {
		for _, n := range names {
			if err := c.RemoveContainer(context.Background(), n, true); err != nil && !errors.Is(err, docker.ErrNotFound) {
				t.Errorf("limpieza: contenedor %s: %v", n, err)
			}
		}
	})

	// Un puerto libre del host, que un proceso escucha.
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	t.Cleanup(func() { ln.Close() })
	start := func(name string) error {
		t.Helper()
		if _, err := c.CreateContainer(ctx, docker.ContainerSpec{Name: name, Image: containerImg, Cmd: []string{"sleep", "600"},
			Labels: labels, Ports: []docker.PortBinding{{HostIP: "0.0.0.0", HostPort: port, ContainerPort: 8080}}}); err != nil {
			t.Fatal(err)
		}
		return c.StartContainer(ctx, name)
	}
	if err := start(names[0]); !errors.Is(err, docker.ErrPortInUse) {
		t.Fatalf("puerto que escucha un proceso del host: %v, esperaba ErrPortInUse", err)
	}

	// Lo libera el proceso y lo toma un contenedor; otro choca con él.
	ln.Close()
	if err := start(names[1]); err != nil {
		t.Fatal(err)
	}
	got, err := c.InspectContainer(ctx, names[1])
	if err != nil || len(got.Ports) != 1 || got.Ports[0].HostPort != port || len(got.PublishedPorts) == 0 || got.PublishedPorts[0].HostPort != port {
		t.Fatalf("el contenedor que tomó el puerto: %+v %v", got, err)
	}
	if err := start(names[2]); !errors.Is(err, docker.ErrPortInUse) {
		t.Fatalf("puerto que publica otro contenedor: %v, esperaba ErrPortInUse", err)
	}
	// Rearrancarlo nunca le da el puerto: o falla igual, o (Docker 29) corre
	// sin publicarlo, y eso se ve en PublishedPorts.
	err = c.StartContainer(ctx, names[2])
	if !errors.Is(err, docker.ErrPortInUse) {
		if err != nil {
			t.Fatal(err)
		}
		got, err := c.InspectContainer(ctx, names[2])
		if err != nil || !got.Running || len(got.Ports) != 1 || len(got.PublishedPorts) != 0 {
			t.Fatalf("rearrancado tras ErrPortInUse: %+v %v", got, err)
		}
	}
}
