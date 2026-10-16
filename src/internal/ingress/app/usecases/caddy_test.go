package usecases

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	outservice "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/caddy"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare/cftest"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
)

// Los nombres de muelle-caddy en estos tests: nunca los fijos.
var testCaddyNames = caddy.Names{
	Container: "t-caddy", PanelNetwork: "t_panel",
	DataVolume: "t-caddy-data", ConfigVolume: "t-caddy-config", AdminVolume: "t-caddy-admin",
}

const (
	// defCaddyImage es la imagen de Caddy de la versión v0.1.0.
	defCaddyImage    = "ghcr.io/cpu-commits/muelle-caddy:v0.1.0"
	newCaddyImage    = "ghcr.io/cpu-commits/muelle-caddy:v0.2.0"
	registryPassword = "reg-secret-1234567890"
)

// lifecycleFixture es syncFixture con Docker en memoria, muelle-caddy
// (caddy.Runtime con nombres de prueba) y EnsureCaddy, más Cloudflare falso
// (sin tokens ni zonas: cada test agrega los suyos) y el registro del panel.
// La imagen de Caddy por defecto es la de v0.1.0.
type lifecycleFixture struct {
	*syncFixture
	docker   *dockertest.Fake
	runtime  *caddy.Runtime
	ensure   *EnsureCaddy
	settings *outservice.ProxySettingsReader
	catalog  *outservice.CatalogReader
	cf       *cftest.Server
	ops      *Ops
	panel    *EnsurePanelDNS
	slog     *slog.Logger
}

func newLifecycleFixture(t *testing.T) *lifecycleFixture {
	t.Helper()
	sf := newSyncFixture(t)
	log := slog.New(slog.NewTextHandler(sf.log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	settings := outservice.NewProxySettingsReader(sf.db, sf.cipher)
	catalog := outservice.NewCatalogReader(sf.db)
	sf.syncer = NewSyncer(settings, sf.repo, catalog, caddy.NewAdmin(sf.caddy.Dial), sf.state, sf.clock,
		defCaddyImage, false, func() { sf.fails.Add(1) }, log)
	d := dockertest.New()
	rt := caddy.NewRuntime(d, testCaddyNames, log)
	cf, ops := cftest.New(t), &Ops{}
	return &lifecycleFixture{
		syncFixture: sf, docker: d, runtime: rt, settings: settings, catalog: catalog, slog: log, cf: cf, ops: ops,
		ensure: NewEnsureCaddy(rt, settings, catalog, outservice.NewRegistriesReader(sf.db, sf.cipher), sf.state, defCaddyImage, log),
		panel:  NewEnsurePanelDNS(settings, cloudflare.New(cf.URL()), ops, 30*time.Second, log),
	}
}

// enableCaddy deja el proxy habilitado con panel_domain, los puertos 8080 y
// 8443 y la imagen de la versión (caddy_image vacío), más lo que pida mod.
func (f *lifecycleFixture) enableCaddy(t *testing.T, mod func(*shared.ProxySettings)) {
	t.Helper()
	p := shared.DefaultProxySettings()
	p.PanelDomain = "panel.auditfy.cl"
	p.HTTPPort, p.HTTPSPort = 8080, 8443
	if mod != nil {
		mod(&p)
	}
	ingresstest.SetProxySettings(t, f.db, f.cipher, p)
}

func (f *lifecycleFixture) container(t *testing.T) docker.Container {
	t.Helper()
	c, err := f.docker.InspectContainer(context.Background(), testCaddyNames.Container)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// count cuenta las llamadas que empiezan con las palabras de prefix, enteras
// ("CreateContainer t-caddy" no cuenta el contenedor de prueba de los
// puertos, t-caddy-probe).
func (f *lifecycleFixture) count(prefix string) int {
	p := strings.Fields(prefix)
	n := 0
	for _, c := range f.docker.Calls() {
		if fs := strings.Fields(c); len(fs) >= len(p) && slices.Equal(fs[:len(p)], p) {
			n++
		}
	}
	return n
}

// creates cuenta los CreateContainer de muelle-caddy.
func (f *lifecycleFixture) creates() int {
	return f.count("CreateContainer " + testCaddyNames.Container)
}

// recordingRuntime es un CaddyRuntime que anota lo que le piden.
type recordingRuntime struct {
	mu    sync.Mutex
	wants []service.CaddyWant
}

func (r *recordingRuntime) Ensure(ctx context.Context, want service.CaddyWant) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wants = append(r.wants, want)
	return false, nil
}

func (r *recordingRuntime) Status(ctx context.Context) (string, bool, bool, error) {
	return "", false, false, nil
}

func (r *recordingRuntime) last(t *testing.T) service.CaddyWant {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.wants) == 0 {
		t.Fatal("no se llamó a Ensure")
	}
	return r.wants[len(r.wants)-1]
}

// withRecorder reemplaza el runtime de EnsureCaddy por uno que anota.
func (f *lifecycleFixture) withRecorder() *recordingRuntime {
	r := &recordingRuntime{}
	f.ensure = NewEnsureCaddy(r, f.settings, f.catalog, outservice.NewRegistriesReader(f.db, f.cipher), f.state, defCaddyImage, f.slog)
	return r
}

// En desarrollo (sin panel_domain) ingress no toca Docker por Caddy (contrato §3.4).
func TestEnsureCaddyDisabled(t *testing.T) {
	f := newLifecycleFixture(t)
	recreated, err := f.ensure.Execute(context.Background())
	if err != nil || recreated {
		t.Fatalf("Execute: %v %v", recreated, err)
	}
	if calls := f.docker.Calls(); len(calls) != 0 {
		t.Fatalf("no debería llamar a Docker: %v", calls)
	}
}

// La imagen es caddy_image o la de la versión, y va con la credencial del
// registry de su host si hay una.
func TestEnsureCaddyImage(t *testing.T) {
	ctx := context.Background()
	f := newLifecycleFixture(t)
	rec := f.withRecorder()
	f.enableCaddy(t, nil)
	if _, err := f.ensure.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if w := rec.last(t); w.Image != defCaddyImage || w.Auth != nil || w.HTTPPort != 8080 || w.HTTPSPort != 8443 {
		t.Fatalf("sin caddy_image ni registry: %+v", w)
	}

	ingresstest.Registry(t, f.db, f.cipher, "ghcr.io", "bot", registryPassword)
	if _, err := f.ensure.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if w := rec.last(t); w.Auth == nil || *w.Auth != (service.Credential{Host: "ghcr.io", Username: "bot", Password: registryPassword}) {
		t.Fatalf("con el registry ghcr.io: %+v", w.Auth)
	}

	// caddy_image fija, de Docker Hub: sin un registry docker.io, anónima.
	f.enableCaddy(t, func(p *shared.ProxySettings) { p.CaddyImage = "caddy:2-alpine" })
	if _, err := f.ensure.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if w := rec.last(t); w.Image != "caddy:2-alpine" || w.Auth != nil {
		t.Fatalf("caddy_image fija: %+v", w)
	}
	ingresstest.Registry(t, f.db, f.cipher, "docker.io", "hub", "hub-pass")
	if _, err := f.ensure.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if w := rec.last(t); w.Auth == nil || w.Auth.Host != "docker.io" {
		t.Fatalf("con el registry docker.io: %+v", w.Auth)
	}
}

// muelle-caddy va conectado a las redes de todos los stages.
func TestEnsureCaddyNetworks(t *testing.T) {
	f := newLifecycleFixture(t)
	rec := f.withRecorder()
	f.enableCaddy(t, nil)
	ingresstest.Stage(t, f.db, "auditfy", "prod")
	ingresstest.Stage(t, f.db, "web", "dev")
	if _, err := f.ensure.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{naming.Network("auditfy", "prod"), naming.Network("auditfy", "staging"), naming.Network("web", "dev")}
	if got := rec.last(t).Networks; !slices.Equal(got, want) {
		t.Fatalf("redes: %v", got)
	}
}

// Un puerto ocupado queda en GET /proxy con los dos puertos, y se limpia
// cuando el contenedor arranca.
func TestEnsureCaddyPortInUse(t *testing.T) {
	f := newLifecycleFixture(t)
	f.enableCaddy(t, nil)
	f.docker.SetImagePresent(defCaddyImage)
	f.docker.BusyHostPort(8080)
	ctx := context.Background()
	if _, err := f.ensure.Execute(ctx); !errors.Is(err, service.ErrPortInUse) {
		t.Fatalf("Execute: %v", err)
	}
	status, msg, _ := f.state.Snapshot(true)
	if status != ProxyOutOfSync || !strings.Contains(msg, "los puertos 8080 o 8443 del host ya están en uso") {
		t.Fatalf("estado: %s %q", status, msg)
	}

	f.docker.FreeHostPort(8080)
	recreated, err := f.ensure.Execute(ctx)
	if err != nil || !recreated {
		t.Fatalf("Execute: %v %v", recreated, err)
	}
	if _, msg, _ := f.state.Snapshot(true); msg != NotSyncedYet {
		t.Fatalf("el error del contenedor se limpia: %q", msg)
	}
}

// Si la imagen nueva no baja, muelle-caddy sigue con la anterior, y el
// mensaje dice qué hacer en Ajustes → Registries (las imágenes de muelle son
// privadas).
func TestEnsureCaddyPullFails(t *testing.T) {
	const (
		base     = "muelle no pudo bajar la imagen " + newCaddyImage + " de Caddy ("
		kept     = "); muelle-caddy sigue con la anterior"
		addIt    = ". Si la imagen es privada, agrega el registry ghcr.io en Ajustes → Registries, con un usuario y un token que puedan leerla"
		updateIt = ". El registry ghcr.io no dio acceso con la credencial guardada: actualízala en Ajustes → Registries (el token puede haber vencido)"
	)
	cases := []struct {
		name     string
		registry bool
		fail     func(*dockertest.Fake)
		suffix   string
	}{
		{"denegada sin registry", false, func(d *dockertest.Fake) { d.DenyImage(newCaddyImage) }, kept + addIt},
		{"denegada con registry", true, func(d *dockertest.Fake) { d.DenyImage(newCaddyImage) }, kept + updateIt},
		{"no existe sin registry", false, func(d *dockertest.Fake) { d.FailNext("PullImage", docker.ErrNotFound) }, kept + addIt},
		{"no existe con registry", true, func(d *dockertest.Fake) { d.FailNext("PullImage", docker.ErrNotFound) }, kept + updateIt},
		{"otro error", false, func(d *dockertest.Fake) { d.FailNext("PullImage", errors.New("el registry no respondió")) }, kept},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newLifecycleFixture(t)
			ctx := context.Background()
			f.enableCaddy(t, nil)
			f.docker.SetImagePresent(defCaddyImage)
			if _, err := f.ensure.Execute(ctx); err != nil {
				t.Fatal(err)
			}
			before := f.container(t)
			if tc.registry {
				ingresstest.Registry(t, f.db, f.cipher, "ghcr.io", "bot", registryPassword)
			}
			f.enableCaddy(t, func(p *shared.ProxySettings) { p.CaddyImage = newCaddyImage })
			tc.fail(f.docker)

			if _, err := f.ensure.Execute(ctx); !errors.Is(err, service.ErrCaddyImagePull) {
				t.Fatalf("Execute: %v", err)
			}
			status, msg, _ := f.state.Snapshot(true)
			if status != ProxyOutOfSync || !strings.HasPrefix(msg, base) || !strings.HasSuffix(msg, tc.suffix) {
				t.Fatalf("estado: %s %q", status, msg)
			}
			if tc.suffix == kept && strings.Contains(msg, "Registries") {
				t.Fatalf("otro error no habla de Registries: %q", msg)
			}
			after := f.container(t)
			if after.ID != before.ID || !after.Running || after.Image != defCaddyImage {
				t.Fatalf("el anterior sigue: %+v", after)
			}
		})
	}
}

// Sin un muelle-caddy que siga corriendo, el mensaje no dice que sigue.
func TestEnsureCaddyPullFailsFirstTime(t *testing.T) {
	f := newLifecycleFixture(t)
	f.enableCaddy(t, nil)
	f.docker.DenyImage(defCaddyImage)
	if _, err := f.ensure.Execute(context.Background()); !errors.Is(err, service.ErrCaddyImagePull) {
		t.Fatalf("Execute: %v", err)
	}
	if _, msg, _ := f.state.Snapshot(true); !strings.Contains(msg, "); muelle-caddy no está corriendo") {
		t.Fatalf("mensaje: %q", msg)
	}
}

// La contraseña del registry (que se usa en cada recreación) nunca sale en
// el estado, el error ni el log.
func TestEnsureCaddyHidesRegistryPassword(t *testing.T) {
	f := newLifecycleFixture(t)
	f.enableCaddy(t, nil)
	ingresstest.Registry(t, f.db, f.cipher, "ghcr.io", "bot", registryPassword)
	f.docker.DenyImage(defCaddyImage)
	_, err := f.ensure.Execute(context.Background())
	if err == nil {
		t.Fatal("debería fallar")
	}
	_, msg, _ := f.state.Snapshot(true)
	for name, s := range map[string]string{"estado": msg, "error": err.Error(), "log": f.log.String()} {
		if strings.Contains(s, registryPassword) {
			t.Errorf("%s muestra la contraseña: %s", name, s)
		}
	}
}

func TestEnsureCaddyDockerDown(t *testing.T) {
	f := newLifecycleFixture(t)
	f.enableCaddy(t, nil)
	f.docker.SetUnavailable(true)
	if _, err := f.ensure.Execute(context.Background()); !errors.Is(err, shared.ErrDockerUnavailable) {
		t.Fatalf("Execute: %v", err)
	}
	if status, msg, _ := f.state.Snapshot(true); status != ProxyOutOfSync || !strings.HasPrefix(msg, "Docker no responde") {
		t.Fatalf("estado: %s %q", status, msg)
	}
}

// Dos Execute a la vez crean un solo contenedor.
func TestEnsureCaddySerialized(t *testing.T) {
	f := newLifecycleFixture(t)
	f.enableCaddy(t, nil)
	f.docker.SetImagePresent(defCaddyImage)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			if _, err := f.ensure.Execute(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if n := f.creates(); n != 1 {
		t.Fatalf("CreateContainer %d veces: %v", n, f.docker.Calls())
	}
}

// Otro error de Docker queda en el estado tal cual, sin decir que fue al
// crearlo (puede ser al inspeccionarlo o al conectar una red).
func TestEnsureCaddyOtherError(t *testing.T) {
	f := newLifecycleFixture(t)
	f.enableCaddy(t, nil)
	f.docker.FailNext("InspectContainer", errors.New("respuesta rara del daemon"))
	if _, err := f.ensure.Execute(context.Background()); err == nil {
		t.Fatal("debería fallar")
	}
	if _, msg, _ := f.state.Snapshot(true); msg != "muelle no pudo dejar muelle-caddy como piden los ajustes: respuesta rara del daemon" {
		t.Fatalf("mensaje: %q", msg)
	}
}
