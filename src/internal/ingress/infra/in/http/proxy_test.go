package http

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/ingress/app/usecases"
	ingressentity "github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	reposqlite "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/repository/sqlite"
	outservice "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/caddy"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

const testImage = "ghcr.io/cpu-commits/muelle-caddy:v0.1.0"

// proxyFixture arma GET /proxy con los casos de uso de verdad (sqlite, un
// caddy.Runtime sobre dockertest y un ProxyState), sin el módulo: el paquete
// ingress importa este, y su test no puede importarlo de vuelta.
type proxyFixture struct {
	db       *sqlite.DB
	cipher   *crypto.Cipher
	docker   *dockertest.Fake
	runtime  *caddy.Runtime
	state    *usecases.ProxyState
	client   *httptestutil.Client
	origin   usecases.OriginRepos
	settings *outservice.ProxySettingsReader
	clock    *clock.Manual
}

func newProxyFixture(t *testing.T) *proxyFixture {
	t.Helper()
	db := sqlitetest.New(t)
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	d := dockertest.New()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := caddy.NewRuntime(d, caddy.Names{Container: "t-caddy", PanelNetwork: "t_panel",
		DataVolume: "t-data", ConfigVolume: "t-config", AdminVolume: "t-admin"}, log)
	f := &proxyFixture{db: db, cipher: c, docker: d, runtime: rt, state: usecases.NewProxyState()}
	settings := outservice.NewProxySettingsReader(db, c)
	f.origin = usecases.OriginRepos{Certs: reposqlite.NewOriginCerts(db, c), Hosts: reposqlite.NewOriginHosts(db)}
	f.settings = settings
	f.clock = clock.NewManual(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC))
	origin := usecases.NewEnsureOriginCA(usecases.OriginDeps{Settings: settings, Repos: f.origin, Clock: f.clock, DefImage: testImage})
	uc := UseCases{ProxyStatus: usecases.NewGetProxyStatus(settings, rt, f.state, origin, f.clock, testImage)}
	f.client = httptestutil.New(t, httptestutil.FakeAuth{
		Sessions: map[string]entity.Principal{"s": {UserID: 1, Username: "felipe"}},
		Tokens:   map[string]entity.Principal{"tok": {UserID: 1, Username: "felipe"}},
	}, func(k *utils.Kit) { Register(k, uc) })
	return f
}

// enable deja el proxy habilitado (panel_domain; la imagen es la de la versión).
func (f *proxyFixture) enable(t *testing.T, mod func(*entity.ProxySettings)) {
	t.Helper()
	p := entity.DefaultProxySettings()
	p.PanelDomain = "panel.auditfy.cl"
	if mod != nil {
		mod(&p)
	}
	ingresstest.SetProxySettings(t, f.db, f.cipher, p)
}

func (f *proxyFixture) get(t *testing.T) map[string]any {
	t.Helper()
	w := f.client.Do("GET", "/api/v1/proxy", nil, httptestutil.WithSession("s"))
	httptestutil.Expect(t, w, http.StatusOK)
	return httptestutil.Decode[map[string]any](t, w)
}

// Al arrancar muelle, antes del primer POST /load (contrato §3.3). Están
// todas las claves, y los null lo son.
func TestProxyStatusInitial(t *testing.T) {
	f := newProxyFixture(t)
	f.enable(t, nil)
	want := map[string]any{
		"status": "out_of_sync", "error": "muelle todavía no le mandó la configuración a Caddy", "synced_at": nil, "ports_rollback": nil,
		"caddy":             map[string]any{"image": nil, "running": false, "http_port": 80.0, "https_port": 443.0},
		"panel_certificate": map[string]any{"hostname": "panel.auditfy.cl", "cert_source": "letsencrypt", "origin_ca": nil},
	}
	if got := f.get(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("GET /proxy:\n got %v\nwant %v", got, want)
	}
}

func TestProxyStatusSynced(t *testing.T) {
	f := newProxyFixture(t)
	f.enable(t, func(p *entity.ProxySettings) { p.HTTPPort, p.HTTPSPort = 8080, 8443 })
	f.docker.SetImagePresent(testImage)
	if _, err := f.runtime.Ensure(context.Background(), service.CaddyWant{Image: testImage, HTTPPort: 8080, HTTPSPort: 8443}); err != nil {
		t.Fatal(err)
	}
	f.state.ContainerOK()
	f.state.Loaded(time.Date(2026, 10, 14, 9, 20, 41, 123456789, time.FixedZone("CLT", -3*3600)))

	want := map[string]any{
		"status": "synced", "error": nil, "synced_at": "2026-10-14T12:20:41.123Z", "ports_rollback": nil,
		"caddy":             map[string]any{"image": testImage, "running": true, "http_port": 8080.0, "https_port": 8443.0},
		"panel_certificate": map[string]any{"hostname": "panel.auditfy.cl", "cert_source": "letsencrypt", "origin_ca": nil},
	}
	if got := f.get(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("GET /proxy:\n got %v\nwant %v", got, want)
	}
}

// Deshabilitado (desarrollo): no consulta Docker, ni siquiera si no responde.
func TestProxyStatusDisabled(t *testing.T) {
	f := newProxyFixture(t)
	f.docker.SetUnavailable(true)
	want := map[string]any{
		"status": "disabled", "error": nil, "synced_at": nil, "ports_rollback": nil,
		"caddy": map[string]any{"image": nil, "running": false, "http_port": 80.0, "https_port": 443.0},
		// Sin panel_domain no hay certificado del panel.
		"panel_certificate": nil,
	}
	if got := f.get(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("GET /proxy:\n got %v\nwant %v", got, want)
	}
	if slices.ContainsFunc(f.docker.Calls(), func(c string) bool { return strings.HasPrefix(c, "InspectContainer") }) {
		t.Fatalf("no debería consultar Docker: %v", f.docker.Calls())
	}
}

// panel_certificate (contrato §3.3): con la opción encendida y el panel en
// una zona, el estado de su certificado de Origin CA, con la regla de un
// dominio.
func TestProxyStatusPanelCertificate(t *testing.T) {
	f := newProxyFixture(t)
	f.enable(t, func(p *entity.ProxySettings) {
		p.CloudflareToken, p.CloudflareOriginCA = "cf-token-1234567890abcdef", true
	})
	f.docker.SetImagePresent(testImage)
	ctx := context.Background()
	if err := f.settings.SavePanelDNS(ctx, service.PanelDNS{ZoneID: "z1"}); err != nil {
		t.Fatal(err)
	}
	exp := f.clock.Now().Add(90 * 24 * time.Hour)
	c := ingressentity.OriginCert{Hostname: "panel.auditfy.cl", ZoneID: "z1", CFCertID: "1", CertPEM: "PEM", KeyPEM: "KEY",
		ExpiresAt: exp, CreatedAt: f.clock.Now()}
	if err := f.origin.Certs.Insert(ctx, &c); err != nil {
		t.Fatal(err)
	}
	if err := f.origin.Hosts.SetObserved(ctx, "panel.auditfy.cl", ingressentity.ObservedProxied, f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	w := f.client.Do("GET", "/api/v1/proxy", nil, httptestutil.WithSession("s"))
	httptestutil.Expect(t, w, http.StatusOK)
	want := `"panel_certificate":{"hostname":"panel.auditfy.cl","cert_source":"cloudflare_origin_ca","origin_ca":{"status":"active","error_code":null,"error":null,"retry_at":null,"expires_at":"2027-01-19T12:00:00Z"}}`
	if !strings.Contains(w.Body.String(), want) {
		t.Fatalf("JSON:\n%s\nesperaba\n%s", w.Body, want)
	}
	// En DNS only en Cloudflare: dns_only, con expires_at del que tiene.
	if err := f.origin.Hosts.SetObserved(ctx, "panel.auditfy.cl", ingressentity.ObservedDNSOnly, f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	got := f.get(t)["panel_certificate"].(map[string]any)
	if got["cert_source"] != "letsencrypt" || got["origin_ca"].(map[string]any)["status"] != "dns_only" {
		t.Fatalf("dns_only: %v", got)
	}
}

// Si muelle-caddy no arrancó con los puertos nuevos y muelle volvió a los
// anteriores (contrato §3.5), ports_rollback lo dice: los que fallaron, a los
// que volvió, por qué y cuándo (en UTC, con a lo más ms). caddy trae los
// puertos de los ajustes, ya vueltos.
func TestProxyStatusPortsRollback(t *testing.T) {
	f := newProxyFixture(t)
	f.enable(t, func(p *entity.ProxySettings) { p.HTTPPort, p.HTTPSPort = 8080, 8443 })
	f.state.ContainerOK()
	f.state.Loaded(time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC))
	f.state.PortsRolledBack(usecases.PortsRollback{
		Attempted: service.ProxyPorts{HTTPPort: 80, HTTPSPort: 443},
		Restored:  service.ProxyPorts{HTTPPort: 8080, HTTPSPort: 8443},
		Error:     "los puertos 80 o 443 del host ya están en uso",
		At:        time.Date(2026, 10, 14, 9, 20, 41, 123456789, time.FixedZone("CLT", -3*3600)),
	})
	got := f.get(t)
	want := map[string]any{
		"attempted": map[string]any{"http_port": 80.0, "https_port": 443.0},
		"restored":  map[string]any{"http_port": 8080.0, "https_port": 8443.0},
		"error":     "los puertos 80 o 443 del host ya están en uso",
		"at":        "2026-10-14T12:20:41.123Z",
	}
	if !reflect.DeepEqual(got["ports_rollback"], want) || got["status"] != "synced" {
		t.Fatalf("GET /proxy: %v", got)
	}
	f.state.ClearPortsRollback()
	if got := f.get(t); got["ports_rollback"] != nil {
		t.Fatalf("limpio: %v", got)
	}
}

func TestProxyStatusDockerDown(t *testing.T) {
	f := newProxyFixture(t)
	f.enable(t, nil)
	f.docker.SetUnavailable(true)
	w := f.client.Do("GET", "/api/v1/proxy", nil, httptestutil.WithSession("s"))
	httptestutil.Expect(t, w, http.StatusServiceUnavailable)
	if code := httptestutil.ErrorCode(t, w); code != "docker_unavailable" {
		t.Fatalf("code: %s", code)
	}
}

func TestProxyStatusRecreating(t *testing.T) {
	f := newProxyFixture(t)
	f.enable(t, nil)
	f.state.ContainerFailed("los puertos 80 o 443 del host ya están en uso")
	f.state.BeginRecreate()
	got := f.get(t)
	if got["status"] != "recreating" || got["error"] != nil {
		t.Fatalf("GET /proxy: %v", got)
	}
}

// Solo con sesión (contrato §3): un token recibe 403 token_not_allowed.
func TestProxyStatusToken(t *testing.T) {
	f := newProxyFixture(t)
	f.enable(t, nil)
	w := f.client.Do("GET", "/api/v1/proxy", nil, httptestutil.WithBearer("tok"))
	httptestutil.Expect(t, w, http.StatusForbidden)
	if code := httptestutil.ErrorCode(t, w); code != "token_not_allowed" {
		t.Fatalf("code: %s", code)
	}
}
