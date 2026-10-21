package dockertest

import (
	"context"
	"errors"
	"testing"

	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

// PutContainer deja un contenedor como los que crea Swarm para las tareas
// (sin CreateContainer, que exige la imagen): InspectContainer y
// ContainerStats lo encuentran por nombre y por id.
func TestFakePutContainer(t *testing.T) {
	f := New()
	ctx := context.Background()
	f.PutContainer(docker.Container{ID: "ctr-web-1", Name: "auditfy_staging_api_api.1.abc", Running: true, Health: "healthy",
		Labels: map[string]string{"x": "1"}})
	// SetStats por nombre: ContainerStats lo encuentra también por id.
	f.SetStats("auditfy_staging_api_api.1.abc", docker.Stats{CPUPercent: 1.5, MemoryBytes: 1 << 20})

	for _, ref := range []string{"ctr-web-1", "auditfy_staging_api_api.1.abc"} {
		c, err := f.InspectContainer(ctx, ref)
		if err != nil || c.ID != "ctr-web-1" || !c.Running || c.Health != "healthy" || c.Labels["x"] != "1" {
			t.Fatalf("InspectContainer(%s): %+v %v", ref, c, err)
		}
		s, err := f.ContainerStats(ctx, ref)
		if err != nil || s.CPUPercent != 1.5 || s.MemoryBytes != 1<<20 {
			t.Fatalf("ContainerStats(%s): %+v %v", ref, s, err)
		}
	}

	// Reemplazarlo por nombre cambia su health; sin ID, se le asigna uno.
	f.PutContainer(docker.Container{ID: "ctr-web-1", Name: "auditfy_staging_api_api.1.abc", Running: true, Health: "unhealthy"})
	if c, _ := f.InspectContainer(ctx, "ctr-web-1"); c.Health != "unhealthy" {
		t.Fatalf("después de reemplazarlo: %+v", c)
	}
	f.PutContainer(docker.Container{Name: "sin-id"})
	c, err := f.InspectContainer(ctx, "sin-id")
	if err != nil || c.ID == "" {
		t.Fatalf("sin ID: %+v %v", c, err)
	}
	// Sin SetStats, la muestra es cero; un contenedor que no existe, ErrNotFound.
	if s, err := f.ContainerStats(ctx, c.ID); err != nil || s != (docker.Stats{}) {
		t.Fatalf("sin stats: %+v %v", s, err)
	}
	if _, err := f.ContainerStats(ctx, "no-existe"); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("no existe: %v", err)
	}
	// Lo que devuelve es una copia.
	c.Labels = map[string]string{"y": "2"}
	if again, _ := f.InspectContainer(ctx, "sin-id"); again.Labels["y"] != "" {
		t.Fatal("InspectContainer devolvió el contenedor guardado, no una copia")
	}
}
