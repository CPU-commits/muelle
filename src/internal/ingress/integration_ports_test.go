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
	outservice "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

// La ranura de it6 de este test: 23601-23606 (lejos del principio del rango,
// que pueden usar otras ramas).
const itPortsRollbackSlot = 6

// Cambiar los puertos del proxy con un puerto del host ocupado, contra Docker
// y Caddy reales (contrato §3.5 y §4.1), con el prefijo it6:
//  1. CheckProxyPorts (lo que llama PATCH /settings antes de guardar) da un
//     422 en proxy_https_port, y el muelle-caddy de antes sigue sirviendo;
//  2. si el puerto se ocupa después del PATCH, la recreación no arranca:
//     muelle vuelve a los puertos anteriores, en los ajustes y en el
//     contenedor, y la app responde otra vez por el puerto de antes. GET
//     /proxy (ports_rollback) y la auditoría lo dicen.
func TestIntegrationProxyPortsRollback(t *testing.T) {
	e := newITEnvIn(t, itPorts6, itPortsRollbackSlot)
	e.opts.Timing.RecreateDelay = 50 * time.Millisecond
	m := e.module()
	ctx := context.Background()

	// muelle-caddy sirviendo web.it6<id>.test por los puertos de la ranura.
	e.startAdminBridge()
	ingresstest.Stage(t, e.db, e.project, "s1")
	webID := ingresstest.App(t, e.db, e.project, "s1", "web", "service")
	web := e.host("web")
	e.addDomain(webID, web, "web", 8080)
	e.settings(shared.DefaultProxySettings())
	if err := e.retryAttach(func() error { return m.EnsureStageNetwork(ctx, e.project, "s1") }); err != nil {
		t.Fatal(err)
	}
	e.startBackend("s1", "web")
	if err := m.Jobs()[0].Run(ctx); err != nil {
		t.Fatalf("job: %v", err)
	}
	first, err := e.caddy()
	if err != nil || !first.Running {
		t.Fatalf("muelle-caddy: %+v %v", first, err)
	}
	e.eventually(30*time.Second, "synced", e.proxyStatus(m, "synced"))
	e.eventually(30*time.Second, "GET "+web, e.expectGet(web, "/", http.StatusOK, "hola web"))

	// 1. Un proceso del host tiene el puerto: el PATCH no lo guardaría.
	l, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", e.ports.Busy))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	old, err := outservice.NewProxySettingsReader(e.db, e.cipher).ProxySettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	next := old
	next.HTTPSPort = e.ports.Busy
	err = m.CheckProxyPorts(ctx, old, next)
	if ve, ok := errors.AsType[*validate.Error](err); !ok || ve.Field != shared.SettingProxyHTTPSPort ||
		!strings.Contains(ve.Message, strconv.Itoa(e.ports.Busy)) {
		t.Fatalf("CheckProxyPorts con el puerto ocupado: %v", err)
	}
	if c, err := e.caddy(); err != nil || c.ID != first.ID || !c.Running {
		t.Fatalf("muelle-caddy cambió: %+v %v", c, err)
	}
	e.eventually(15*time.Second, "GET "+web+" después de la prueba", e.expectGet(web, "/", http.StatusOK, "hola web"))

	// 2. El puerto se ocupa después de que el PATCH lo probó y lo guardó: la
	// recreación (Run) vuelve a los puertos anteriores.
	e.run(m)
	e.settings(next)
	m.ProxySettingsChanged(ctx, old, next)
	e.eventually(90*time.Second, "vuelve a los puertos anteriores", func() (bool, error) {
		status, msg, _ := m.state.Snapshot(true)
		if rb := m.state.LastPortsRollback(); rb == nil || status != "synced" {
			return false, fmt.Errorf("estado %s %q, ports_rollback %+v", status, msg, rb)
		}
		return true, nil
	})
	got, err := outservice.NewProxySettingsReader(e.db, e.cipher).ProxySettings(ctx)
	if err != nil || got.HTTPPort != e.ports.HTTP || got.HTTPSPort != e.ports.HTTPS {
		t.Fatalf("los ajustes vuelven a %d/%d: %+v %v", e.ports.HTTP, e.ports.HTTPS, got, err)
	}
	c, err := e.caddy()
	if err != nil {
		t.Fatal(err)
	}
	published := slices.ContainsFunc(c.PublishedPorts, func(b docker.PortBinding) bool { return b.HostPort == e.ports.HTTPS })
	if c.ID == first.ID || !c.Running || !published {
		t.Fatalf("muelle-caddy de nuevo con los puertos anteriores: %+v", c)
	}
	e.eventually(30*time.Second, "GET "+web+" por el puerto de antes", e.expectGet(web, "/", http.StatusOK, "hola web"))

	client := httptestutil.New(t, httptestutil.FakeAuth{Sessions: map[string]shared.Principal{"s": {UserID: 1, Username: "felipe"}}},
		m.RegisterRoutes)
	w := client.Do("GET", "/api/v1/proxy", nil, httptestutil.WithSession("s"))
	httptestutil.Expect(t, w, http.StatusOK)
	body := httptestutil.Decode[map[string]any](t, w)
	rb, _ := body["ports_rollback"].(map[string]any)
	attempted, _ := rb["attempted"].(map[string]any)
	restored, _ := rb["restored"].(map[string]any)
	if attempted["https_port"] != float64(e.ports.Busy) || restored["https_port"] != float64(e.ports.HTTPS) ||
		!strings.Contains(fmt.Sprint(rb["error"]), strconv.Itoa(e.ports.Busy)) {
		t.Fatalf("GET /proxy: %v", body)
	}
	var actor, details string
	if err := e.db.Conn(ctx).QueryRowContext(ctx, `SELECT actor_type, details FROM audit_events WHERE action = 'settings.ports_rollback'`).
		Scan(&actor, &details); err != nil || actor != "system" || !strings.Contains(details, `"proxy_https_port":`+strconv.Itoa(e.ports.HTTPS)) {
		t.Fatalf("auditoría: %q %q %v", actor, details, err)
	}
}
