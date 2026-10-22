package http

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/ingress/app/usecases"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	reposqlite "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/repository/sqlite"
	outservice "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/caddy"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/caddy/caddytest"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare/cftest"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

const (
	cfToken = "cf-token-1234567890abcdef"
	apiPath = "/api/v1/projects/auditfy/stages/staging/apps/api"
)

// unreachableProbe es una sonda que nunca llega a Caddy: la verificación no
// cambia nada.
type unreachableProbe struct{}

func (unreachableProbe) Probe(context.Context, string) (service.CertObservation, error) {
	return service.CertObservation{}, service.ErrCaddyUnreachable
}

// domainsFixture arma las rutas de los dominios con los casos de uso de
// verdad: sqlite, cftest con el cliente real (la zona z1 tudominio.cl),
// caddytest con el Syncer y una sonda que no llega a Caddy.
type domainsFixture struct {
	db     *sqlite.DB
	cipher *crypto.Cipher
	cf     *cftest.Server
	client *httptestutil.Client
}

func newDomainsFixture(t *testing.T) *domainsFixture {
	t.Helper()
	db := sqlitetest.New(t)
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := &domainsFixture{db: db, cipher: c, cf: cftest.New(t)}
	f.cf.AddToken(cfToken)
	f.cf.AddZone("z1", "tudominio.cl")
	admin := caddytest.New(t)
	clk := clock.NewManual(time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC))
	settings := outservice.NewProxySettingsReader(db, c)
	catalog := outservice.NewCatalogReader(db)
	repo := reposqlite.NewDomains(db)
	state := usecases.NewProxyState()
	state.Loaded(clk.Now())
	syncer := usecases.NewSyncer(settings, repo, catalog, usecases.OriginRepos{}, caddy.NewAdmin(admin.Dial), state, clk, "", false, nil, log)
	ops := &usecases.Ops{}
	d := usecases.DomainDeps{
		Repo: repo, Catalog: catalog, Settings: settings, DNS: cloudflare.New(f.cf.URL()), Syncer: syncer,
		Certs: usecases.NewCertChecker(unreachableProbe{}, repo, state, ops, clk, time.Second, time.Second, time.Second, log),
		Ops:   ops, Clock: clk, Audit: sqlite.NewAuditRecorder(db, clk, log), Log: log,
		DomainOp: 30 * time.Second, DomainRetry: 20 * time.Second,
	}
	uc := UseCases{
		ListDomains: usecases.NewListDomains(d), CreateDomain: usecases.NewCreateDomain(d), UpdateDomain: usecases.NewUpdateDomain(d),
		DeleteDomain: usecases.NewDeleteDomain(d), AdoptDomain: usecases.NewAdoptDomain(d), RetryDomain: usecases.NewRetryDomain(d),
	}
	f.client = httptestutil.New(t, httptestutil.FakeAuth{
		Sessions: map[string]entity.Principal{"s": {UserID: 1, Username: "felipe"}},
		Tokens:   map[string]entity.Principal{"tok": {UserID: 1, Username: "felipe"}},
	}, func(k *utils.Kit) { Register(k, uc) })
	ingresstest.App(t, db, "auditfy", "staging", "api", "service")
	ingresstest.App(t, db, "auditfy", "staging", "infra", "compose")
	p := entity.DefaultProxySettings()
	p.PanelDomain, p.CaddyImage, p.CloudflareToken, p.PublicIP = "panel.tudominio.cl", "caddy:2-alpine", cfToken, "203.0.113.24"
	ingresstest.SetProxySettings(t, db, c, p)
	return f
}

// create crea host en la app api (api:3000) y falla si no da 201.
func (f *domainsFixture) create(t *testing.T, host string) map[string]any {
	t.Helper()
	w := f.client.Do("POST", apiPath+"/domains", map[string]any{"hostname": host, "target_service": "api", "target_port": 3000},
		httptestutil.WithSession("s"))
	httptestutil.Expect(t, w, http.StatusCreated)
	return httptestutil.Decode[map[string]any](t, w)
}

// expectError comprueba el status, el code y, si no es "", el comienzo del mensaje.
func expectError(t *testing.T, c *httptestutil.Client, method, path string, body any, status int, code, prefix string) {
	t.Helper()
	w := c.Do(method, path, body, httptestutil.WithSession("s"))
	httptestutil.Expect(t, w, status)
	got := httptestutil.Decode[map[string]map[string]string](t, w)["error"]
	if got["code"] != code || !strings.HasPrefix(got["message"], prefix) {
		t.Fatalf("%s %s: %v, esperaba %s %q…", method, path, got, code, prefix)
	}
}

// domainKeys son las claves del dominio del contrato §3.1: siempre todas.
var domainKeys = []string{
	"cert_expires_at", "cert_issuer", "cf_record_id", "cf_zone_id", "created_at", "hostname", "last_error", "last_error_code",
	"proxied", "records", "status", "target_port", "target_service", "tls_mode", "updated_at",
}

func checkKeys(t *testing.T, d map[string]any) {
	t.Helper()
	if got := slices.Sorted(maps.Keys(d)); !slices.Equal(got, domainKeys) {
		t.Fatalf("claves: %v", got)
	}
}

func TestHTTPListDomains(t *testing.T) {
	f := newDomainsFixture(t)
	w := f.client.Do("GET", apiPath+"/domains", nil, httptestutil.WithSession("s"))
	httptestutil.Expect(t, w, http.StatusOK)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("sin dominios: %s", w.Body)
	}
	f.create(t, "b.tudominio.cl")
	f.create(t, "a.tudominio.cl")
	w = f.client.Do("GET", apiPath+"/domains", nil, httptestutil.WithSession("s"))
	httptestutil.Expect(t, w, http.StatusOK)
	got := httptestutil.Decode[[]map[string]any](t, w)
	if len(got) != 2 || got[0]["hostname"] != "a.tudominio.cl" || got[1]["hostname"] != "b.tudominio.cl" {
		t.Fatalf("%v", got)
	}
	checkKeys(t, got[0])
	expectError(t, f.client, "GET", "/api/v1/projects/auditfy/stages/staging/apps/nada/domains", nil, 404, "not_found", "")
}

func TestHTTPCreateDomain(t *testing.T) {
	f := newDomainsFixture(t)
	d := f.create(t, "api.tudominio.cl")
	checkKeys(t, d)
	if d["proxied"] != false || d["status"] != "pending" || d["tls_mode"] != "cloudflare" || d["cf_zone_id"] != "z1" ||
		d["cf_record_id"] == nil || d["last_error"] != nil || d["last_error_code"] != nil || d["cert_issuer"] != nil ||
		d["cert_expires_at"] != nil || d["target_port"] != 3000.0 || d["created_at"] != "2026-10-14T12:00:00Z" {
		t.Fatalf("%v", d)
	}
	records, _ := d["records"].([]any)
	if len(records) != 1 || records[0].(map[string]any)["content"] != "203.0.113.24" {
		t.Fatalf("records: %v", d["records"])
	}

	post := apiPath + "/domains"
	expectError(t, f.client, "POST", post, map[string]any{"hostname": "x.tudominio.cl", "target_service": "api", "target_port": 3000, "project": "x"},
		400, "invalid_json", "")
	expectError(t, f.client, "POST", post, `{"hostname":"x.tudominio.cl","target_service":"api","target_port":"3000"}`, 400, "invalid_json", "")
	expectError(t, f.client, "POST", post, map[string]any{"hostname": "X_x", "target_service": "api", "target_port": 3000}, 422, "invalid", "hostname: ")
	expectError(t, f.client, "POST", post, map[string]any{"hostname": "panel.tudominio.cl", "target_service": "api", "target_port": 3000},
		422, "invalid", "hostname: es el dominio del panel")
	expectError(t, f.client, "POST", post, map[string]any{"hostname": "x.tudominio.cl", "target_service": "web", "target_port": 3000}, 422, "invalid", "target_service: ")
	expectError(t, f.client, "POST", post, map[string]any{"hostname": "x.tudominio.cl", "target_service": "api", "target_port": 70000}, 422, "invalid", "target_port: ")
	expectError(t, f.client, "POST", "/api/v1/projects/auditfy/stages/staging/apps/infra/domains",
		map[string]any{"hostname": "api.tudominio.cl", "target_service": "redis", "target_port": 6379}, 409, "conflict", "Ese hostname ya está en uso")
	expectError(t, f.client, "POST", "/api/v1/projects/auditfy/stages/staging/apps/nada/domains",
		map[string]any{"hostname": "x.tudominio.cl", "target_service": "nada", "target_port": 3000}, 404, "not_found", "")
}

func TestHTTPUpdateDomain(t *testing.T) {
	f := newDomainsFixture(t)
	f.create(t, "api.tudominio.cl")
	f.create(t, "web.otro.com") // fuera de las zonas: sin registro propio
	path := apiPath + "/domains/api.tudominio.cl"

	w := f.client.Do("PATCH", path, map[string]any{}, httptestutil.WithSession("s"))
	httptestutil.Expect(t, w, http.StatusOK)
	checkKeys(t, httptestutil.Decode[map[string]any](t, w))
	w = f.client.Do("PATCH", path, map[string]any{"target_port": 4000}, httptestutil.WithSession("s"))
	httptestutil.Expect(t, w, http.StatusOK)
	if got := httptestutil.Decode[map[string]any](t, w); got["target_port"] != 4000.0 {
		t.Fatalf("%v", got)
	}
	expectError(t, f.client, "PATCH", path, map[string]any{"hostname": "x"}, 400, "invalid_json", "")
	expectError(t, f.client, "PATCH", apiPath+"/domains/web.otro.com", map[string]any{"proxied": true}, 422, "invalid",
		"proxied: muelle no maneja el registro DNS de este dominio")
	expectError(t, f.client, "PATCH", path, map[string]any{"target_service": "otro"}, 422, "invalid", "target_service: ")
	expectError(t, f.client, "PATCH", "/api/v1/projects/auditfy/stages/staging/apps/infra/domains/api.tudominio.cl", map[string]any{},
		404, "not_found", "")
}

func TestHTTPDeleteDomain(t *testing.T) {
	f := newDomainsFixture(t)
	f.create(t, "api.tudominio.cl")
	f.create(t, "web.tudominio.cl")

	f.cf.SetDown(true)
	expectError(t, f.client, "DELETE", apiPath+"/domains/api.tudominio.cl", nil, 502, "cloudflare_unavailable", "")
	f.cf.SetDown(false)
	w := f.client.Do("GET", apiPath+"/domains", nil, httptestutil.WithSession("s"))
	if got := httptestutil.Decode[[]map[string]any](t, w); len(got) != 2 {
		t.Fatalf("el dominio debía seguir: %v", got)
	}

	w = f.client.Do("DELETE", apiPath+"/domains/web.tudominio.cl", nil, httptestutil.WithSession("s"))
	httptestutil.Expect(t, w, http.StatusNoContent)
	if w.Body.Len() != 0 {
		t.Fatalf("cuerpo: %q", w.Body)
	}
	expectError(t, f.client, "DELETE", apiPath+"/domains/web.tudominio.cl", nil, 404, "not_found", "")
}

func TestHTTPAdoptDomain(t *testing.T) {
	f := newDomainsFixture(t)
	for _, h := range []string{"a.tudominio.cl", "b.tudominio.cl"} {
		f.cf.PutRecord("z1", service.Record{Type: "A", Name: h, Content: "198.51.100.7", Proxied: true})
		if d := f.create(t, h); d["last_error_code"] != "record_exists" {
			t.Fatalf("%v", d)
		}
	}
	w := f.client.Do("POST", apiPath+"/domains/a.tudominio.cl/adopt", nil, httptestutil.WithSession("s"))
	httptestutil.Expect(t, w, http.StatusOK)
	if got := httptestutil.Decode[map[string]any](t, w); got["status"] != "pending" || got["proxied"] != true || got["cf_record_id"] == nil {
		t.Fatalf("%v", got)
	}
	expectError(t, f.client, "POST", apiPath+"/domains/b.tudominio.cl/adopt", map[string]any{"x": 1}, 400, "invalid_json", "")
	w = f.client.Do("POST", apiPath+"/domains/b.tudominio.cl/adopt", "{}", httptestutil.WithSession("s"))
	httptestutil.Expect(t, w, http.StatusOK)
	expectError(t, f.client, "POST", apiPath+"/domains/a.tudominio.cl/adopt", nil, 409, "nothing_to_adopt", "No hay un registro que adoptar")
}

func TestHTTPRetryDomain(t *testing.T) {
	f := newDomainsFixture(t)
	f.create(t, "api.tudominio.cl")
	path := apiPath + "/domains/api.tudominio.cl/retry"
	for _, body := range []any{nil, "{}", " \n"} {
		w := f.client.Do("POST", path, body, httptestutil.WithSession("s"))
		httptestutil.Expect(t, w, http.StatusOK)
		checkKeys(t, httptestutil.Decode[map[string]any](t, w))
	}
	for _, body := range []string{"null", "[]", `{"x":1}`} {
		expectError(t, f.client, "POST", path, body, 400, "invalid_json", "")
	}
	expectError(t, f.client, "POST", apiPath+"/domains/nada.tudominio.cl/retry", nil, 404, "not_found", "")
}

// Solo con sesión (contrato §3): un token recibe 403 token_not_allowed.
func TestHTTPDomainsToken(t *testing.T) {
	f := newDomainsFixture(t)
	for _, r := range []struct{ method, path string }{
		{"GET", apiPath + "/domains"},
		{"POST", apiPath + "/domains"},
		{"PATCH", apiPath + "/domains/a.tudominio.cl"},
		{"DELETE", apiPath + "/domains/a.tudominio.cl"},
		{"POST", apiPath + "/domains/a.tudominio.cl/adopt"},
		{"POST", apiPath + "/domains/a.tudominio.cl/retry"},
	} {
		w := f.client.Do(r.method, r.path, nil, httptestutil.WithBearer("tok"))
		httptestutil.Expect(t, w, http.StatusForbidden)
		if code := httptestutil.ErrorCode(t, w); code != "token_not_allowed" {
			t.Fatalf("%s %s: %s", r.method, r.path, code)
		}
	}
}
