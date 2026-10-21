package container

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/settings"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
)

// originEnv arma muelle con s y crea el primer usuario desde el Origin
// first, como el link de setup que imprime install.
type originEnv struct {
	api
	session httptestutil.Opt
	n       int
}

func newOriginEnv(t *testing.T, s settings.Settings, first string) *originEnv {
	t.Helper()
	c, err := New(context.Background(), s, testLogger(t), WithDocker(dockertest.New()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	e := &originEnv{api: api{t: t, c: c}}
	token, _, err := c.Identity.EnsureSetupInvite(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	w := e.do("POST", "/api/v1/auth/setup", map[string]string{"token": token, "username": "admin", "password": testPassword}, httptestutil.WithOrigin(first))
	httptestutil.Expect(t, w, http.StatusCreated)
	e.session = httptestutil.WithSession(httptestutil.SessionFrom(t, w))
	return e
}

// write hace una escritura con cookie desde origin y devuelve el status
// (403 solo si es bad_origin).
func (e *originEnv) write(origin string) int {
	e.t.Helper()
	e.n++
	w := e.do("POST", "/api/v1/projects", map[string]string{"slug": "p" + strings.Repeat("x", e.n), "name": "P"}, e.session, httptestutil.WithOrigin(origin))
	if w.Code == http.StatusForbidden && httptestutil.ErrorCode(e.t, w) != "bad_origin" {
		e.t.Fatalf("Origin %s: %s", origin, w.Body.String())
	}
	return w.Code
}

func (e *originEnv) expectWrites(ok, rejected []string) {
	e.t.Helper()
	for _, o := range ok {
		if code := e.write(o); code != http.StatusCreated {
			e.t.Fatalf("Origin %s: %d, esperaba 201", o, code)
		}
	}
	for _, o := range rejected {
		if code := e.write(o); code != http.StatusForbidden {
			e.t.Fatalf("Origin %s: %d, esperaba 403 bad_origin", o, code)
		}
	}
}

// inviteURL crea una invitación desde origin y devuelve su link.
func (e *originEnv) inviteURL(origin string) string {
	e.t.Helper()
	w := e.do("POST", "/api/v1/invites", nil, e.session, httptestutil.WithOrigin(origin))
	httptestutil.Expect(e.t, w, http.StatusCreated)
	url, _ := httptestutil.Decode[map[string]any](e.t, w)["url"].(string)
	return url
}

func (e *originEnv) patchPorts(origin string, httpPort, httpsPort int) {
	e.t.Helper()
	w := e.do("PATCH", "/api/v1/settings", map[string]any{"proxy_http_port": httpPort, "proxy_https_port": httpsPort}, e.session, httptestutil.WithOrigin(origin))
	httptestutil.Expect(e.t, w, http.StatusOK)
}

// Lo que instala install: MUELLE_PANEL_DOMAIN, sin MUELLE_PANEL_ORIGIN, con
// --https-port 8443 (spec §16 paso 2). El origen del panel, el Origin
// permitido y los links salen de los ajustes, y siguen al cambio de puertos
// (paso 5) enseguida, sin tocar la configuración de muelle: el origen nuevo
// pasa y el viejo ya no.
func TestContractPanelOriginFromPanelDomain(t *testing.T) {
	const before, after = "https://panel.test:8443", "https://panel.test"
	s := testSettings(t)
	s.PanelOrigin, s.Domain = "", "panel.test"
	s.Init.ProxyHTTPPort, s.Init.ProxyHTTPSPort = 8080, 8443
	e := newOriginEnv(t, s, before)
	ctx := context.Background()

	if o := e.c.PanelOrigin(ctx); o != before {
		t.Fatalf("origen del panel después de install: %q", o)
	}
	if url := e.inviteURL(before); !strings.HasPrefix(url, before+"/invite/") {
		t.Fatalf("link de invitación: %s", url)
	}
	e.expectWrites([]string{before}, []string{after, "https://panel.test:9443", "http://panel.test:8443"})

	e.patchPorts(before, 80, 443)
	if o := e.c.PanelOrigin(ctx); o != after {
		t.Fatalf("origen del panel después del cambio: %q", o)
	}
	e.expectWrites([]string{after}, []string{before})
	if url := e.inviteURL(after); !strings.HasPrefix(url, after+"/invite/") {
		t.Fatalf("link de invitación después del cambio: %s", url)
	}
	// El login, que no tiene sesión, pasa desde el origen nuevo y no desde el
	// viejo.
	login := map[string]string{"username": "admin", "password": testPassword}
	httptestutil.Expect(t, e.do("POST", "/api/v1/auth/login", login, httptestutil.WithOrigin(after)), http.StatusOK)
	expect(t, "login desde el origen viejo", e.do("POST", "/api/v1/auth/login", login, httptestutil.WithOrigin(before)), http.StatusForbidden, "bad_origin")
}

// Detrás de otro proxy: MUELLE_PANEL_ORIGIN, puesto a mano, es un origen
// permitido más, y el de los links. Sigue pasando después de cambiar los
// puertos; el origen de los ajustes cambia como siempre.
func TestContractPanelOriginEnvStaysAllowed(t *testing.T) {
	const env, before, after = "https://muelle.otro.test", "https://panel.test:8443", "https://panel.test:9443"
	s := testSettings(t)
	s.PanelOrigin, s.Domain = env, "panel.test"
	s.Init.ProxyHTTPPort, s.Init.ProxyHTTPSPort = 8080, 8443
	e := newOriginEnv(t, s, env)

	e.expectWrites([]string{env, before}, []string{after})
	e.patchPorts(env, 8080, 9443)
	e.expectWrites([]string{env, after}, []string{before})
	if o := e.c.PanelOrigin(context.Background()); o != env {
		t.Fatalf("origen del panel: %q", o)
	}
	if url := e.inviteURL(after); !strings.HasPrefix(url, env+"/invite/") {
		t.Fatalf("link de invitación: %s", url)
	}
}

// Sin dominio del panel (desarrollo, un MUELLE_PANEL_ORIGIN con una IP), los
// ajustes no dan ningún origen: pasa solo MUELLE_PANEL_ORIGIN, como antes,
// aunque cambien los puertos del proxy.
func TestContractPanelOriginWithoutPanelDomain(t *testing.T) {
	const env = "https://127.0.0.1:18443"
	s := testSettings(t)
	s.PanelOrigin = env
	e := newOriginEnv(t, s, env)

	e.expectWrites([]string{env}, []string{"https://127.0.0.1", "https://127.0.0.1:9443"})
	e.patchPorts(env, 8080, 9443)
	e.expectWrites([]string{env}, []string{"https://127.0.0.1:9443"})
	if o := e.c.PanelOrigin(context.Background()); o != env {
		t.Fatalf("origen del panel: %q", o)
	}
}
