package usecases

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/app/dto"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	outservice "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/caddy"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

// hookedRuntime es el runtime de verdad, con before y after alrededor de
// cada Ensure (n es cuántos Ensure van, desde 1): así un test cambia los
// ajustes o el Docker en medio de una recreación.
type hookedRuntime struct {
	service.CaddyRuntime
	n             int
	before, after func(n int)
}

func (r *hookedRuntime) Ensure(ctx context.Context, want service.CaddyWant) (bool, error) {
	r.n++
	if r.before != nil {
		r.before(r.n)
	}
	recreated, err := r.CaddyRuntime.Ensure(ctx, want)
	if r.after != nil {
		r.after(r.n)
	}
	return recreated, err
}

// withHookedRuntime reemplaza el runtime de EnsureCaddy por uno con ganchos.
func (f *lifecycleFixture) withHookedRuntime(before, after func(n int)) {
	f.ensure = f.newEnsure(&hookedRuntime{CaddyRuntime: f.runtime, before: before, after: after})
}

// servingFixture deja muelle-caddy sirviendo con 8080/8443.
func servingFixture(t *testing.T) (*lifecycleFixture, docker.Container) {
	t.Helper()
	f := newLifecycleFixture(t)
	f.enableCaddy(t, nil)
	f.docker.SetImagePresent(defCaddyImage)
	if _, err := f.ensure.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	return f, f.container(t)
}

func (f *lifecycleFixture) ports(t *testing.T) service.ProxyPorts {
	t.Helper()
	p, err := f.settings.ProxySettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return service.ProxyPorts{HTTPPort: p.HTTPPort, HTTPSPort: p.HTTPSPort}
}

var servingPorts = service.ProxyPorts{HTTPPort: 8080, HTTPSPort: 8443}

// La recreación de un cambio de puertos que no arranca (un puerto que alguien
// tomó después de la prueba del PATCH) vuelve a los puertos con que servía
// muelle-caddy: los ajustes y el contenedor. Queda en GET /proxy
// (ports_rollback), en la auditoría y en el log, y el origen del panel en
// caché se invalida (Done).
func TestRecreateRollsBackBusyPort(t *testing.T) {
	f, before := servingFixture(t)
	f.changePorts(t, 9080)
	f.docker.BusyHostPort(9080)
	ctx := context.Background()

	recreated, err := f.ensure.Recreate(ctx)
	if !errors.Is(err, service.ErrPortInUse) || !recreated {
		t.Fatalf("Recreate: %v %v", recreated, err)
	}
	if got := f.ports(t); got != servingPorts {
		t.Fatalf("los ajustes vuelven a 8080/8443: %+v", got)
	}
	c := f.container(t)
	if c.ID == before.ID || !c.Running || c.Ports[0].HostPort != 8080 || len(c.PublishedPorts) == 0 {
		t.Fatalf("muelle-caddy de nuevo con 8080/8443: %+v", c)
	}
	if _, msg, _ := f.state.Snapshot(true); msg != NotSyncedYet {
		t.Fatalf("sin error del contenedor (falta el Load): %q", msg)
	}
	rb := f.state.LastPortsRollback()
	if rb == nil || rb.Attempted != (service.ProxyPorts{HTTPPort: 9080, HTTPSPort: 8443}) || rb.Restored != servingPorts ||
		!strings.Contains(rb.Error, "9080") || !rb.At.Equal(f.clock.Now()) {
		t.Fatalf("ports_rollback: %+v", rb)
	}
	if got := f.audits(t); len(got) != 1 || got[0] !=
		`settings.ports_rollback settings {"attempted":{"proxy_http_port":9080,"proxy_https_port":8443},"error":"`+rb.Error+`","proxy_http_port":8080,"proxy_https_port":8443}` {
		t.Fatalf("auditoría: %v", got)
	}
	if n := f.rolledBack.Load(); n != 1 {
		t.Fatalf("Done: %d", n)
	}
	if !strings.Contains(f.log.String(), "se vuelve a los puertos anteriores") {
		t.Fatalf("el log: %s", f.log)
	}
}

// El job (Execute) no vuelve atrás: un cambio hecho a mano (el UPDATE del
// contrato §4.1) se reintenta con los mismos ajustes.
func TestExecuteDoesNotRollBack(t *testing.T) {
	f, _ := servingFixture(t)
	f.changePorts(t, 9080)
	f.docker.BusyHostPort(9080)
	if _, err := f.ensure.Execute(context.Background()); !errors.Is(err, service.ErrPortInUse) {
		t.Fatalf("Execute: %v", err)
	}
	if got := f.ports(t); got.HTTPPort != 9080 {
		t.Fatalf("los ajustes no cambian: %+v", got)
	}
	if f.state.LastPortsRollback() != nil || len(f.audits(t)) != 0 {
		t.Fatalf("no vuelve atrás: %+v %v", f.state.LastPortsRollback(), f.audits(t))
	}
}

// Si un PATCH /settings cambió los puertos mientras se recreaba, ese gana: no
// se vuelve a los anteriores (su recreación viene después).
func TestRecreateRollbackKeepsNewerPatch(t *testing.T) {
	f, _ := servingFixture(t)
	f.changePorts(t, 9080)
	f.docker.BusyHostPort(9080)
	f.withHookedRuntime(func(n int) {
		if n == 1 {
			f.enableCaddy(t, func(p *shared.ProxySettings) { p.HTTPPort, p.HTTPSPort = 9080, 9443 })
		}
	}, nil)
	if _, err := f.ensure.Recreate(context.Background()); !errors.Is(err, service.ErrPortInUse) {
		t.Fatalf("Recreate: %v", err)
	}
	if got := f.ports(t); got != (service.ProxyPorts{HTTPPort: 9080, HTTPSPort: 9443}) {
		t.Fatalf("el PATCH nuevo queda: %+v", got)
	}
	if status, msg, _ := f.state.Snapshot(true); status != ProxyOutOfSync || !strings.Contains(msg, "9080") {
		t.Fatalf("estado: %s %q", status, msg)
	}
	if f.state.LastPortsRollback() != nil || len(f.audits(t)) != 0 || f.rolledBack.Load() != 0 {
		t.Fatal("no hubo vuelta atrás")
	}
}

// Si los puertos anteriores tampoco arrancan (alguien los tomó mientras
// tanto), los ajustes quedan en los anteriores, el error del contenedor es
// el de esos puertos y ports_rollback dice lo que pasó.
func TestRecreateRollbackAlsoFails(t *testing.T) {
	f, _ := servingFixture(t)
	f.changePorts(t, 9080)
	f.docker.BusyHostPort(9080)
	f.withHookedRuntime(nil, func(n int) {
		if n == 1 {
			f.docker.BusyHostPort(8443)
		}
	})
	if _, err := f.ensure.Recreate(context.Background()); !errors.Is(err, service.ErrPortInUse) {
		t.Fatalf("Recreate: %v", err)
	}
	if got := f.ports(t); got != servingPorts {
		t.Fatalf("los ajustes vuelven igual: %+v", got)
	}
	status, msg, _ := f.state.Snapshot(true)
	if status != ProxyOutOfSync || !strings.Contains(msg, "los puertos 8080 o 8443 del host ya están en uso") {
		t.Fatalf("estado: %s %q", status, msg)
	}
	if rb := f.state.LastPortsRollback(); rb == nil || rb.Restored != servingPorts {
		t.Fatalf("ports_rollback: %+v", rb)
	}
}

// Un stage cuya red se asegura mientras la vuelta atrás crea el muelle-caddy
// con los puertos anteriores (el paso 4 de un deploy): el segundo Ensure usa
// los stages de la primera lectura, así que, como Execute, Recreate vuelve a
// leerlos después y lo conecta a la red que falta.
func TestRecreateRollbackConnectsStageAddedWhileRecreating(t *testing.T) {
	f, _ := servingFixture(t)
	f.changePorts(t, 9080)
	f.docker.BusyHostPort(9080)
	ctx := context.Background()
	eng := &hookEngine{Fake: f.docker}
	f.ensure = f.newEnsure(caddy.NewRuntime(eng, testCaddyNames, f.slog))
	networks := NewEnsureStageNetwork(outservice.NewDockerNetworks(f.docker), testCaddyNames.Container, f.slog)
	web := naming.Network("web", "dev")
	eng.beforeCreate = func() {
		ingresstest.Stage(t, f.db, "web", "dev")
		if err := networks.Execute(ctx, dto.StageRef{Project: "web", Stage: "dev"}); err != nil {
			t.Errorf("EnsureStageNetwork: %v", err)
		}
	}

	recreated, err := f.ensure.Recreate(ctx)
	if !errors.Is(err, service.ErrPortInUse) || !recreated {
		t.Fatalf("Recreate: %v %v", recreated, err)
	}
	if eng.beforeCreate != nil || f.state.LastPortsRollback() == nil {
		t.Fatal("no volvió atrás creando muelle-caddy de nuevo")
	}
	if c := f.container(t); c.Ports[0].HostPort != 8080 || !slices.Contains(c.Networks, web) {
		t.Fatalf("muelle-caddy con 8080 y conectado a %s: %+v", web, c)
	}
}

// Si la imagen no baja, el muelle-caddy anterior sigue sirviendo con los
// puertos de antes: los ajustes vuelven a esos, y el panel sigue en su
// origen (el Origin que pasa sale de los ajustes).
func TestRecreateRollbackPullFails(t *testing.T) {
	f, before := servingFixture(t)
	f.enableCaddy(t, func(p *shared.ProxySettings) { p.HTTPPort, p.CaddyImage = 9080, newCaddyImage })
	f.docker.DenyImage(newCaddyImage)
	if _, err := f.ensure.Recreate(context.Background()); !errors.Is(err, service.ErrCaddyImagePull) {
		t.Fatalf("Recreate: %v", err)
	}
	if got := f.ports(t); got != servingPorts {
		t.Fatalf("los ajustes vuelven: %+v", got)
	}
	if c := f.container(t); c.ID != before.ID || !c.Running {
		t.Fatalf("el anterior sigue: %+v", c)
	}
	if rb := f.state.LastPortsRollback(); rb == nil || !strings.Contains(rb.Error, "no pudo bajar la imagen") {
		t.Fatalf("ports_rollback: %+v", rb)
	}
}

// Sin cambio de puertos (por ejemplo, la imagen de una versión nueva que no
// baja, o un muelle-caddy que no servía) no hay a qué volver; Docker caído
// tampoco vuelve atrás: no es culpa de los puertos.
func TestRecreateWithoutRollback(t *testing.T) {
	t.Run("sin cambio de puertos", func(t *testing.T) {
		f, _ := servingFixture(t)
		f.enableCaddy(t, func(p *shared.ProxySettings) { p.CaddyImage = newCaddyImage })
		f.docker.DenyImage(newCaddyImage)
		if _, err := f.ensure.Recreate(context.Background()); !errors.Is(err, service.ErrCaddyImagePull) {
			t.Fatalf("Recreate: %v", err)
		}
		if f.state.LastPortsRollback() != nil || len(f.audits(t)) != 0 {
			t.Fatal("no hay a qué volver")
		}
	})
	t.Run("sin muelle-caddy", func(t *testing.T) {
		f := newLifecycleFixture(t)
		f.enableCaddy(t, nil)
		f.docker.SetImagePresent(defCaddyImage)
		f.docker.BusyHostPort(8080)
		if _, err := f.ensure.Recreate(context.Background()); !errors.Is(err, service.ErrPortInUse) {
			t.Fatalf("Recreate: %v", err)
		}
		if f.state.LastPortsRollback() != nil || len(f.audits(t)) != 0 {
			t.Fatal("no hay a qué volver")
		}
	})
	t.Run("Docker caído", func(t *testing.T) {
		f, _ := servingFixture(t)
		f.changePorts(t, 9080)
		f.docker.FailNext("RemoveContainer", docker.ErrUnavailable)
		if _, err := f.ensure.Recreate(context.Background()); !errors.Is(err, shared.ErrDockerUnavailable) {
			t.Fatalf("Recreate: %v", err)
		}
		if got := f.ports(t); got.HTTPPort != 9080 || f.state.LastPortsRollback() != nil {
			t.Fatalf("no vuelve atrás: %+v %+v", got, f.state.LastPortsRollback())
		}
	})
}

// El Recreator usa Recreate: al final GET /proxy dice synced, con los
// puertos de antes y ports_rollback.
func TestRecreatorRollsBack(t *testing.T) {
	f, r := newRecreatorFixture(t, time.Millisecond)
	f.changePorts(t, 9080)
	f.docker.BusyHostPort(9080)
	r.Schedule()
	eventually(t, 2*time.Second, "la recreación termina", func() bool { return !f.state.Recreating() })
	if status, msg, _ := f.state.Snapshot(true); status != ProxySynced {
		t.Fatalf("al final: %s %q", status, msg)
	}
	if f.state.LastPortsRollback() == nil || f.ports(t) != servingPorts {
		t.Fatalf("vuelta atrás: %+v %+v", f.state.LastPortsRollback(), f.ports(t))
	}
}
