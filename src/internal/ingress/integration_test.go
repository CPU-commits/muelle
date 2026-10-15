//go:build integration

package ingress

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

// Caddy real (caddy:2-alpine, con la CA interna) acepta la configuración que
// arma muelle, la recibe por el socket de la admin API y enruta: por hostname
// a la app por la red del stage, y en el panel por client_ip (spec §15).
func TestIntegrationCaddyAcceptsConfig(t *testing.T) {
	e := newITEnv(t, 1)
	m := e.module()
	ctx := context.Background()

	// 1. El puente, la red del stage (por el módulo: si conecta algo, es
	// it3-<id>-caddy, nunca muelle-caddy), el backend y Caddy.
	e.startAdminBridge()
	ingresstest.Stage(t, e.db, e.project, "s1")
	if err := m.EnsureStageNetwork(ctx, e.project, "s1"); err != nil {
		t.Fatal(err)
	}
	e.startBackend("s1", "web")
	e.startCaddyManually("s1")

	// 2. El dominio web.it3<id>.test → web:8080.
	appID := ingresstest.App(t, e.db, e.project, "s1", "web", "service")
	web := e.host("web")
	e.addDomain(appID, web, "web", 8080)
	e.settings(shared.DefaultProxySettings())

	// 3. Caddy está arrancando: el primer POST /load puede no llegar.
	e.eventually(30*time.Second, "SyncRoutes", func() (bool, error) {
		err := m.SyncRoutes(ctx)
		return err == nil, err
	})
	if status, msg, _ := m.state.Snapshot(true); status != "synced" {
		t.Fatalf("estado después del Load: %s %q", status, msg)
	}

	// 4. Por hostname, a la app por la red del stage.
	e.eventually(30*time.Second, "GET "+web, e.expectGet(web, "/", http.StatusOK, "hola web"))

	// 5. Con IPs del panel, la del cliente (la del bridge de Docker) no está:
	// 403. Prueba el matcher client_ip en Caddy real.
	p := shared.DefaultProxySettings()
	p.PanelAllowedCIDRs = []string{"203.0.113.0/24"}
	e.settings(p)
	if err := m.SyncRoutes(ctx); err != nil {
		t.Fatal(err)
	}
	e.eventually(15*time.Second, "panel con IPs", e.expectGet(e.panelHost(), "/", http.StatusForbidden, ""))

	// 6. Sin la lista, la ruta existe y va a muelle_muelle:8080, que acá no
	// existe: 502. La app sigue respondiendo.
	e.settings(shared.DefaultProxySettings())
	if err := m.SyncRoutes(ctx); err != nil {
		t.Fatal(err)
	}
	e.eventually(15*time.Second, "panel sin IPs", e.expectGet(e.panelHost(), "/", http.StatusBadGateway, ""))
	e.eventually(15*time.Second, "GET "+web+" otra vez", e.expectGet(web, "/", http.StatusOK, "hola web"))
}

// El ciclo de vida de muelle-caddy contra Docker real (contrato §3.4, spec
// §15): el job lo crea con los nombres del test, se conecta a una red de
// stage nueva en caliente, los stages quedan aislados entre sí, se recrea
// con otros puertos, un puerto ocupado lo deja borrado y en GET /proxy, y
// las redes de stage huérfanas se borran a la segunda pasada.
func TestIntegrationCaddyLifecycle(t *testing.T) {
	e := newITEnv(t, 2)
	e.opts.Timing.RecreateDelay = 50 * time.Millisecond
	m := e.module()
	ctx := context.Background()
	job := func() error { return m.Jobs()[0].Run(ctx) }

	// 1. Primera pasada: el job crea it3-<id>-caddy (nunca muelle-caddy) en
	// la red panel y en la del stage, y le manda la configuración.
	e.startAdminBridge()
	ingresstest.Stage(t, e.db, e.project, "s1")
	webID := ingresstest.App(t, e.db, e.project, "s1", "web", "service")
	web := e.host("web")
	e.addDomain(webID, web, "web", 8080)
	e.settings(shared.DefaultProxySettings())
	if err := m.EnsureStageNetwork(ctx, e.project, "s1"); err != nil {
		t.Fatal(err)
	}
	e.startBackend("s1", "web")
	if err := job(); err != nil {
		t.Fatalf("primera pasada: %v", err)
	}
	first, err := e.caddy()
	if err != nil {
		t.Fatal(err)
	}
	s1 := naming.Network(e.project, "s1")
	if !first.Running || !slices.Contains(first.Networks, s1) || !slices.Contains(first.Networks, e.opts.PanelNetwork) {
		t.Fatalf("muelle-caddy: %+v", first)
	}
	e.eventually(30*time.Second, "synced", e.proxyStatus(m, "synced"))
	e.eventually(30*time.Second, "GET "+web, e.expectGet(web, "/", http.StatusOK, "hola web"))

	// Otra pasada no lo toca: Docker devuelve lo que muelle pidió.
	if err := job(); err != nil {
		t.Fatalf("segunda pasada: %v", err)
	}
	if c, err := e.caddy(); err != nil || c.ID != first.ID {
		t.Fatalf("la segunda pasada lo recreó: %+v %v", c, err)
	}

	// 2. Una red de stage nueva se conecta en caliente: el mismo contenedor.
	ingresstest.Stage(t, e.db, e.project, "s2")
	if err := m.EnsureStageNetwork(ctx, e.project, "s2"); err != nil {
		t.Fatal(err)
	}
	s2 := naming.Network(e.project, "s2")
	if c, err := e.caddy(); err != nil || c.ID != first.ID || !c.StartedAt.Equal(first.StartedAt) || !slices.Contains(c.Networks, s2) {
		t.Fatalf("en caliente: %+v %v", c, err)
	}

	// 3. Aislamiento entre stages: la sonda de s1 alcanza el web de s1 y no
	// el web2 de s2; solo así sirve "aislado".
	e.startBackend("s2", "web2")
	web2ID := ingresstest.App(t, e.db, e.project, "s2", "web2", "service")
	web2 := e.host("web2")
	e.addDomain(web2ID, web2, "web2", 8080)
	if err := job(); err != nil {
		t.Fatal(err)
	}
	e.eventually(30*time.Second, "GET "+web2, e.expectGet(web2, "/", http.StatusOK, "hola web2"))
	probeCmd := fmt.Sprintf("ok=0; for i in $(seq 1 20); do wget -T 3 -q -O /dev/null http://%s:8080 && { ok=1; break; }; sleep 1; done; "+
		"[ $ok = 1 ] || exit 1; wget -T 3 -q -O /dev/null http://%s:8080 && exit 1; exec caddy respond --listen :8080 --body aislado",
		naming.Service(e.project, "s1", "web", "web"), naming.Service(e.project, "s2", "web2", "web2"))
	e.startBackendCmd("s1", "probe", []string{"sh", "-c", probeCmd})
	probeID := ingresstest.App(t, e.db, e.project, "s1", "probe", "service")
	probe := e.host("probe")
	e.addDomain(probeID, probe, "probe", 8080)
	if err := job(); err != nil {
		t.Fatal(err)
	}
	e.eventually(60*time.Second, "GET "+probe, e.expectGet(probe, "/", http.StatusOK, "aislado"))

	// 4. Recreación con los puertos de repuesto, agendada (la corre Run).
	e.run(m)
	p := shared.DefaultProxySettings()
	p.HTTPPort, p.HTTPSPort = e.ports.HTTP2, e.ports.HTTPS2
	e.settings(p)
	m.scheduleRecreate()
	e.eventually(60*time.Second, "recreado con los puertos nuevos", func() (bool, error) {
		c, err := e.caddy()
		if err != nil {
			return false, err
		}
		published := slices.ContainsFunc(c.PublishedPorts, func(b docker.PortBinding) bool { return b.HostPort == e.ports.HTTPS2 })
		return c.ID != first.ID && c.Running && published, fmt.Errorf("%s %v %+v", c.ID, c.Running, c.PublishedPorts)
	})
	e.eventually(30*time.Second, "synced después de recrear", e.proxyStatus(m, "synced"))
	e.ports.HTTPS = e.ports.HTTPS2
	e.eventually(30*time.Second, "GET "+web+" por el puerto nuevo", e.expectGet(web, "/", http.StatusOK, "hola web"))

	// 5. Un puerto ocupado por un proceso del host: el contenedor queda
	// borrado y GET /proxy lo dice; al liberarlo, el job lo crea de nuevo.
	l, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", e.ports.Busy))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	p.HTTPPort = e.ports.Busy
	e.settings(p)
	if err := job(); !errors.Is(err, service.ErrPortInUse) {
		t.Fatalf("con el puerto ocupado: %v", err)
	}
	client := httptestutil.New(t, httptestutil.FakeAuth{Sessions: map[string]shared.Principal{"s": {UserID: 1, Username: "felipe"}}},
		m.RegisterRoutes)
	w := client.Do("GET", "/api/v1/proxy", nil, httptestutil.WithSession("s"))
	httptestutil.Expect(t, w, http.StatusOK)
	got := httptestutil.Decode[map[string]any](t, w)
	if msg, _ := got["error"].(string); got["status"] != "out_of_sync" || !strings.Contains(msg, strconv.Itoa(e.ports.Busy)) {
		t.Fatalf("GET /proxy: %v", got)
	}
	if _, err := e.caddy(); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("el contenedor debería estar borrado: %v", err)
	}
	l.Close()
	if err := job(); err != nil {
		t.Fatalf("con el puerto libre: %v", err)
	}
	e.eventually(30*time.Second, "synced con el puerto libre", e.proxyStatus(m, "synced"))

	// 6. Una red con etiquetas de un stage que no existe se borra a la
	// segunda pasada.
	orphan := naming.Network(e.project, "viejo")
	labels := naming.StageLabels(e.project, "viejo")
	labels[itTestLabel] = "it3"
	if _, err := e.docker.CreateNetwork(ctx, docker.NetworkSpec{Name: orphan, Driver: "overlay", Attachable: true, Labels: labels}); err != nil {
		t.Fatal(err)
	}
	if err := job(); err != nil {
		t.Fatal(err)
	}
	if _, err := e.docker.InspectNetwork(ctx, orphan); err != nil {
		t.Fatalf("la primera pasada no la borra: %v", err)
	}
	if err := job(); err != nil {
		t.Fatal(err)
	}
	if _, err := e.docker.InspectNetwork(ctx, orphan); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("la segunda pasada la borra: %v", err)
	}
}
