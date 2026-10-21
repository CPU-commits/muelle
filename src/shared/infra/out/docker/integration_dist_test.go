//go:build integration

// Tests de la Fase 5 (distribución) contra el Docker real:
// go test -tags integration -run '^TestIntegrationDist' ./src/shared/infra/out/docker/
//
// Usan los helpers de integration_test.go, pero con su propio prefijo: todo
// lo que crean empieza con "it5" y lleva la etiqueta muelle.test=it5; se borra
// al terminar, también si el test falla. No tocan nada más.
package docker_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

const distPrefix = "it5"

// distName es un nombre único de esta línea: "it5" y 8 hex.
func distName(t *testing.T) string {
	t.Helper()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return distPrefix + hex.EncodeToString(b)
}

// ForceUpdateService reinicia la tarea de un servicio de un stack sin cambiar
// su spec, y el contenedor de la tarea se encuentra por su hostname (el id
// corto) con la etiqueta del servicio, como hace muelle settings
// reset-panel-ips dentro de su propio contenedor (contrato §5.4).
func TestIntegrationDistForceUpdate(t *testing.T) {
	c := realClient(t)
	requireLocalImage(t, c, serviceImage)
	ctx := context.Background()
	stack := distName(t)
	svcName := stack + "_app"

	// Los contenedores de las tareas: Swarm los borra después de stack rm, y
	// la limpieza espera a que no quede ninguno.
	var containers []string
	t.Cleanup(func() {
		ctx := context.Background()
		if out, err := c.RemoveStack(ctx, stack); err != nil {
			t.Errorf("limpieza: stack rm %s: %v\n%s", stack, err, out)
		}
		gone := func(what string, check func() (bool, error)) {
			deadline := time.Now().Add(waitReleasing)
			for {
				ok, err := check()
				if ok {
					return
				}
				if time.Now().After(deadline) {
					t.Errorf("limpieza: %s sigue ahí (%v)", what, err)
					return
				}
				time.Sleep(time.Second)
			}
		}
		gone("el servicio "+svcName, func() (bool, error) {
			list, err := c.ListServices(ctx, map[string]string{naming.LabelStackNamespace: stack})
			return err == nil && len(list) == 0, err
		})
		for _, id := range containers {
			gone("el contenedor "+id, func() (bool, error) {
				_, err := c.InspectContainer(ctx, id)
				return errors.Is(err, docker.ErrNotFound), err
			})
		}
		if err := removeNetworkEventually(ctx, c, stack+"_default"); err != nil {
			t.Errorf("limpieza: red %s_default: %v", stack, err)
		}
		gone("la red "+stack+"_default", func() (bool, error) {
			_, err := c.InspectNetwork(ctx, stack+"_default")
			return errors.Is(err, docker.ErrNotFound), err
		})
	})

	compose := `services:
  app:
    image: ` + serviceImage + `
    entrypoint: ["sh", "-c", "trap 'exit 0' TERM; sleep 600 & wait"]
    deploy:
      labels:
        ` + testLabel + `: ` + distPrefix + `
      resources:
        limits:
          memory: 32M
`
	if out, err := c.DeployStack(ctx, docker.StackDeploy{Name: stack, Compose: []byte(compose)}); err != nil {
		t.Fatalf("stack deploy: %v\n%s", err, out)
	}

	running := func() (docker.Task, bool, error) {
		tasks, err := c.ListTasks(ctx, docker.TaskFilter{Service: svcName, DesiredState: docker.TaskRunning})
		if err != nil {
			return docker.Task{}, false, err
		}
		for _, task := range tasks {
			if task.State == docker.TaskRunning && task.ContainerID != "" {
				if !slices.Contains(containers, task.ContainerID) {
					containers = append(containers, task.ContainerID)
				}
				return task, true, nil
			}
		}
		return docker.Task{}, false, nil
	}
	var first docker.Task
	eventually(t, waitConverge, "la tarea corre", func() (bool, error) {
		var ok bool
		var err error
		first, ok, err = running()
		return ok, err
	})

	// Dentro del contenedor, os.Hostname es el id corto (el stack no pone
	// hostname), y el daemon lo encuentra igual que por el id entero.
	ctr, err := c.InspectContainer(ctx, first.ContainerID[:12])
	if err != nil {
		t.Fatal(err)
	}
	if got := ctr.Labels[naming.LabelSwarmService]; got != svcName {
		t.Fatalf("etiqueta %s=%q, esperaba %q", naming.LabelSwarmService, got, svcName)
	}

	before, err := c.InspectService(ctx, svcName)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ForceUpdateService(ctx, svcName); err != nil {
		t.Fatal(err)
	}
	after, err := c.InspectService(ctx, svcName)
	if err != nil {
		t.Fatal(err)
	}
	if after.Version <= before.Version || after.Image != before.Image || after.Labels[testLabel] != distPrefix {
		t.Fatalf("antes %+v\ndespués %+v", before, after)
	}
	eventually(t, waitConverge, "una tarea nueva corre", func() (bool, error) {
		task, ok, err := running()
		return ok && task.ID != first.ID, err
	})

	if err := c.ForceUpdateService(ctx, distPrefix+"-no-existe"); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("servicio que no existe: %v, esperaba ErrNotFound", err)
	}
}
