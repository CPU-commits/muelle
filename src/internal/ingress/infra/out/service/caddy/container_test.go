package caddy

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
)

// Los nombres de prueba: nunca los fijos de producción.
var testNames = Names{
	Container: "t-caddy", PanelNetwork: "t_panel",
	DataVolume: "t-caddy-data", ConfigVolume: "t-caddy-config", AdminVolume: "t-caddy-admin",
}

const (
	testImage    = "ghcr.io/cpu-commits/muelle-caddy:v0.1.0"
	testImage2   = "ghcr.io/cpu-commits/muelle-caddy:v0.2.0"
	testPassword = "reg-secret-1234567890"
)

// recordingEngine envuelve el fake y anota lo que el fake no guarda: el spec
// de cada CreateContainer y la credencial de cada PullImage (D20). failOn
// hace fallar una operación sobre un contenedor ("CreateContainer t-caddy"),
// sin tocar la del contenedor de prueba de los puertos.
type recordingEngine struct {
	*dockertest.Fake
	mu     sync.Mutex
	specs  []docker.ContainerSpec
	auths  []*docker.RegistryAuth
	failOn map[string]error
}

func (r *recordingEngine) fail(op, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	err := r.failOn[op+" "+name]
	delete(r.failOn, op+" "+name)
	return err
}

func (r *recordingEngine) CreateContainer(ctx context.Context, spec docker.ContainerSpec) (string, error) {
	r.mu.Lock()
	r.specs = append(r.specs, spec)
	r.mu.Unlock()
	if err := r.fail("CreateContainer", spec.Name); err != nil {
		return "", err
	}
	return r.Fake.CreateContainer(ctx, spec)
}

func (r *recordingEngine) StartContainer(ctx context.Context, name string) error {
	if err := r.fail("StartContainer", name); err != nil {
		return err
	}
	return r.Fake.StartContainer(ctx, name)
}

// created son los specs de los CreateContainer de ese nombre.
func (r *recordingEngine) created(name string) []docker.ContainerSpec {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []docker.ContainerSpec
	for _, s := range r.specs {
		if s.Name == name {
			out = append(out, s)
		}
	}
	return out
}

func (r *recordingEngine) PullImage(ctx context.Context, ref string, auth *docker.RegistryAuth) error {
	r.mu.Lock()
	if auth != nil {
		c := *auth
		auth = &c
	}
	r.auths = append(r.auths, auth)
	r.mu.Unlock()
	return r.Fake.PullImage(ctx, ref, auth)
}

type runtimeFixture struct {
	fake    *dockertest.Fake
	engine  *recordingEngine
	runtime *Runtime
	log     *bytes.Buffer
}

func newRuntimeFixture(t *testing.T) *runtimeFixture {
	t.Helper()
	f := dockertest.New()
	e := &recordingEngine{Fake: f, failOn: map[string]error{}}
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return &runtimeFixture{fake: f, engine: e, runtime: NewRuntime(e, testNames, log), log: &buf}
}

// stageNetwork crea la red de un stage, como EnsureStageNetwork.
func (f *runtimeFixture) stageNetwork(t *testing.T, name string) {
	t.Helper()
	if _, err := f.fake.CreateNetwork(context.Background(), docker.NetworkSpec{Name: name, Driver: "overlay", Attachable: true}); err != nil {
		t.Fatal(err)
	}
}

func (f *runtimeFixture) container(t *testing.T) docker.Container {
	t.Helper()
	c, err := f.fake.InspectContainer(context.Background(), testNames.Container)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *runtimeFixture) ensure(t *testing.T, want service.CaddyWant) bool {
	t.Helper()
	recreated, err := f.runtime.Ensure(context.Background(), want)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	return recreated
}

// since son las llamadas desde la n-ésima.
func (f *runtimeFixture) since(n int) []string { return f.fake.Calls()[n:] }

func baseWant(networks ...string) service.CaddyWant {
	return service.CaddyWant{Image: testImage, HTTPPort: 8080, HTTPSPort: 8443, Networks: networks}
}

// callMatches dice si la llamada empieza con las palabras de prefix, enteras:
// "StartContainer t-caddy" no es "StartContainer t-caddy-probe".
func callMatches(call, prefix string) bool {
	c, p := strings.Fields(call), strings.Fields(prefix)
	return len(c) >= len(p) && slices.Equal(c[:len(p)], p)
}

func hasCall(calls []string, prefix string) bool {
	return slices.ContainsFunc(calls, func(c string) bool { return callMatches(c, prefix) })
}

func callIndex(calls []string, prefix string) int {
	return slices.IndexFunc(calls, func(c string) bool { return callMatches(c, prefix) })
}

func lastCallIndex(calls []string, prefix string) int {
	for i := len(calls) - 1; i >= 0; i-- {
		if callMatches(calls[i], prefix) {
			return i
		}
	}
	return -1
}

// Desde Docker vacío: crea la red panel y el contenedor como dice el
// contrato §3.4, y lo conecta a las redes de stage antes de arrancarlo. Una
// red de stage que ya no existe se salta.
func TestEnsureCreates(t *testing.T) {
	f := newRuntimeFixture(t)
	f.fake.SetImagePresent(testImage)
	f.stageNetwork(t, "auditfy_staging")
	f.stageNetwork(t, "auditfy_prod")

	if !f.ensure(t, baseWant("auditfy_staging", "auditfy_borrado", "auditfy_prod")) {
		t.Fatal("recreated debería ser true")
	}

	n, err := f.fake.InspectNetwork(context.Background(), testNames.PanelNetwork)
	if err != nil || n.Driver != "overlay" || !n.Attachable {
		t.Fatalf("red panel: %+v %v", n, err)
	}
	specs := f.engine.created(testNames.Container)
	if len(specs) != 1 {
		t.Fatalf("CreateContainer: %d", len(specs))
	}
	spec := specs[0]
	if spec.Name != testNames.Container || spec.Image != testImage || spec.RestartPolicy != "unless-stopped" ||
		spec.Network != testNames.PanelNetwork {
		t.Fatalf("spec: %+v", spec)
	}
	if !slices.Equal(spec.Cmd, naming.CaddyCommand()) || !slices.Equal(spec.Env, naming.CaddyEnv()) {
		t.Fatalf("Cmd/Env: %v %v", spec.Cmd, spec.Env)
	}
	wantMounts := []docker.Mount{
		{Volume: testNames.DataVolume, Target: naming.CaddyDataDir},
		{Volume: testNames.ConfigVolume, Target: naming.CaddyConfigDir},
		{Volume: testNames.AdminVolume, Target: naming.CaddyAdminDir},
	}
	if !slices.Equal(spec.Mounts, wantMounts) {
		t.Fatalf("Mounts: %+v", spec.Mounts)
	}
	wantPorts := []docker.PortBinding{
		{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
		{HostPort: 8443, ContainerPort: 443, Protocol: "tcp"},
		{HostPort: 8443, ContainerPort: 443, Protocol: "udp"},
	}
	if !slices.Equal(spec.Ports, wantPorts) {
		t.Fatalf("Ports: %+v", spec.Ports)
	}

	calls := f.fake.Calls()
	start := callIndex(calls, "StartContainer "+testNames.Container)
	for _, net := range []string{"auditfy_staging", "auditfy_prod"} {
		i := callIndex(calls, "ConnectNetwork "+net+" "+testNames.Container)
		if i < 0 || start < 0 || i > start {
			t.Fatalf("la red %s se conecta antes de arrancar: %v", net, calls)
		}
	}
	c := f.container(t)
	if !c.Running || len(c.PublishedPorts) == 0 {
		t.Fatalf("contenedor: %+v", c)
	}
	if want := []string{"auditfy_prod", "auditfy_staging", testNames.PanelNetwork}; !slices.Equal(c.Networks, want) {
		t.Fatalf("redes: %v", c.Networks)
	}
	if !strings.Contains(f.log.String(), "auditfy_borrado") {
		t.Errorf("la red que no existe va al log: %s", f.log)
	}
}

// Si ya está como se pide, Ensure no lo toca.
func TestEnsureIdempotent(t *testing.T) {
	f := newRuntimeFixture(t)
	f.fake.SetImagePresent(testImage)
	f.stageNetwork(t, "auditfy_staging")
	f.ensure(t, baseWant("auditfy_staging"))
	id := f.container(t).ID
	n := len(f.fake.Calls())

	if f.ensure(t, baseWant("auditfy_staging")) {
		t.Fatal("recreated debería ser false")
	}
	for _, op := range []string{"CreateContainer", "RemoveContainer", "StopContainer", "ConnectNetwork", "PullImage"} {
		if hasCall(f.since(n), op) {
			t.Errorf("la segunda llamada no debería tener %s: %v", op, f.since(n))
		}
	}
	if f.container(t).ID != id {
		t.Fatal("el id cambió")
	}
}

// Una red de stage nueva se conecta en caliente: el mismo contenedor.
func TestEnsureConnectsNewNetwork(t *testing.T) {
	f := newRuntimeFixture(t)
	f.fake.SetImagePresent(testImage)
	f.stageNetwork(t, "auditfy_staging")
	f.ensure(t, baseWant("auditfy_staging"))
	before := f.container(t)
	f.stageNetwork(t, "auditfy_prod")
	n := len(f.fake.Calls())

	if f.ensure(t, baseWant("auditfy_staging", "auditfy_prod")) {
		t.Fatal("recreated debería ser false")
	}
	var mutating []string
	for _, c := range f.since(n) {
		if !strings.HasPrefix(c, "Inspect") {
			mutating = append(mutating, c)
		}
	}
	if !slices.Equal(mutating, []string{"ConnectNetwork auditfy_prod " + testNames.Container}) {
		t.Fatalf("solo debería conectar la red nueva: %v", f.since(n))
	}
	after := f.container(t)
	if after.ID != before.ID || !after.StartedAt.Equal(before.StartedAt) || !slices.Contains(after.Networks, "auditfy_prod") {
		t.Fatalf("antes %+v, después %+v", before, after)
	}
}

// Si alguien lo desconectó de la red panel, Ensure lo vuelve a conectar: si
// no, el panel no llegaría a muelle hasta otra recreación.
func TestEnsureReconnectsPanel(t *testing.T) {
	f := newRuntimeFixture(t)
	f.fake.SetImagePresent(testImage)
	f.ensure(t, baseWant())
	id := f.container(t).ID
	ctx := context.Background()
	if err := f.fake.DisconnectNetwork(ctx, testNames.PanelNetwork, testNames.Container); err != nil {
		t.Fatal(err)
	}
	n := len(f.fake.Calls())

	if f.ensure(t, baseWant()) {
		t.Fatal("recreated debería ser false")
	}
	if !slices.Contains(f.since(n), "ConnectNetwork "+testNames.PanelNetwork+" "+testNames.Container) {
		t.Fatalf("debería reconectar la red panel: %v", f.since(n))
	}
	c := f.container(t)
	if c.ID != id || !slices.Contains(c.Networks, testNames.PanelNetwork) {
		t.Fatalf("contenedor: %+v", c)
	}
}

// Cada diferencia del contrato §3.4 recrea: detener (si corría), borrar y
// crear uno nuevo, nunca rearrancar el mismo.
func TestEnsureRecreates(t *testing.T) {
	ctx := context.Background()
	// create crea a mano el contenedor de baseWant, cambiado por mod, sin arrancarlo.
	create := func(t *testing.T, f *runtimeFixture, mod func(*docker.ContainerSpec)) {
		spec := f.runtime.spec(baseWant())
		mod(&spec)
		f.fake.SetImagePresent(spec.Image)
		if _, err := f.fake.CreateNetwork(ctx, docker.NetworkSpec{Name: testNames.PanelNetwork, Driver: "overlay", Attachable: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.fake.CreateContainer(ctx, spec); err != nil {
			t.Fatal(err)
		}
	}
	// manual es create y lo arranca.
	manual := func(mod func(*docker.ContainerSpec)) func(*testing.T, *runtimeFixture) {
		return func(t *testing.T, f *runtimeFixture) {
			create(t, f, mod)
			if err := f.fake.StartContainer(ctx, testNames.Container); err != nil {
				t.Fatal(err)
			}
		}
	}
	cases := []struct {
		name    string
		setup   func(*testing.T, *runtimeFixture)
		want    service.CaddyWant
		running bool // corría antes de Ensure
	}{
		{"otra imagen", func(t *testing.T, f *runtimeFixture) { f.fake.SetImagePresent(testImage); f.ensure(t, baseWant()) },
			service.CaddyWant{Image: testImage2, HTTPPort: 8080, HTTPSPort: 8443}, true},
		{"otros puertos", func(t *testing.T, f *runtimeFixture) { f.fake.SetImagePresent(testImage); f.ensure(t, baseWant()) },
			service.CaddyWant{Image: testImage, HTTPPort: 9080, HTTPSPort: 8443}, true},
		{"otro Cmd", manual(func(s *docker.ContainerSpec) {
			s.Cmd = []string{"caddy", "run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"}
		}), baseWant(), true},
		{"otros volúmenes", manual(func(s *docker.ContainerSpec) { s.Mounts = s.Mounts[:2] }), baseWant(), true},
		{"detenido", func(t *testing.T, f *runtimeFixture) {
			f.fake.SetImagePresent(testImage)
			f.ensure(t, baseWant())
			if err := f.fake.StopContainer(ctx, testNames.Container, 0); err != nil {
				t.Fatal(err)
			}
		}, baseWant(), false},
		{"corriendo sin PublishedPorts", func(t *testing.T, f *runtimeFixture) {
			// Como Docker 29: el arranque falla por un puerto ocupado y, si
			// alguien lo vuelve a arrancar, corre sin publicar sus puertos.
			create(t, f, func(*docker.ContainerSpec) {})
			f.fake.BusyHostPort(8080)
			if err := f.fake.StartContainer(ctx, testNames.Container); !errors.Is(err, docker.ErrPortInUse) {
				t.Fatalf("el puerto ocupado: %v", err)
			}
			f.fake.FreeHostPort(8080)
			if err := f.fake.StartContainer(ctx, testNames.Container); err != nil {
				t.Fatal(err)
			}
			if c := f.container(t); !c.Running || len(c.PublishedPorts) != 0 {
				t.Fatalf("debería correr sin puertos: %+v", c)
			}
		}, baseWant(), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeFixture(t)
			tc.setup(t, f)
			before := f.container(t)
			n := len(f.fake.Calls())

			if !f.ensure(t, tc.want) {
				t.Fatal("recreated debería ser true")
			}
			calls := f.since(n)
			stop, remove := callIndex(calls, "StopContainer "+testNames.Container), callIndex(calls, "RemoveContainer "+testNames.Container)
			if remove < 0 || (tc.running && (stop < 0 || stop > remove)) || (!tc.running && stop >= 0) {
				t.Fatalf("detener (si corría) y después borrar: %v", calls)
			}
			after := f.container(t)
			if after.ID == before.ID || !after.Running || len(after.PublishedPorts) == 0 || after.Image != tc.want.Image {
				t.Fatalf("antes %+v, después %+v", before, after)
			}
			if tc.name == "otra imagen" {
				if pull := callIndex(calls, "PullImage "+testImage2); pull < 0 || pull > stop {
					t.Fatalf("la imagen nueva se baja antes de detenerlo: %v", calls)
				}
			}
		})
	}
}

// Si la imagen nueva no baja, el contenedor anterior sigue sirviendo los
// sitios y el panel (D11).
func TestEnsureRecreateImagePullFails(t *testing.T) {
	for _, pullErr := range []error{docker.ErrDenied, docker.ErrNotFound} {
		t.Run(pullErr.Error(), func(t *testing.T) {
			f := newRuntimeFixture(t)
			f.fake.SetImagePresent(testImage)
			f.ensure(t, baseWant())
			before := f.container(t)
			n := len(f.fake.Calls())
			f.fake.FailNext("PullImage", pullErr)

			_, err := f.runtime.Ensure(context.Background(), service.CaddyWant{Image: testImage2, HTTPPort: 8080, HTTPSPort: 8443})
			if !errors.Is(err, service.ErrCaddyImagePull) || !errors.Is(err, pullErr) || !strings.Contains(err.Error(), testImage2) {
				t.Fatalf("error: %v", err)
			}
			for _, op := range []string{"StopContainer", "RemoveContainer", "CreateContainer"} {
				if hasCall(f.since(n), op) {
					t.Fatalf("no debería tocar el contenedor (%s): %v", op, f.since(n))
				}
			}
			after := f.container(t)
			if after.ID != before.ID || !after.Running || after.Image != testImage {
				t.Fatalf("el anterior sigue: %+v", after)
			}
		})
	}
}

// La imagen se baja solo si falta, con la credencial del registry si hay.
func TestEnsurePullsImage(t *testing.T) {
	f := newRuntimeFixture(t)
	want := baseWant()
	want.Auth = &service.Credential{Host: "ghcr.io", Username: "bot", Password: testPassword}
	f.ensure(t, want)
	if got := f.fake.Pulls(); !slices.Equal(got, []string{testImage}) {
		t.Fatalf("pulls: %v", got)
	}
	if len(f.engine.auths) != 1 || f.engine.auths[0] == nil ||
		*f.engine.auths[0] != (docker.RegistryAuth{Host: "ghcr.io", Username: "bot", Password: testPassword}) {
		t.Fatalf("la credencial llega al pull: %+v", f.engine.auths)
	}

	// Anónima: sin credencial.
	g := newRuntimeFixture(t)
	g.ensure(t, baseWant())
	if len(g.engine.auths) != 1 || g.engine.auths[0] != nil {
		t.Fatalf("sin credencial: %+v", g.engine.auths)
	}

	// Ya en el nodo: no se baja.
	h := newRuntimeFixture(t)
	h.fake.SetImagePresent(testImage)
	h.ensure(t, baseWant())
	if got := h.fake.Pulls(); len(got) != 0 {
		t.Fatalf("pulls con la imagen en el nodo: %v", got)
	}
}

// Un puerto del host ocupado: el contenedor que no arrancó se borra enseguida
// (si no, alguien podría arrancarlo sin puertos), y se reintenta con los
// mismos ajustes.
func TestEnsurePortInUse(t *testing.T) {
	f := newRuntimeFixture(t)
	f.fake.SetImagePresent(testImage)
	f.fake.BusyHostPort(8080)
	_, err := f.runtime.Ensure(context.Background(), baseWant())
	if !errors.Is(err, service.ErrPortInUse) {
		t.Fatalf("error: %v", err)
	}
	ctx := context.Background()
	for _, name := range []string{testNames.Container, testProbe} {
		if _, err := f.fake.InspectContainer(ctx, name); !errors.Is(err, docker.ErrNotFound) {
			t.Fatalf("%s debería estar borrado: %v", name, err)
		}
	}
	// Lo encontró la prueba: muelle-caddy ni se creó, así que ninguna red de
	// Swarm quedó a medias.
	if len(f.engine.created(testNames.Container)) != 0 {
		t.Fatalf("no debería crear muelle-caddy: %v", f.fake.Calls())
	}

	f.fake.FreeHostPort(8080)
	if !f.ensure(t, baseWant()) {
		t.Fatal("recreated debería ser true")
	}
	if c := f.container(t); !c.Running || len(c.PublishedPorts) == 0 {
		t.Fatalf("contenedor: %+v", c)
	}
}

// testProbe es el contenedor con que Ensure prueba los puertos.
const testProbe = "t-caddy-probe"

// Antes de crear muelle-caddy, Ensure prueba los puertos con un contenedor
// descartable sin redes de Swarm (la red bridge por defecto): si el arranque
// de muelle-caddy fallara por un puerto ocupado, Docker no suelta la
// conexión de Swarm de la red panel (una tarea "attachment" que no se borra
// y se queda con una IP de esa red). La prueba se borra siempre.
func TestEnsureProbesPorts(t *testing.T) {
	f := newRuntimeFixture(t)
	f.fake.SetImagePresent(testImage)
	f.ensure(t, baseWant())
	probes := f.engine.created(testProbe)
	if len(probes) != 1 {
		t.Fatalf("una prueba: %+v", probes)
	}
	p := probes[0]
	real := f.engine.created(testNames.Container)[0]
	if p.Image != testImage || p.Network != "" || len(p.Mounts) != 0 || !slices.Equal(p.Ports, real.Ports) ||
		!slices.Equal(p.Cmd, []string{"caddy", "version"}) || p.RestartPolicy != "" {
		t.Fatalf("la prueba: %+v", p)
	}
	calls := f.fake.Calls()
	start, remove := callIndex(calls, "StartContainer "+testProbe), lastCallIndex(calls, "RemoveContainer "+testProbe)
	create := callIndex(calls, "CreateContainer "+testNames.Container)
	if start < 0 || remove < start || create < remove {
		t.Fatalf("probar, borrar la prueba y recién crear muelle-caddy: %v", calls)
	}
	if _, err := f.fake.InspectContainer(context.Background(), testProbe); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("la prueba queda borrada: %v", err)
	}

	// Una prueba que falla por otra cosa (sin red bridge, por ejemplo) no
	// frena a muelle-caddy: queda en el log.
	g := newRuntimeFixture(t)
	g.fake.SetImagePresent(testImage)
	g.engine.failOn["CreateContainer "+testProbe] = errors.New("network bridge not found")
	if !g.ensure(t, baseWant()) {
		t.Fatal("recreated debería ser true")
	}
	if !strings.Contains(g.log.String(), "network bridge not found") {
		t.Fatalf("el log: %s", g.log)
	}
}

func TestEnsureDockerDown(t *testing.T) {
	f := newRuntimeFixture(t)
	f.fake.SetUnavailable(true)
	if _, err := f.runtime.Ensure(context.Background(), baseWant()); !errors.Is(err, entity.ErrDockerUnavailable) {
		t.Fatalf("error: %v", err)
	}

	// Docker se cae justo en el pull: no es "la imagen no bajó".
	g := newRuntimeFixture(t)
	g.fake.FailNext("PullImage", docker.ErrUnavailable)
	_, err := g.runtime.Ensure(context.Background(), baseWant())
	if !errors.Is(err, entity.ErrDockerUnavailable) || errors.Is(err, service.ErrCaddyImagePull) {
		t.Fatalf("Docker caído en el pull: %v", err)
	}
}

// La contraseña del registry nunca sale en un error ni en el log, falle lo
// que falle.
func TestEnsureHidesRegistryPassword(t *testing.T) {
	for _, op := range []string{"PullImage", "CreateContainer", "StartContainer"} {
		t.Run(op, func(t *testing.T) {
			f := newRuntimeFixture(t)
			if op != "PullImage" {
				f.fake.SetImagePresent(testImage)
			}
			f.engine.failOn[op+" "+testNames.Container] = errors.New("falló " + op)
			if op == "PullImage" {
				f.fake.FailNext(op, errors.New("falló "+op))
			}
			want := baseWant()
			want.Auth = &service.Credential{Host: "ghcr.io", Username: "bot", Password: testPassword}
			_, err := f.runtime.Ensure(context.Background(), want)
			if err == nil {
				t.Fatal("debería fallar")
			}
			if strings.Contains(err.Error(), testPassword) || strings.Contains(f.log.String(), testPassword) {
				t.Fatalf("la contraseña sale: %v\n%s", err, f.log)
			}
		})
	}
}

func TestStatus(t *testing.T) {
	f := newRuntimeFixture(t)
	ctx := context.Background()
	if image, running, found, err := f.runtime.Status(ctx); err != nil || found || running || image != "" {
		t.Fatalf("sin contenedor: %q %v %v %v", image, running, found, err)
	}
	f.fake.SetImagePresent(testImage)
	f.ensure(t, baseWant())
	if image, running, found, err := f.runtime.Status(ctx); err != nil || !found || !running || image != testImage {
		t.Fatalf("corriendo: %q %v %v %v", image, running, found, err)
	}
	if err := f.fake.StopContainer(ctx, testNames.Container, 0); err != nil {
		t.Fatal(err)
	}
	if _, running, found, err := f.runtime.Status(ctx); err != nil || !found || running {
		t.Fatalf("detenido: %v %v %v", running, found, err)
	}
	f.fake.SetUnavailable(true)
	if _, _, _, err := f.runtime.Status(ctx); !errors.Is(err, entity.ErrDockerUnavailable) {
		t.Fatalf("Docker caído: %v", err)
	}
}
