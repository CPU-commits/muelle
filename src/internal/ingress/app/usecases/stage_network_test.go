package usecases

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/ingress/app/dto"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	outservice "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
)

var staging = dto.StageRef{Project: "auditfy", Stage: "staging"}

type fixture struct {
	docker *dockertest.Fake
	ensure *EnsureStageNetwork
	remove *RemoveStageNetwork
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := dockertest.New()
	nets := outservice.NewDockerNetworks(f)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &fixture{docker: f, ensure: NewEnsureStageNetwork(nets, log), remove: NewRemoveStageNetwork(nets, log)}
}

// withCaddy crea muelle-caddy en muelle_panel, como lo deja ingress (Fase 3).
func (f *fixture) withCaddy(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.docker.CreateNetwork(ctx, docker.NetworkSpec{Name: naming.PanelNetwork, Driver: "overlay", Attachable: true}); err != nil {
		t.Fatal(err)
	}
	f.docker.SetImagePresent("caddy:test")
	if _, err := f.docker.CreateContainer(ctx, docker.ContainerSpec{Name: naming.CaddyContainer, Image: "caddy:test", Network: naming.PanelNetwork}); err != nil {
		t.Fatal(err)
	}
	if err := f.docker.StartContainer(ctx, naming.CaddyContainer); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) caddyNetworks(t *testing.T) []string {
	t.Helper()
	c, err := f.docker.InspectContainer(context.Background(), naming.CaddyContainer)
	if err != nil {
		t.Fatal(err)
	}
	return c.Networks
}

// §3.2: la red del stage es overlay y attachable, con las etiquetas del stage,
// y muelle-caddy se conecta en caliente (sin recrearlo).
func TestEnsureStageNetworkCreatesAndConnectsCaddy(t *testing.T) {
	f := newFixture(t)
	f.withCaddy(t)
	ctx := context.Background()
	before, _ := f.docker.InspectContainer(ctx, naming.CaddyContainer)

	if err := f.ensure.Execute(ctx, staging); err != nil {
		t.Fatal(err)
	}
	n, err := f.docker.InspectNetwork(ctx, "auditfy_staging")
	if err != nil {
		t.Fatal(err)
	}
	if n.Driver != "overlay" || !n.Attachable || n.Labels[naming.LabelProject] != "auditfy" || n.Labels[naming.LabelStage] != "staging" {
		t.Fatalf("red: %+v", n)
	}
	after, _ := f.docker.InspectContainer(ctx, naming.CaddyContainer)
	if after.ID != before.ID || !after.StartedAt.Equal(before.StartedAt) || !slices.Contains(after.Networks, "auditfy_staging") {
		t.Fatalf("Caddy: antes %+v, después %+v", before, after)
	}

	// Es idempotente: la segunda vez no crea ni conecta nada nuevo, y no falla.
	calls := len(f.docker.Calls())
	if err := f.ensure.Execute(ctx, staging); err != nil {
		t.Fatal(err)
	}
	for _, c := range f.docker.Calls()[calls:] {
		if c == "CreateNetwork auditfy_staging" {
			t.Fatalf("la segunda vez creó la red otra vez: %v", f.docker.Calls()[calls:])
		}
	}
}

// Sin muelle-caddy (antes de que ingress lo cree, o en desarrollo) la red se
// crea igual: Caddy se conecta a todas las redes de stage cuando se crea.
func TestEnsureStageNetworkWithoutCaddy(t *testing.T) {
	f := newFixture(t)
	if err := f.ensure.Execute(context.Background(), staging); err != nil {
		t.Fatal(err)
	}
	if _, err := f.docker.InspectNetwork(context.Background(), "auditfy_staging"); err != nil {
		t.Fatal(err)
	}
}

// Una red con el nombre del stage que no es overlay attachable no se usa: no
// es de muelle y las apps no podrían compartirla con Caddy.
func TestEnsureStageNetworkRejectsForeignNetwork(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.docker.CreateNetwork(ctx, docker.NetworkSpec{Name: "auditfy_staging", Driver: "bridge"}); err != nil {
		t.Fatal(err)
	}
	if err := f.ensure.Execute(ctx, staging); !errors.Is(err, service.ErrNotStageNetwork) {
		t.Fatalf("red bridge con el nombre del stage: %v", err)
	}
}

func TestEnsureStageNetworkDockerDown(t *testing.T) {
	f := newFixture(t)
	f.docker.SetUnavailable(true)
	if err := f.ensure.Execute(context.Background(), staging); !errors.Is(err, entity.ErrDockerUnavailable) {
		t.Fatalf("Docker caído: %v (cmd/http lo responde 503)", err)
	}
}

// §3.2: al borrar el stage se desconecta Caddy y se borra la red.
func TestRemoveStageNetwork(t *testing.T) {
	f := newFixture(t)
	f.withCaddy(t)
	ctx := context.Background()
	if err := f.ensure.Execute(ctx, staging); err != nil {
		t.Fatal(err)
	}
	if err := f.remove.Execute(ctx, staging); err != nil {
		t.Fatal(err)
	}
	if _, err := f.docker.InspectNetwork(ctx, "auditfy_staging"); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("la red sigue: %v", err)
	}
	if got := f.caddyNetworks(t); !slices.Equal(got, []string{naming.PanelNetwork}) {
		t.Fatalf("Caddy quedó en %v", got)
	}
	// Borrar una red que ya no existe no es un error (reintentos, stages de antes de la Fase 3).
	if err := f.remove.Execute(ctx, staging); err != nil {
		t.Fatal(err)
	}
}

// Si algo sigue conectado (las tareas de un stack que Swarm todavía está
// bajando), el stage se borra igual y la red queda para la reconciliación de
// ingress (Fase 3), que borra las redes de stage huérfanas.
func TestRemoveStageNetworkInUse(t *testing.T) {
	f := newFixture(t)
	f.withCaddy(t)
	ctx := context.Background()
	if err := f.ensure.Execute(ctx, staging); err != nil {
		t.Fatal(err)
	}
	if _, err := f.docker.CreateContainer(ctx, docker.ContainerSpec{Name: "tarea-que-se-baja", Image: "caddy:test", Network: "auditfy_staging"}); err != nil {
		t.Fatal(err)
	}
	if err := f.remove.Execute(ctx, staging); err != nil {
		t.Fatalf("red en uso: %v", err)
	}
	if _, err := f.docker.InspectNetwork(ctx, "auditfy_staging"); err != nil {
		t.Fatalf("la red en uso no se borra: %v", err)
	}
	if slices.Contains(f.caddyNetworks(t), "auditfy_staging") {
		t.Fatal("Caddy se desconecta igual")
	}
}

func TestRemoveStageNetworkDockerDown(t *testing.T) {
	f := newFixture(t)
	f.docker.SetUnavailable(true)
	if err := f.remove.Execute(context.Background(), staging); !errors.Is(err, entity.ErrDockerUnavailable) {
		t.Fatalf("Docker caído: %v", err)
	}
}
