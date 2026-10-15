package usecases

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	outservice "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

// newProxyJob arma el job "proxy" sobre el lifecycleFixture.
func newProxyJob(f *lifecycleFixture) *ProxyJob {
	nets := outservice.NewDockerNetworks(f.docker)
	return NewProxyJob(f.settings, f.catalog,
		NewEnsureStageNetwork(nets, testCaddyNames.Container, f.slog), f.ensure,
		NewRemoveOrphanNetworks(nets, f.catalog, testCaddyNames.Container, "", f.slog),
		f.syncer, f.state, defCaddyImage, time.Millisecond, 4*time.Millisecond, f.slog)
}

// En desarrollo no hace nada: ni Docker ni la admin API.
func TestProxyJobDisabled(t *testing.T) {
	f := newLifecycleFixture(t)
	if err := newProxyJob(f).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls := f.docker.Calls(); len(calls) != 0 {
		t.Fatalf("Docker: %v", calls)
	}
	if n := len(f.caddy.Loads()); n != 0 {
		t.Fatalf("loads: %d", n)
	}
}

// Desde Docker vacío (el primer arranque): las redes de los stages, la red
// panel, muelle-caddy conectado a todas y la configuración.
func TestProxyJobFirstRun(t *testing.T) {
	f := newLifecycleFixture(t) // trae el stage auditfy/staging
	f.enableCaddy(t, nil)
	ingresstest.Stage(t, f.db, "auditfy", "prod")
	if err := newProxyJob(f).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, n := range []string{"auditfy_prod", "auditfy_staging", testCaddyNames.PanelNetwork} {
		if net, err := f.docker.InspectNetwork(ctx, n); err != nil || net.Driver != "overlay" || !net.Attachable {
			t.Fatalf("red %s: %+v %v", n, net, err)
		}
	}
	c := f.container(t)
	if want := []string{"auditfy_prod", "auditfy_staging", testCaddyNames.PanelNetwork}; !c.Running || !slices.Equal(c.Networks, want) {
		t.Fatalf("muelle-caddy: %+v", c)
	}
	if n := len(f.caddy.Loads()); n != 1 {
		t.Fatalf("loads: %d", n)
	}
	if status, msg, _ := f.state.Snapshot(true); status != ProxySynced {
		t.Fatalf("estado: %s %q", status, msg)
	}

	// La segunda pasada no recrea nada.
	if err := newProxyJob(f).Run(ctx); err != nil {
		t.Fatal(err)
	}
	if n := f.creates(); n != 1 {
		t.Fatalf("CreateContainer %d veces", n)
	}
}

// Con una recreación agendada, o frenada por un hook, el job no toca
// muelle-caddy (la hace el Recreator), pero sí manda la configuración.
func TestProxyJobSkipsCaddyWhileRecreating(t *testing.T) {
	for _, name := range []string{"agendada", "Hold"} {
		t.Run(name, func(t *testing.T) {
			f := newLifecycleFixture(t)
			f.enableCaddy(t, nil)
			f.docker.SetImagePresent(defCaddyImage)
			job := newProxyJob(f)
			ctx := context.Background()
			if err := job.Run(ctx); err != nil {
				t.Fatal(err)
			}
			f.changePorts(t, 9080)
			if name == "agendada" {
				f.state.BeginRecreate()
			} else {
				r := NewRecreator(f.ensure, f.syncer, f.state, time.Hour, time.Millisecond, time.Millisecond, f.slog)
				defer r.Hold()()
			}
			loads := len(f.caddy.Loads())
			if err := job.Run(ctx); err != nil {
				t.Fatal(err)
			}
			for _, op := range []string{"CreateContainer", "RemoveContainer", "StopContainer"} {
				if n := f.count(op + " " + testCaddyNames.Container); n > 1 || (op != "CreateContainer" && n > 0) {
					t.Fatalf("no debería tocar muelle-caddy: %v", f.docker.Calls())
				}
			}
			if len(f.caddy.Loads()) != loads+1 {
				t.Fatal("el Sync va igual")
			}
		})
	}
}

// Con Docker caído: un error que lo dice, y GET /proxy también.
func TestProxyJobDockerDown(t *testing.T) {
	f := newLifecycleFixture(t)
	f.enableCaddy(t, nil)
	f.docker.SetUnavailable(true)
	err := newProxyJob(f).Run(context.Background())
	if !errors.Is(err, shared.ErrDockerUnavailable) {
		t.Fatalf("Run: %v", err)
	}
	if status, msg, _ := f.state.Snapshot(true); status != ProxyOutOfSync || !strings.HasPrefix(msg, "Docker no responde") {
		t.Fatalf("estado: %s %q", status, msg)
	}
}

// Las redes de stage huérfanas se borran a la segunda pasada del job.
func TestProxyJobRemovesOrphans(t *testing.T) {
	f := newLifecycleFixture(t)
	f.enableCaddy(t, nil)
	f.docker.SetImagePresent(defCaddyImage)
	ctx := context.Background()
	if _, err := f.docker.CreateNetwork(ctx, docker.NetworkSpec{Name: "auditfy_viejo", Driver: "overlay", Attachable: true,
		Labels: naming.StageLabels("auditfy", "viejo")}); err != nil {
		t.Fatal(err)
	}
	job := newProxyJob(f)
	for range 2 {
		if err := job.Run(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.docker.InspectNetwork(ctx, "auditfy_viejo"); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("la huérfana sigue: %v", err)
	}
}

// Una red ajena con el nombre de un stage no corta el job: muelle-caddy y la
// configuración van igual, y el error se devuelve.
func TestProxyJobKeepsGoing(t *testing.T) {
	f := newLifecycleFixture(t)
	f.enableCaddy(t, nil)
	f.docker.SetImagePresent(defCaddyImage)
	ctx := context.Background()
	if _, err := f.docker.CreateNetwork(ctx, docker.NetworkSpec{Name: "auditfy_staging", Driver: "bridge"}); err != nil {
		t.Fatal(err)
	}
	err := newProxyJob(f).Run(ctx)
	if err == nil || !strings.Contains(err.Error(), "auditfy_staging") {
		t.Fatalf("Run: %v", err)
	}
	if c := f.container(t); !c.Running {
		t.Fatalf("muelle-caddy: %+v", c)
	}
	if n := len(f.caddy.Loads()); n != 1 {
		t.Fatalf("loads: %d", n)
	}
}

// Si el job crea muelle-caddy (el primer arranque), espera a que su admin
// API responda antes de dar el Sync por fallido: Caddy tarda un momento en
// abrir el socket.
func TestProxyJobWaitsForNewCaddy(t *testing.T) {
	f := newLifecycleFixture(t)
	f.enableCaddy(t, nil)
	f.docker.SetImagePresent(defCaddyImage)
	f.caddy.SetDown(true)
	done := make(chan error, 1)
	go func() { done <- newProxyJob(f).Run(context.Background()) }()
	eventually(t, 2*time.Second, "crea muelle-caddy", func() bool { return f.count("StartContainer "+testCaddyNames.Container) == 1 })
	time.Sleep(20 * time.Millisecond)
	f.caddy.SetDown(false)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run no volvió")
	}
	if !f.state.Synced() {
		t.Fatal("debería quedar synced")
	}

	// Sin recrear, un Caddy que no responde es un error enseguida (lo
	// reintenta el Retrier).
	f.caddy.SetDown(true)
	if err := newProxyJob(f).Run(context.Background()); !errors.Is(err, service.ErrCaddyUnreachable) {
		t.Fatalf("Run: %v", err)
	}
}
