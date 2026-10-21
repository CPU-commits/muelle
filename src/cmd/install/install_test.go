package install

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
)

const testLink = "https://panel.example.com/setup?token=ABC"

var muelleNames = naming.InstallNamesFor(naming.MuelleStack)

type harness struct {
	f              *dockertest.Fake
	o              Options
	c              Config
	stdout, stderr *bytes.Buffer
	detects        int
	start          int // las llamadas que hizo el test antes de run
}

// newInstall arma un nodo con un Swarm activo y las dos imágenes de la
// versión v0.1.0 ya bajadas, y un muelle que al desplegarse escribe el link
// de setup y empieza a escuchar.
func newInstall(t *testing.T) *harness {
	t.Helper()
	f := dockertest.New()
	f.SetImagePresent(muelleRef)
	f.SetImagePresent(caddyRef)
	return newInstallWith(t, f)
}

// newInstallWith es newInstall sobre un fake que el test ya preparó.
func newInstallWith(t *testing.T, f *dockertest.Fake) *harness {
	t.Helper()
	h := &harness{f: f, stdout: new(bytes.Buffer), stderr: new(bytes.Buffer)}
	h.o = Options{PanelDomain: "panel.example.com", Email: "a@example.com", HTTPPort: 80, HTTPSPort: 443, NamePrefix: naming.MuelleStack}
	h.c = Config{
		Version: "v0.1.0", Docker: f, Stdout: h.stdout, Stderr: h.stderr,
		DetectIP: func(ctx context.Context) (string, error) {
			h.detects++
			return "203.0.113.24", nil
		},
		SetupTimeout: 2 * time.Second, Poll: 5 * time.Millisecond, NetworkWait: 200 * time.Millisecond,
		// Nunca el config.json de quien corre los tests.
		DockerConfig: filepath.Join(t.TempDir(), "no-existe.json"),
	}
	muelleStarts(f, muelleNames)
	return h
}

func (h *harness) run() error {
	h.start = len(h.f.Calls())
	return Run(context.Background(), h.o, h.c)
}

// calls son las llamadas a Docker de la última corrida, sin las que hizo el
// test para preparar el nodo.
func (h *harness) calls() []string { return h.f.Calls()[h.start:] }

// muelleStarts hace que el deploy del stack cree el servicio con una tarea
// que corre y escribe esas líneas (por defecto, el link y listening).
func muelleStarts(f *dockertest.Fake, names naming.InstallNames, logLines ...string) {
	if logLines == nil {
		logLines = []string{setupLine(testLink), listeningLine}
	}
	f.OnDeployStack = func(d docker.StackDeploy) error {
		f.PutStackService(names.Stack, docker.Service{Name: names.Service}, running("t1", 0))
		f.AppendTaskLogs("t1", lines(logLines...)...)
		return nil
	}
}

// inOrder verifica que want aparezca en calls en ese orden (puede haber
// otras llamadas entre medio).
func inOrder(t *testing.T, calls []string, want ...string) {
	t.Helper()
	i := 0
	for _, c := range calls {
		if i < len(want) && c == want[i] {
			i++
		}
	}
	if i < len(want) {
		t.Fatalf("falta %q (en orden) en las llamadas:\n%s", want[i], strings.Join(calls, "\n"))
	}
}

func noCall(t *testing.T, calls []string, prefixes ...string) {
	t.Helper()
	for _, c := range calls {
		for _, p := range prefixes {
			if c == p || strings.HasPrefix(c, p+" ") {
				t.Errorf("no debió llamar a %q:\n%s", c, strings.Join(calls, "\n"))
			}
		}
	}
}

func indexOf(calls []string, call string) int { return slices.Index(calls, call) }

func compose(t *testing.T, f *dockertest.Fake) string {
	t.Helper()
	d := f.StackDeploys()
	if len(d) == 0 {
		t.Fatal("no hubo DeployStack")
	}
	return string(d[len(d)-1].Compose)
}

func gone(t *testing.T, err error, what string) {
	t.Helper()
	if !errors.Is(err, docker.ErrNotFound) {
		t.Errorf("%s sigue ahí (%v)", what, err)
	}
}

func TestRunFreshInstall(t *testing.T) {
	h := newInstall(t)
	if err := h.run(); err != nil {
		t.Fatalf("%v\nstderr: %s", err, h.stderr)
	}
	calls := h.calls()
	inOrder(t, calls, "Info",
		"InspectService muelle_muelle", "InspectSecret muelle_master_key", "ListVolumes", "InspectNetwork muelle_panel",
		"InspectImage "+muelleRef,
		"RemoveContainer muelle-install-probe", "CreateContainer muelle-install-probe "+muelleRef,
		"StartContainer muelle-install-probe", "RemoveContainer muelle-install-probe",
		"CreateNetwork muelle_panel", "CreateSecret muelle_master_key", "DeployStack muelle")
	noCall(t, calls, "InitSwarm", "PullImage", "RemoveStack")

	ctx := context.Background()
	n, err := h.f.InspectNetwork(ctx, "muelle_panel")
	if err != nil || n.Driver != "overlay" || !n.Attachable || n.Scope != "swarm" {
		t.Fatalf("red=%+v err=%v", n, err)
	}
	_, err = h.f.InspectContainer(ctx, naming.InstallProbeContainer)
	gone(t, err, "el contenedor de prueba")

	key, ok := h.f.SecretData("muelle_master_key")
	raw, err := base64.StdEncoding.DecodeString(string(key))
	if !ok || err != nil || len(raw) != 32 {
		t.Fatalf("clave=%q err=%v", key, err)
	}
	out := h.stdout.String()
	ik, il := strings.Index(out, "    "+string(key)+"\n"), strings.Index(out, "    "+testLink+"\n")
	if ik < 0 || il < 0 || ik > il {
		t.Fatalf("stdout sin la clave antes del link:\n%s", out)
	}
	if strings.Contains(h.stderr.String(), string(key)) {
		t.Fatal("la clave salió por stderr")
	}
	c := compose(t, h.f)
	for _, want := range []string{"node.id == node-1", `"image": "` + muelleRef + `"`, `"MUELLE_INIT_PUBLIC_IP": "203.0.113.24"`,
		`"MUELLE_PANEL_DOMAIN": "panel.example.com"`} {
		if !strings.Contains(c, want) {
			t.Errorf("el compose no trae %s:\n%s", want, c)
		}
	}
	if strings.Contains(c, "MUELLE_PANEL_ORIGIN") {
		t.Errorf("con --panel-domain, MUELLE_PANEL_ORIGIN no va (el origen sale de los ajustes):\n%s", c)
	}
	if strings.Contains(c, "MUELLE_INIT_REGISTRY_FILE") || strings.Contains(c, "MUELLE_INIT_CLOUDFLARE_TOKEN_FILE") {
		t.Errorf("sin token ni credencial, no monta sus secretos:\n%s", c)
	}
	if d := h.f.StackDeploys()[0]; len(d.Registries) != 0 {
		t.Errorf("Registries=%d sin credencial", len(d.Registries))
	}
	for _, want := range []string{"Docker 29.6.1", "IP pública: 203.0.113.24", "puertos 80 y 443 libres", "red muelle_panel creada"} {
		if !strings.Contains(h.stderr.String(), want) {
			t.Errorf("stderr sin %q:\n%s", want, h.stderr)
		}
	}
}

func TestRunPullsMissingImage(t *testing.T) {
	f := dockertest.New()
	h := newInstallWith(t, f)
	if err := h.run(); err != nil {
		t.Fatal(err)
	}
	// Las dos: la de Caddy también, así ingress la encuentra en el primer arranque.
	inOrder(t, f.Calls(), "InspectImage "+muelleRef, "PullImage "+muelleRef, "InspectImage "+caddyRef, "PullImage "+caddyRef,
		"CreateContainer muelle-install-probe "+muelleRef)
}

func TestRunDevNeedsImage(t *testing.T) {
	h := newInstall(t)
	h.c.Version = "dev"
	err := h.run()
	if err == nil || !strings.Contains(err.Error(), "--image") {
		t.Fatalf("err=%v", err)
	}
	if calls := h.calls(); len(calls) != 0 {
		t.Fatalf("tocó Docker: %q", calls)
	}

	h = newInstall(t)
	h.c.Version = "dev"
	h.o.Image = "it5-muelle:test"
	h.f.SetImagePresent("it5-muelle:test")
	if err := h.run(); err != nil {
		t.Fatal(err)
	}
	inOrder(t, h.calls(), "CreateContainer muelle-install-probe it5-muelle:test")
	// dev no tiene una imagen de Caddy por defecto: no busca ninguna.
	noCall(t, h.calls(), "InspectImage "+caddyRef, "PullImage")
	if c := compose(t, h.f); !strings.Contains(c, `"image": "it5-muelle:test"`) {
		t.Fatalf("compose:\n%s", c)
	}
}

func TestRunDockerDown(t *testing.T) {
	h := newInstall(t)
	h.f.SetUnavailable(true)
	if err := h.run(); err == nil || !strings.Contains(err.Error(), "/var/run/docker.sock") {
		t.Fatalf("err=%v", err)
	}
}

func TestRunSwarmStates(t *testing.T) {
	for name, info := range map[string]docker.Info{
		"worker":  {ServerVersion: "29.6.1", SwarmState: docker.SwarmActive, NodeID: "n2", IsManager: false},
		"pending": {ServerVersion: "29.6.1", SwarmState: docker.SwarmPending},
		"error":   {ServerVersion: "29.6.1", SwarmState: docker.SwarmError},
		"locked":  {ServerVersion: "29.6.1", SwarmState: docker.SwarmLocked},
	} {
		t.Run(name, func(t *testing.T) {
			h := newInstall(t)
			h.f.SetInfo(info)
			err := h.run()
			if err == nil {
				t.Fatal("sin error")
			}
			if calls := h.calls(); len(calls) != 1 || calls[0] != "Info" {
				t.Fatalf("siguió después de Info: %q", calls)
			}
		})
	}
}

func TestRunInitsSwarm(t *testing.T) {
	h := newInstall(t)
	h.f.SetInfo(docker.Info{ServerVersion: "29.6.1", SwarmState: docker.SwarmInactive})
	h.o.AdvertiseAddr = "10.0.0.5"
	if err := h.run(); err != nil {
		t.Fatalf("%v\nstderr: %s", err, h.stderr)
	}
	calls := h.calls()
	inOrder(t, calls, "InspectNetwork muelle_panel", "StartContainer muelle-install-probe", "RemoveContainer muelle-install-probe",
		"InitSwarm 10.0.0.5", "CreateNetwork muelle_panel")
	// Sin Swarm no hay servicios ni secretos que buscar (el daemon respondería 503).
	if i := indexOf(calls, "InitSwarm 10.0.0.5"); slices.ContainsFunc(calls[:i], func(c string) bool {
		return strings.HasPrefix(c, "InspectService") || strings.HasPrefix(c, "InspectSecret")
	}) {
		t.Fatalf("buscó servicios o secretos sin Swarm: %q", calls[:i])
	}
	if c := compose(t, h.f); !strings.Contains(c, "node.id == node-1") {
		t.Fatalf("la restricción no usa el nodo que devolvió InitSwarm:\n%s", c)
	}
}

func TestRunInitSwarmFails(t *testing.T) {
	h := newInstall(t)
	h.f.SetInfo(docker.Info{ServerVersion: "29.6.1", SwarmState: docker.SwarmInactive})
	dockerErr := "could not choose an IP address to advertise since this system has multiple addresses"
	h.f.FailNext("InitSwarm", errors.New(dockerErr))
	err := h.run()
	if err == nil || !strings.Contains(err.Error(), dockerErr) || !strings.Contains(err.Error(), "--advertise-addr <ip>") {
		t.Fatalf("err=%v", err)
	}
	_, ierr := h.f.InspectContainer(context.Background(), naming.InstallProbeContainer)
	gone(t, ierr, "el contenedor de prueba")
	noCall(t, h.calls(), "CreateNetwork", "CreateSecret", "DeployStack")
}

func TestRunAdvertiseAddrIgnored(t *testing.T) {
	h := newInstall(t)
	h.o.AdvertiseAddr = "10.0.0.5"
	if err := h.run(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.stderr.String(), "aviso: --advertise-addr") {
		t.Errorf("stderr=%s", h.stderr)
	}
	noCall(t, h.calls(), "InitSwarm")
}

func TestRunAlreadyInstalled(t *testing.T) {
	for _, reinstall := range []bool{false, true} {
		h := newInstall(t)
		h.o.Reinstall = reinstall
		h.f.PutService(docker.Service{Name: "muelle_muelle"})
		err := h.run()
		if !errors.Is(err, ErrAlreadyInstalled) || !strings.Contains(err.Error(), "docker service update --with-registry-auth --image") {
			t.Fatalf("reinstall=%v: err=%v", reinstall, err)
		}
		noCall(t, h.calls(), "CreateContainer", "CreateNetwork", "CreateSecret", "DeployStack", "PullImage")
	}
}

func TestRunPreviousDataNeedsReinstall(t *testing.T) {
	for name, prepare := range map[string]func(f *dockertest.Fake){
		"secreto": func(f *dockertest.Fake) {
			f.CreateSecret(context.Background(), "muelle_master_key", []byte("vieja"), nil)
		},
		"volumen": func(f *dockertest.Fake) { f.PutVolume(docker.Volume{Name: "muelle_data"}) },
	} {
		t.Run(name, func(t *testing.T) {
			h := newInstall(t)
			prepare(h.f)
			err := h.run()
			if !errors.Is(err, ErrPreviousData) || !strings.Contains(err.Error(), "--reinstall") {
				t.Fatalf("err=%v", err)
			}
			noCall(t, h.calls(), "CreateContainer", "CreateNetwork", "CreateSecret", "DeployStack", "PullImage")
		})
	}
}

func TestRunReinstall(t *testing.T) {
	h := newInstall(t)
	h.f.CreateSecret(context.Background(), "muelle_master_key", []byte("vieja"), nil)
	h.f.PutVolume(docker.Volume{Name: "muelle_data"})
	h.o.Reinstall = true
	if err := h.run(); err != nil {
		t.Fatalf("%v\nstderr: %s", err, h.stderr)
	}
	noCall(t, h.calls(), "CreateSecret muelle_master_key", "RemoveSecret")
	if out := h.stdout.String(); strings.Contains(out, "Clave maestra") || !strings.Contains(out, testLink) {
		t.Fatalf("stdout=%s", out)
	}
	if !strings.Contains(h.stderr.String(), "la base ya existe") {
		t.Fatalf("stderr=%s", h.stderr)
	}
	if data, _ := h.f.SecretData("muelle_master_key"); string(data) != "vieja" {
		t.Fatal("cambió la clave")
	}
}

func TestRunReinstallWithoutSecret(t *testing.T) {
	h := newInstall(t)
	h.f.PutVolume(docker.Volume{Name: "muelle_data"})
	h.o.Reinstall = true
	err := h.run()
	if err == nil || !strings.Contains(err.Error(), "muelle_master_key") {
		t.Fatalf("err=%v", err)
	}
	noCall(t, h.calls(), "CreateContainer", "CreateNetwork", "CreateSecret", "DeployStack")
}

// Con --reinstall y un muelle-caddy que ya existe, ese contenedor tiene los
// puertos: la prueba no se hace.
func TestRunReinstallSkipsProbe(t *testing.T) {
	h := newInstall(t)
	ctx := context.Background()
	h.f.CreateSecret(ctx, "muelle_master_key", []byte("vieja"), nil)
	if _, err := h.f.CreateContainer(ctx, docker.ContainerSpec{Name: naming.CaddyContainer, Image: caddyRef}); err != nil {
		t.Fatal(err)
	}
	h.f.BusyHostPort(443)
	h.o.Reinstall = true
	if err := h.run(); err != nil {
		t.Fatalf("%v\nstderr: %s", err, h.stderr)
	}
	noCall(t, h.calls(), "CreateContainer muelle-install-probe")
}

func TestRunTokenSecret(t *testing.T) {
	token := strings.Repeat("c", 40)
	for _, leftover := range []bool{false, true} {
		h := newInstall(t)
		h.o.CloudflareToken = token
		if leftover {
			h.f.CreateSecret(context.Background(), "muelle_init_cloudflare_token", []byte("viejo"), nil)
		}
		if err := h.run(); err != nil {
			t.Fatalf("leftover=%v: %v", leftover, err)
		}
		if data, _ := h.f.SecretData("muelle_init_cloudflare_token"); string(data) != token {
			t.Fatalf("leftover=%v: secreto=%q", leftover, data)
		}
		c := compose(t, h.f)
		if !strings.Contains(c, `"MUELLE_INIT_CLOUDFLARE_TOKEN_FILE": "/run/secrets/muelle_init_cloudflare_token"`) ||
			!strings.Contains(c, `"name": "muelle_init_cloudflare_token"`) || strings.Contains(c, token) {
			t.Fatalf("compose:\n%s", c)
		}
		if leftover {
			inOrder(t, h.calls(), "RemoveSecret muelle_init_cloudflare_token", "CreateSecret muelle_init_cloudflare_token")
		} else {
			noCall(t, h.calls(), "RemoveSecret")
		}
		if strings.Contains(h.stdout.String()+h.stderr.String(), token) {
			t.Fatal("el token salió en la salida")
		}
	}
}

func TestRunReinstallKeepsTokenSecret(t *testing.T) {
	h := newInstall(t)
	ctx := context.Background()
	h.f.CreateSecret(ctx, "muelle_master_key", []byte("vieja"), nil)
	h.f.CreateSecret(ctx, "muelle_init_cloudflare_token", []byte("viejo"), nil)
	h.o.Reinstall = true
	if err := h.run(); err != nil {
		t.Fatal(err)
	}
	noCall(t, h.calls(), "RemoveSecret", "CreateSecret")
	if c := compose(t, h.f); strings.Contains(c, "init_cloudflare_token") {
		t.Fatalf("montó el secreto que quedó:\n%s", c)
	}
}

func TestRunPortsBusy(t *testing.T) {
	for _, withCaddy := range []bool{false, true} {
		h := newInstall(t)
		h.f.BusyHostPort(443)
		if withCaddy {
			if _, err := h.f.CreateContainer(context.Background(), docker.ContainerSpec{Name: naming.CaddyContainer, Image: caddyRef}); err != nil {
				t.Fatal(err)
			}
		}
		err := h.run()
		if !errors.Is(err, ErrPortsBusy) || !strings.Contains(err.Error(), "--http-port") || !strings.Contains(err.Error(), "--https-port") {
			t.Fatalf("err=%v", err)
		}
		if got := strings.Contains(err.Error(), "docker rm -f muelle-caddy"); got != withCaddy {
			t.Fatalf("withCaddy=%v: err=%v", withCaddy, err)
		}
		_, ierr := h.f.InspectContainer(context.Background(), naming.InstallProbeContainer)
		gone(t, ierr, "el contenedor de prueba")
		noCall(t, h.calls(), "InitSwarm", "CreateNetwork", "CreateSecret", "DeployStack")
	}
}

func TestProbeSpec(t *testing.T) {
	s := probeSpec("it5abc-install-probe", "it5-muelle:test", 22080, 22443)
	want := []docker.PortBinding{
		{HostPort: 22080, ContainerPort: 80, Protocol: "tcp"},
		{HostPort: 22443, ContainerPort: 443, Protocol: "tcp"},
		{HostPort: 22443, ContainerPort: 443, Protocol: "udp"},
	}
	if s.Name != "it5abc-install-probe" || s.Image != "it5-muelle:test" || !slices.Equal(s.Cmd, []string{"help"}) ||
		!slices.Equal(s.Ports, want) || s.Network != "" || s.RestartPolicy != "" || len(s.Mounts) != 0 {
		t.Fatalf("spec=%+v", s)
	}
}

func TestRunRemovesStaleProbe(t *testing.T) {
	h := newInstall(t)
	if _, err := h.f.CreateContainer(context.Background(), docker.ContainerSpec{Name: naming.InstallProbeContainer, Image: muelleRef}); err != nil {
		t.Fatal(err)
	}
	if err := h.run(); err != nil {
		t.Fatal(err)
	}
	inOrder(t, h.calls(), "RemoveContainer muelle-install-probe", "CreateContainer muelle-install-probe "+muelleRef)
}

func TestRunHTTP01Warning(t *testing.T) {
	h := newInstall(t)
	h.o.HTTPPort, h.o.HTTPSPort = 8080, 8443
	if err := h.run(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(h.stderr.String(), "HTTP-01") {
		t.Errorf("sin aviso de HTTP-01: %s", h.stderr)
	}
	// El puerto no va en la configuración de muelle: el origen del panel
	// sale de los ajustes (MUELLE_INIT_PROXY_HTTPS_PORT, después el panel).
	if c := compose(t, h.f); !strings.Contains(c, `"MUELLE_PANEL_DOMAIN": "panel.example.com"`) || strings.Contains(c, "MUELLE_PANEL_ORIGIN") ||
		!strings.Contains(c, `"MUELLE_INIT_PROXY_HTTPS_PORT": "8443"`) {
		t.Errorf("compose:\n%s", c)
	}

	h = newInstall(t)
	h.o.HTTPPort, h.o.HTTPSPort = 8080, 8443
	h.o.CloudflareToken = strings.Repeat("c", 40)
	if err := h.run(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(h.stderr.String(), "HTTP-01") {
		t.Errorf("con token, sin aviso: %s", h.stderr)
	}
}

func TestRunPublicIP(t *testing.T) {
	h := newInstall(t)
	if err := h.run(); err != nil {
		t.Fatal(err)
	}
	if h.detects != 1 || !strings.Contains(compose(t, h.f), `"MUELLE_INIT_PUBLIC_IP": "203.0.113.24"`) {
		t.Fatalf("detects=%d", h.detects)
	}

	h = newInstall(t)
	h.c.DetectIP = func(ctx context.Context) (string, error) { return "", errors.New("sin red") }
	if err := h.run(); err != nil {
		t.Fatal(err)
	}
	if c := compose(t, h.f); strings.Contains(c, "MUELLE_INIT_PUBLIC_IP") || !strings.Contains(h.stderr.String(), "aviso:") {
		t.Fatalf("stderr=%s\ncompose:\n%s", h.stderr, c)
	}

	h = newInstall(t)
	h.o.PublicIP = "198.51.100.7"
	if err := h.run(); err != nil {
		t.Fatal(err)
	}
	if h.detects != 0 || !strings.Contains(compose(t, h.f), `"MUELLE_INIT_PUBLIC_IP": "198.51.100.7"`) {
		t.Fatalf("detects=%d", h.detects)
	}
}

func TestRunExistingNetwork(t *testing.T) {
	for _, deployFails := range []bool{false, true} {
		h := newInstall(t)
		ctx := context.Background()
		if _, err := h.f.CreateNetwork(ctx, docker.NetworkSpec{Name: "muelle_panel", Driver: "overlay", Attachable: true}); err != nil {
			t.Fatal(err)
		}
		if deployFails {
			h.f.FailNext("DeployStack", errors.New("falló"))
		}
		err := h.run()
		if (err != nil) != deployFails {
			t.Fatalf("deployFails=%v: err=%v", deployFails, err)
		}
		// El único CreateNetwork es el del test, antes de run.
		noCall(t, h.calls(), "CreateNetwork")
		if _, err := h.f.InspectNetwork(ctx, "muelle_panel"); err != nil {
			t.Fatalf("deployFails=%v: borró la red que ya existía: %v", deployFails, err)
		}
		noCall(t, h.calls(), "RemoveNetwork")
	}
}

func TestRunWrongNetwork(t *testing.T) {
	for name, tc := range map[string]struct {
		net      docker.Network
		inactive bool
	}{
		"bridge sin Swarm":      {docker.Network{Name: "muelle_panel", Driver: "bridge", Scope: "local", Subnets: []string{"172.30.0.0/16"}}, true},
		"bridge":                {docker.Network{Name: "muelle_panel", Driver: "bridge", Scope: "local", Subnets: []string{"172.30.0.0/16"}}, false},
		"overlay no attachable": {docker.Network{Name: "muelle_panel", Driver: "overlay", Scope: "swarm", Subnets: []string{"10.0.9.0/24"}}, false},
	} {
		t.Run(name, func(t *testing.T) {
			h := newInstall(t)
			if tc.inactive {
				h.f.SetInfo(docker.Info{ServerVersion: "29.6.1", SwarmState: docker.SwarmInactive})
			}
			h.f.PutNetwork(tc.net)
			err := h.run()
			if err == nil || !strings.Contains(err.Error(), "muelle_panel") {
				t.Fatalf("err=%v", err)
			}
			noCall(t, h.calls(), "PullImage", "CreateContainer", "InitSwarm", "CreateNetwork", "CreateSecret")
		})
	}

	t.Run("subred pública", func(t *testing.T) {
		h := newInstall(t)
		h.f.PutNetwork(docker.Network{Name: "muelle_panel", Driver: "overlay", Scope: "swarm", Attachable: true, Subnets: []string{"203.0.113.0/24"}})
		err := h.run()
		if err == nil || !strings.Contains(err.Error(), "203.0.113.0/24") {
			t.Fatalf("err=%v", err)
		}
		calls := h.calls()
		inOrder(t, calls, "StartContainer muelle-install-probe", "RemoveContainer muelle-install-probe")
		noCall(t, calls, "CreateSecret", "DeployStack", "RemoveNetwork")
		if _, err := h.f.InspectNetwork(context.Background(), "muelle_panel"); err != nil {
			t.Fatalf("borró la red que ya existía: %v", err)
		}
	})
}

func TestRunDeployFailsRollsBack(t *testing.T) {
	ctx := context.Background()
	check := func(t *testing.T, h *harness, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("sin error")
		}
		if strings.Contains(h.stdout.String(), "Clave maestra") || h.stdout.Len() != 0 {
			t.Fatalf("stdout=%s", h.stdout)
		}
		_, serr := h.f.InspectSecret(ctx, "muelle_master_key")
		gone(t, serr, "el secreto muelle_master_key")
		_, nerr := h.f.InspectNetwork(ctx, "muelle_panel")
		gone(t, nerr, "la red muelle_panel")
	}

	t.Run("el CLI no creó nada", func(t *testing.T) {
		h := newInstall(t)
		h.f.FailNext("DeployStack", errors.New("Error response from daemon: rpc error"))
		err := h.run()
		check(t, h, err)
		if !strings.Contains(err.Error(), "failed to deploy stack") {
			t.Fatalf("el error no trae la salida del CLI: %v", err)
		}
		noCall(t, h.calls(), "RemoveStack")
	})

	t.Run("el CLI creó el servicio", func(t *testing.T) {
		h := newInstall(t)
		h.f.OnDeployStack = func(d docker.StackDeploy) error {
			h.f.PutStackService("muelle", docker.Service{Name: "muelle_muelle"})
			return errors.New("se cortó")
		}
		check(t, h, h.run())
		calls := h.calls()
		inOrder(t, calls, "DeployStack muelle", "RemoveStack muelle", "RemoveSecret muelle_master_key", "RemoveNetwork muelle_panel")
		_, err := h.f.InspectService(ctx, "muelle_muelle")
		gone(t, err, "el servicio")
	})

	// La tarea alcanzó a arrancar: el daemon creó los volúmenes y muelle una
	// base cifrada con una clave que nadie vio.
	startsAndFails := func(h *harness) {
		h.f.OnDeployStack = func(d docker.StackDeploy) error {
			h.f.PutVolume(docker.Volume{Name: "muelle_data"})
			h.f.PutVolume(docker.Volume{Name: "muelle_caddy_admin"})
			h.f.PutStackService("muelle", docker.Service{Name: "muelle_muelle"})
			return errors.New("se cortó")
		}
	}
	t.Run("con volúmenes", func(t *testing.T) {
		h := newInstall(t)
		startsAndFails(h)
		check(t, h, h.run())
		calls := h.calls()
		inOrder(t, calls, "RemoveStack muelle", "RemoveVolume muelle_data")
		inOrder(t, calls, "RemoveStack muelle", "RemoveVolume muelle_caddy_admin")
		if vols, _ := h.f.ListVolumes(ctx, nil); len(vols) != 0 {
			t.Fatalf("quedaron volúmenes: %v", vols)
		}
	})

	t.Run("reinstall no borra la base que ya existía", func(t *testing.T) {
		h := newInstall(t)
		h.f.CreateSecret(ctx, "muelle_master_key", []byte("vieja"), nil)
		h.f.PutVolume(docker.Volume{Name: "muelle_data"})
		h.o.Reinstall = true
		startsAndFails(h)
		if err := h.run(); err == nil {
			t.Fatal("sin error")
		}
		noCall(t, h.calls(), "RemoveVolume muelle_data", "RemoveSecret muelle_master_key")
		inOrder(t, h.calls(), "RemoveStack muelle", "RemoveVolume muelle_caddy_admin")
		vols, _ := h.f.ListVolumes(ctx, nil)
		if len(vols) != 1 || vols[0].Name != "muelle_data" {
			t.Fatalf("volúmenes=%v", vols)
		}
		if _, err := h.f.InspectSecret(ctx, "muelle_master_key"); err != nil {
			t.Fatalf("borró la clave que ya existía: %v", err)
		}
	})

	t.Run("un volumen que no se pudo borrar", func(t *testing.T) {
		h := newInstall(t)
		startsAndFails(h)
		h.f.FailNext("RemoveVolume", errors.New("x"))
		err := h.run()
		if err == nil || !strings.Contains(err.Error(), "se cortó") {
			t.Fatalf("err=%v (esperaba el del deploy)", err)
		}
		if !strings.Contains(h.stderr.String(), "docker volume rm muelle_data") {
			t.Fatalf("stderr=%s", h.stderr)
		}
	})
}

// failSecret hace fallar CreateSecret solo con name; antes corre before (si no es nil).
type failSecret struct {
	*dockertest.Fake
	name   string
	err    error
	before func()
}

func (w *failSecret) CreateSecret(ctx context.Context, name string, data []byte, labels map[string]string) (string, error) {
	if name != w.name {
		return w.Fake.CreateSecret(ctx, name, data, labels)
	}
	if w.before != nil {
		w.before()
	}
	return "", w.err
}

func TestRunUndoOnTokenSecretFailure(t *testing.T) {
	for _, ctrlC := range []bool{false, true} {
		h := newInstall(t)
		h.o.CloudflareToken = strings.Repeat("c", 40)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		w := &failSecret{Fake: h.f, name: "muelle_init_cloudflare_token", err: errors.New("falló el secreto")}
		if ctrlC {
			w.before, w.err = cancel, context.Canceled
		}
		h.c.Docker = w
		if err := Run(ctx, h.o, h.c); err == nil {
			t.Fatalf("ctrlC=%v: sin error", ctrlC)
		}
		inOrder(t, h.calls(), "CreateNetwork muelle_panel", "CreateSecret muelle_master_key")
		_, nerr := h.f.InspectNetwork(context.Background(), "muelle_panel")
		gone(t, nerr, "la red")
		_, serr := h.f.InspectSecret(context.Background(), "muelle_master_key")
		gone(t, serr, "muelle_master_key")
		if h.stdout.Len() != 0 {
			t.Fatalf("ctrlC=%v: stdout=%s", ctrlC, h.stdout)
		}
		noCall(t, h.calls(), "DeployStack")
	}
}

func TestRunUndoOnLeftoverTokenSecret(t *testing.T) {
	h := newInstall(t)
	h.o.CloudflareToken = strings.Repeat("c", 40)
	h.f.CreateSecret(context.Background(), "muelle_init_cloudflare_token", []byte("viejo"), nil)
	h.f.FailNext("RemoveSecret", errors.New("no se pudo"))
	if err := h.run(); err == nil {
		t.Fatal("sin error")
	}
	_, nerr := h.f.InspectNetwork(context.Background(), "muelle_panel")
	gone(t, nerr, "la red")
	_, serr := h.f.InspectSecret(context.Background(), "muelle_master_key")
	gone(t, serr, "muelle_master_key")
	if h.stdout.Len() != 0 {
		t.Fatalf("stdout=%s", h.stdout)
	}
}

// noSubnets es una red recién creada que todavía no tiene subredes.
type noSubnets struct{ *dockertest.Fake }

func (n noSubnets) InspectNetwork(ctx context.Context, name string) (docker.Network, error) {
	net, err := n.Fake.InspectNetwork(ctx, name)
	net.Subnets = nil
	return net, err
}

func TestRunUndoNetworkWithoutSubnets(t *testing.T) {
	h := newInstall(t)
	h.c.Docker = noSubnets{h.f}
	h.c.NetworkWait = 100 * time.Millisecond
	err := h.run()
	if err == nil || !strings.Contains(err.Error(), "subred") {
		t.Fatalf("err=%v", err)
	}
	inOrder(t, h.calls(), "CreateNetwork muelle_panel", "RemoveNetwork muelle_panel")
	_, nerr := h.f.InspectNetwork(context.Background(), "muelle_panel")
	gone(t, nerr, "la red")
	noCall(t, h.calls(), "CreateSecret")
}

func TestRunSetupTimeoutKeepsStack(t *testing.T) {
	h := newInstall(t)
	h.f.OnDeployStack = func(d docker.StackDeploy) error {
		h.f.PutStackService("muelle", docker.Service{Name: "muelle_muelle"}, running("t1", 0))
		return nil
	}
	h.c.SetupTimeout = 200 * time.Millisecond
	err := h.run()
	if !errors.Is(err, ErrSetupTimeout) {
		t.Fatalf("err=%v", err)
	}
	key, _ := h.f.SecretData("muelle_master_key")
	if !strings.Contains(h.stdout.String(), string(key)) {
		t.Fatalf("la clave no salió: %s", h.stdout)
	}
	noCall(t, h.calls(), "RemoveStack", "RemoveSecret")
}

func TestRunHasUsers(t *testing.T) {
	h := newInstall(t)
	muelleStarts(h.f, muelleNames, listeningLine)
	if err := h.run(); err != nil {
		t.Fatal(err)
	}
	if out := h.stdout.String(); !strings.Contains(out, "ya tiene usuarios") || !strings.Contains(out, "https://panel.example.com") {
		t.Fatalf("stdout=%s", out)
	}
}

// Con usuarios (por ejemplo, con --reinstall), el origen del panel es el
// que dice muelle en listening, que sale de sus ajustes: --https-port solo
// vale en el primer arranque de la base.
func TestRunHasUsersPanelFromMuelle(t *testing.T) {
	h := newInstall(t)
	h.o.HTTPSPort = 8443
	muelleStarts(h.f, muelleNames, listeningPanelLine("https://panel.example.com"))
	if err := h.run(); err != nil {
		t.Fatal(err)
	}
	if out := h.stdout.String(); !strings.Contains(out, "entra al panel en https://panel.example.com\n") {
		t.Fatalf("stdout=%s", out)
	}
}

func TestRunNamePrefix(t *testing.T) {
	h := newInstall(t)
	h.o.NamePrefix = "it5abc"
	h.o.PanelDomain, h.o.PanelOrigin = "", "https://127.0.0.1:22443"
	h.o.CloudflareToken = strings.Repeat("c", 40)
	names := naming.InstallNamesFor("it5abc")
	muelleStarts(h.f, names, setupLine("https://127.0.0.1:22443/setup?token=X"), listeningLine)
	if err := h.run(); err != nil {
		t.Fatalf("%v\nstderr: %s", err, h.stderr)
	}
	calls := h.calls()
	inOrder(t, calls, "InspectService it5abc_muelle", "InspectSecret it5abc_master_key", "InspectNetwork it5abc_panel",
		"CreateContainer it5abc-install-probe "+muelleRef, "CreateNetwork it5abc_panel", "CreateSecret it5abc_master_key",
		"CreateSecret it5abc_init_cloudflare_token", "DeployStack it5abc")
	fixed := naming.InstallNamesFor(naming.MuelleStack)
	for _, c := range calls {
		for _, arg := range strings.Fields(c)[1:] {
			if slices.Contains([]string{fixed.Stack, fixed.Service, fixed.PanelNetwork, fixed.DataVolume, fixed.CaddyAdminVolume,
				fixed.MasterKeySecret, fixed.InitCloudflareTokenSecret, fixed.InitRegistrySecret, fixed.ProbeContainer, fixed.CaddyContainer}, arg) {
				t.Errorf("usó un nombre fijo: %q", c)
			}
		}
	}
	if !strings.Contains(h.stdout.String(), "https://127.0.0.1:22443/setup?token=X") {
		t.Fatalf("stdout=%s", h.stdout)
	}
}

// --- Las imágenes privadas ----------------------------------------------------------

// recordPulls anota la credencial de cada PullImage (el fake solo anota la ref).
type recordPulls struct {
	*dockertest.Fake
	mu    sync.Mutex
	auths map[string]*docker.RegistryAuth
}

func (r *recordPulls) PullImage(ctx context.Context, ref string, auth *docker.RegistryAuth) error {
	r.mu.Lock()
	if r.auths == nil {
		r.auths = map[string]*docker.RegistryAuth{}
	}
	if auth != nil {
		a := *auth
		auth = &a
	}
	r.auths[ref] = auth
	r.mu.Unlock()
	return r.Fake.PullImage(ctx, ref, auth)
}

// checkRegistryInstall verifica una instalación con la credencial de ghcr.io:
// las dos imágenes bajadas con ella, el secreto muelle_init_registry, el
// stack que lo monta y lo pasa a docker stack deploy, y la contraseña en
// ninguna salida.
func checkRegistryInstall(t *testing.T, h *harness, r *recordPulls, user string) {
	t.Helper()
	want := docker.RegistryAuth{Host: "ghcr.io", Username: user, Password: regPass}
	for _, ref := range []string{muelleRef, caddyRef} {
		if a := r.auths[ref]; a == nil || *a != want {
			t.Errorf("PullImage %s con %v", ref, a)
		}
	}
	data, ok := h.f.SecretData("muelle_init_registry")
	var got map[string]string
	if !ok || json.Unmarshal(data, &got) != nil || len(got) != 3 ||
		got["host"] != "ghcr.io" || got["username"] != user || got["password"] != regPass {
		t.Fatalf("secreto=%s", data)
	}
	c := compose(t, h.f)
	if !strings.Contains(c, `"MUELLE_INIT_REGISTRY_FILE": "/run/secrets/muelle_init_registry"`) || !strings.Contains(c, `"name": "muelle_init_registry"`) {
		t.Fatalf("compose:\n%s", c)
	}
	if d := h.f.StackDeploys(); len(d[0].Registries) != 1 || d[0].Registries[0] != want {
		t.Fatalf("Registries=%d", len(d[0].Registries))
	}
	if out := h.stdout.String() + h.stderr.String() + c; strings.Contains(out, regPass) {
		t.Fatal("la contraseña salió en la salida o en el compose")
	}
}

func TestRunRegistryFromFlags(t *testing.T) {
	f := dockertest.New()
	r := &recordPulls{Fake: f}
	h := newInstallWith(t, f)
	h.c.Docker = r
	h.o.RegistryUser, h.o.RegistryPassword = "bot", regPass
	if err := h.run(); err != nil {
		t.Fatalf("%v\nstderr: %s", err, h.stderr)
	}
	checkRegistryInstall(t, h, r, "bot")
}

func TestRunRegistryFromDockerConfig(t *testing.T) {
	f := dockertest.New()
	r := &recordPulls{Fake: f}
	h := newInstallWith(t, f)
	h.c.Docker = r
	h.c.DockerConfig = writeDockerConfig(t, `{"auths":{"ghcr.io":{"auth":"`+authOf("cfgbot", regPass)+`"}}}`)
	if err := h.run(); err != nil {
		t.Fatalf("%v\nstderr: %s", err, h.stderr)
	}
	checkRegistryInstall(t, h, r, "cfgbot")
}

// Un helper de credenciales no corre en el contenedor: el error pide los
// flags antes de tocar Docker.
func TestRunRegistryHelperFails(t *testing.T) {
	h := newInstall(t)
	h.c.DockerConfig = writeDockerConfig(t, `{"auths":{"ghcr.io":{}},"credsStore":"secretservice"}`)
	err := h.run()
	if err == nil || !strings.Contains(err.Error(), "--registry-user") {
		t.Fatalf("err=%v", err)
	}
	if calls := h.calls(); len(calls) != 0 {
		t.Fatalf("tocó Docker: %q", calls)
	}
}

func TestRunPullDenied(t *testing.T) {
	for _, withCreds := range []bool{false, true} {
		f := dockertest.New()
		f.DenyImage(muelleRef)
		h := newInstallWith(t, f)
		if withCreds {
			h.o.RegistryUser, h.o.RegistryPassword = "bot", regPass
		}
		err := h.run()
		if err == nil {
			t.Fatalf("withCreds=%v: sin error", withCreds)
		}
		for _, want := range []string{"docker login ghcr.io", "read:packages", "--registry-user", "--registry-password-stdin", "~/.docker/config.json"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("withCreds=%v: el error no dice %q: %v", withCreds, want, err)
			}
		}
		if strings.Contains(err.Error()+h.stderr.String()+h.stdout.String(), regPass) {
			t.Fatal("la contraseña salió")
		}
		noCall(t, f.Calls(), "CreateContainer", "CreateNetwork", "CreateSecret", "DeployStack")
	}
}

func TestRunRegistrySecretUndo(t *testing.T) {
	h := newInstall(t)
	h.o.RegistryUser, h.o.RegistryPassword = "bot", regPass
	h.f.FailNext("DeployStack", errors.New("falló"))
	if err := h.run(); err == nil {
		t.Fatal("sin error")
	}
	inOrder(t, h.calls(), "CreateSecret muelle_init_registry", "DeployStack muelle", "RemoveSecret muelle_init_registry")
	_, err := h.f.InspectSecret(context.Background(), "muelle_init_registry")
	gone(t, err, "muelle_init_registry")
}

// El secreto del registry se trata como el del token: el que quedó se
// reemplaza si viene una credencial, y si no, no se toca ni se monta.
func TestRunReinstallRegistrySecret(t *testing.T) {
	for _, withCreds := range []bool{false, true} {
		h := newInstall(t)
		ctx := context.Background()
		h.f.CreateSecret(ctx, "muelle_master_key", []byte("vieja"), nil)
		h.f.CreateSecret(ctx, "muelle_init_registry", []byte(`{"host":"ghcr.io","username":"viejo","password":"x"}`), nil)
		h.o.Reinstall = true
		if withCreds {
			h.o.RegistryUser, h.o.RegistryPassword = "bot", regPass
		}
		if err := h.run(); err != nil {
			t.Fatal(err)
		}
		c := compose(t, h.f)
		if withCreds {
			inOrder(t, h.calls(), "RemoveSecret muelle_init_registry", "CreateSecret muelle_init_registry")
			if !strings.Contains(c, "MUELLE_INIT_REGISTRY_FILE") {
				t.Fatalf("compose:\n%s", c)
			}
		} else {
			noCall(t, h.calls(), "RemoveSecret", "CreateSecret")
			if strings.Contains(c, "init_registry") {
				t.Fatalf("montó el secreto que quedó:\n%s", c)
			}
		}
	}
}

// Unas Options armadas a mano con el usuario y sin la contraseña (o al
// revés) no sirven: ParseArgs nunca las deja así.
func TestRunRegistryNeedsBoth(t *testing.T) {
	for _, o := range []Options{{RegistryUser: "bot"}, {RegistryPassword: regPass}} {
		h := newInstall(t)
		h.o.RegistryUser, h.o.RegistryPassword = o.RegistryUser, o.RegistryPassword
		err := h.run()
		if err == nil || !strings.Contains(err.Error(), "--registry-user") || strings.Contains(err.Error(), regPass) {
			t.Fatalf("err=%v", err)
		}
		if calls := h.calls(); len(calls) != 0 {
			t.Fatalf("tocó Docker: %q", calls)
		}
	}
}
