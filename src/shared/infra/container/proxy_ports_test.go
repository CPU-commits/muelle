package container

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/internal/ingress"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/caddy/caddytest"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare/cftest"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
)

// Cambiar los puertos del proxy con un puerto ocupado (contrato §3.5 y §4.1),
// con muelle completo sobre Docker y Caddy falsos:
//   - si el PATCH ya lo ve ocupado, 422 en ese campo y no se guarda nada;
//   - si lo toman después de responder el PATCH, la recreación no arranca,
//     muelle vuelve a los puertos anteriores (los ajustes y muelle-caddy), y
//     el Origin del puerto anterior pasa enseguida (sin esperar el TTL del
//     origen en caché). GET /proxy y la auditoría lo dicen.
func TestContractProxyPortsRollback(t *testing.T) {
	const before, after = "https://panel.test:8443", "https://panel.test:9443"
	s := testSettings(t)
	s.PanelOrigin, s.Domain = "", "panel.test"
	s.Init.ProxyHTTPPort, s.Init.ProxyHTTPSPort = 8080, 8443
	s.Init.CaddyImage = "caddy:test" // el proxy habilitado sin una versión
	fake := dockertest.New()
	fake.SetImagePresent("caddy:test")
	c, err := New(context.Background(), s, testLogger(t), WithDocker(fake), WithIngress(ingress.Options{
		CaddyAdminDial: caddytest.New(t).Dial, CloudflareURL: cftest.New(t).URL(),
		Timing: ingress.Timing{RecreateDelay: 500 * time.Millisecond, RetryMin: time.Millisecond, RetryMax: 5 * time.Millisecond},
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		<-done
	})
	e := &originEnv{api: api{t: t, c: c}}
	token, _, err := c.Identity.EnsureSetupInvite(ctx)
	if err != nil {
		t.Fatal(err)
	}
	w := e.do("POST", "/api/v1/auth/setup", map[string]string{"token": token, "username": "admin", "password": testPassword}, httptestutil.WithOrigin(before))
	httptestutil.Expect(t, w, http.StatusCreated)
	e.session = httptestutil.WithSession(httptestutil.SessionFrom(t, w))
	for _, j := range c.Jobs {
		if j.Name == "proxy" {
			if err := j.Run(ctx); err != nil {
				t.Fatalf("job proxy: %v", err)
			}
		}
	}
	go func() { defer close(done); c.Run(ctx) }()
	settings := func() map[string]any {
		t.Helper()
		w := e.do("GET", "/api/v1/settings", nil, e.session)
		httptestutil.Expect(t, w, http.StatusOK)
		return httptestutil.Decode[map[string]any](t, w)
	}
	proxy := func() map[string]any {
		t.Helper()
		w := e.do("GET", "/api/v1/proxy", nil, e.session)
		httptestutil.Expect(t, w, http.StatusOK)
		return httptestutil.Decode[map[string]any](t, w)
	}
	if got := proxy(); got["status"] != "synced" || got["ports_rollback"] != nil {
		t.Fatalf("GET /proxy al empezar: %v", got)
	}

	// 1. Ocupado al guardar: 422 en el campo, y nada guardado.
	fake.BusyHostPort(9443)
	w = e.do("PATCH", "/api/v1/settings", map[string]any{"proxy_https_port": 9443, "acme_email": "tu@correo.cl"}, e.session, httptestutil.WithOrigin(before))
	expect(t, "PATCH con el puerto ocupado", w, http.StatusUnprocessableEntity, "invalid")
	if msg := errorMessage(t, w); !strings.HasPrefix(msg, "proxy_https_port: el puerto 9443 del host ya está en uso") {
		t.Fatalf("mensaje: %q", msg)
	}
	if got := settings(); got["proxy_https_port"] != 8443.0 || got["acme_email"] != "" {
		t.Fatalf("no se guardó nada: %v", got)
	}

	// 2. Libre al guardar y ocupado al recrear: vuelve a 8443.
	fake.FreeHostPort(9443)
	e.patchPorts(before, 8080, 9443)
	fake.BusyHostPort(9443)
	e.expectWrites([]string{after}, []string{before}) // el origen nuevo queda en caché
	deadline := time.Now().Add(10 * time.Second)
	for {
		got := proxy()
		if got["status"] == "synced" && got["ports_rollback"] != nil {
			rb, _ := got["ports_rollback"].(map[string]any)
			attempted, _ := rb["attempted"].(map[string]any)
			restored, _ := rb["restored"].(map[string]any)
			caddy, _ := got["caddy"].(map[string]any)
			if attempted["https_port"] != 9443.0 || restored["https_port"] != 8443.0 || caddy["https_port"] != 8443.0 || caddy["running"] != true {
				t.Fatalf("GET /proxy: %v", got)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no volvió a los puertos anteriores: %v", got)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := settings(); got["proxy_https_port"] != 8443.0 {
		t.Fatalf("los ajustes vuelven: %v", got)
	}
	e.expectWrites([]string{before}, []string{after})
	var actor, details string
	if err := c.DB.Conn(ctx).QueryRowContext(ctx, `SELECT actor_type, details FROM audit_events WHERE action = 'settings.ports_rollback'`).Scan(&actor, &details); err != nil ||
		actor != "system" || !strings.Contains(details, `"proxy_https_port":8443`) {
		t.Fatalf("auditoría: %q %q %v", actor, details, err)
	}
}
