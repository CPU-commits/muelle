package container

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/internal/ingress"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare/cftest"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
)

// Contratos de ingress (Fase 3) con el contenedor completo. Todos usan un
// Cloudflare falso (WithIngress con un cftest): ninguno llega a la API real.

// newIngressContainer arma el contenedor con Docker en memoria y el
// Cloudflare falso cf.
func newIngressContainer(t *testing.T, cf *cftest.Server) api {
	t.Helper()
	c, err := New(context.Background(), testSettings(t), testLogger(t), WithDocker(dockertest.New()),
		WithIngress(ingress.Options{CloudflareURL: cf.URL()}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return api{t: t, c: c}
}

// errorMessage es el mensaje de una respuesta {"error": {"code", "message"}}.
func errorMessage(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	return httptestutil.Decode[struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}](t, w).Error.Message
}

// admin → ingress (contrato §3.5 y §4.1): PATCH /settings con un token lo
// verifica ingress antes de guardarlo, y guarda cuántas zonas ve.
func TestContractCloudflareTokenThroughSettings(t *testing.T) {
	const token = "cf-token-1234567890abcdef"
	cf := cftest.New(t)
	cf.AddToken(token)
	cf.AddZone("z1", "tudominio.cl")
	cf.AddZone("z2", "otro.cl")
	e := newIngressContainer(t, cf)
	s := e.setup()

	w := e.do("PATCH", "/api/v1/settings", map[string]any{"cloudflare_token": token}, s)
	httptestutil.Expect(t, w, http.StatusOK)
	got := httptestutil.Decode[map[string]any](t, w)
	if got["cloudflare_zones"] != 2.0 || got["cloudflare_token_set"] != true || strings.Contains(w.Body.String(), token) {
		t.Fatalf("PATCH: %s", w.Body)
	}

	w = e.do("PATCH", "/api/v1/settings", map[string]any{"cloudflare_token": "cf-token-desconocido-123456"}, s)
	expect(t, "un token que Cloudflare no conoce", w, http.StatusUnprocessableEntity, "invalid")
	if msg := errorMessage(t, w); !strings.HasPrefix(msg, "cloudflare_token: ") || strings.Contains(msg, "desconocido") {
		t.Fatalf("mensaje: %q", msg)
	}

	cf.SetDown(true)
	w = e.do("PATCH", "/api/v1/settings", map[string]any{"cloudflare_token": "cf-token-otro-1234567890"}, s)
	expect(t, "Cloudflare caído", w, http.StatusBadGateway, "cloudflare_unavailable")

	// Nada de lo que falló se guardó: sigue el primer token, con sus zonas.
	w = e.do("GET", "/api/v1/settings", nil, s)
	httptestutil.Expect(t, w, http.StatusOK)
	if got := httptestutil.Decode[map[string]any](t, w); got["cloudflare_zones"] != 2.0 || got["cloudflare_token_set"] != true {
		t.Fatalf("GET: %v", got)
	}
}

// admin → ingress (contrato §3.5, §3.7 y §4.1): cloudflare_origin_ca de
// punta a punta. Encenderla sin token, o con un token sin Zone → SSL and
// Certificates → Edit en una zona activa, es 422 y no guarda nada; con el
// permiso se guarda y GET /settings la trae. El dominio trae cert_source y
// origin_ca.
func TestContractOriginCAThroughSettings(t *testing.T) {
	const token = "cf-token-1234567890abcdef"
	cf := cftest.New(t)
	cf.AddToken(token)
	cf.AddZone("z1", "tudominio.cl")
	cf.AddZone("z2", "nueva.cl")
	cf.SetZoneStatus("z2", "pending") // no se revisa
	e := newIngressContainer(t, cf)
	s := e.setup()

	w := e.do("PATCH", "/api/v1/settings", map[string]any{"cloudflare_origin_ca": true}, s)
	expect(t, "sin token", w, http.StatusUnprocessableEntity, "invalid")
	if msg := errorMessage(t, w); msg != "cloudflare_origin_ca: necesita el token de Cloudflare: configúralo antes de activarla" {
		t.Fatalf("sin token: %q", msg)
	}
	httptestutil.Expect(t, e.do("PATCH", "/api/v1/settings", map[string]any{"cloudflare_token": token}, s), http.StatusOK)

	w = e.do("PATCH", "/api/v1/settings", map[string]any{"cloudflare_origin_ca": true, "public_ip": "203.0.113.24"}, s)
	expect(t, "sin el permiso", w, http.StatusUnprocessableEntity, "invalid")
	want := "cloudflare_origin_ca: el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona tudominio.cl (lo necesitan los certificados de Cloudflare): agrégalo al token en Cloudflare, o deja la opción apagada"
	if msg := errorMessage(t, w); msg != want || strings.Contains(msg, token) {
		t.Fatalf("sin el permiso: %q", msg)
	}
	cf.GrantSSL(token, "z1")
	cf.SetDown(true)
	expect(t, "Cloudflare caído", e.do("PATCH", "/api/v1/settings", map[string]any{"cloudflare_origin_ca": true}, s),
		http.StatusBadGateway, "cloudflare_unavailable")
	w = e.do("GET", "/api/v1/settings", nil, s)
	if got := httptestutil.Decode[map[string]any](t, w); got["cloudflare_origin_ca"] != false || got["public_ip"] != "" {
		t.Fatalf("no se guardó nada: %v", got)
	}

	cf.SetDown(false)
	w = e.do("PATCH", "/api/v1/settings", map[string]any{"cloudflare_origin_ca": true}, s)
	httptestutil.Expect(t, w, http.StatusOK)
	if got := httptestutil.Decode[map[string]any](t, w); got["cloudflare_origin_ca"] != true {
		t.Fatalf("PATCH: %v", got)
	}
	w = e.do("GET", "/api/v1/settings", nil, s)
	if got := httptestutil.Decode[map[string]any](t, w); got["cloudflare_origin_ca"] != true {
		t.Fatalf("GET: %v", got)
	}
	// Quitar el token sin apagarla: 422.
	expect(t, "quitar el token", e.do("PATCH", "/api/v1/settings", map[string]any{"cloudflare_token": ""}, s),
		http.StatusUnprocessableEntity, "invalid")

	// Un dominio: el proxy está deshabilitado en el contenedor de prueba (sin
	// panel_domain), así que origin_ca es null y cert_source letsencrypt.
	const P = "/api/v1/projects/p"
	e.create(s, "/api/v1/projects", map[string]string{"slug": "p", "name": "P"})
	e.create(s, P+"/stages", map[string]string{"slug": "tmp"})
	e.create(s, P+"/stages/tmp/apps", map[string]any{"slug": "api", "kind": "service", "spec": map[string]any{"image": "x:1", "port": 80}})
	got := e.create(s, P+"/stages/tmp/apps/api/domains", map[string]any{"hostname": "api.tudominio.cl", "target_service": "api", "target_port": 80})
	if got["cert_source"] != "letsencrypt" || got["origin_ca"] != nil {
		t.Fatalf("dominio: %v", got)
	}
	if _, ok := got["origin_ca"]; !ok {
		t.Fatalf("falta origin_ca: %v", got)
	}
}

// Las claves panel_cf_* (contrato §3.6) son de ingress: GET /settings no las
// muestra y PATCH /settings no las acepta.
func TestContractPanelKeysHidden(t *testing.T) {
	e := newIngressContainer(t, cftest.New(t))
	s := e.setup()
	for _, k := range []string{shared.SettingPanelCFZoneID, shared.SettingPanelCFRecordID, shared.SettingPanelCFRecordIDAAAA} {
		e.exec(`INSERT INTO settings (key, value, encrypted) VALUES (?, 'x', 0)`, k)
	}
	w := e.do("GET", "/api/v1/settings", nil, s)
	httptestutil.Expect(t, w, http.StatusOK)
	for k := range httptestutil.Decode[map[string]any](t, w) {
		if strings.HasPrefix(k, "panel_cf_") {
			t.Fatalf("GET /settings muestra %s: %s", k, w.Body)
		}
	}
	expect(t, "PATCH de una clave del panel", e.do("PATCH", "/api/v1/settings", map[string]any{"panel_cf_zone_id": "x"}, s),
		http.StatusBadRequest, "invalid_json")
}

// catalog ↔ ingress: un dominio creado por la API (sin token: tls http, sin
// Cloudflare) cuenta para catalog, que no borra la app mientras lo tenga.
func TestContractDomainsWired(t *testing.T) {
	cf := cftest.New(t)
	e := newIngressContainer(t, cf)
	s := e.setup()
	const P = "/api/v1/projects/p"
	const D = P + "/stages/tmp/apps/api/domains"
	e.create(s, "/api/v1/projects", map[string]string{"slug": "p", "name": "P"})
	e.create(s, P+"/stages", map[string]string{"slug": "tmp"})
	e.create(s, P+"/stages/tmp/apps", map[string]any{"slug": "api", "kind": "service", "spec": map[string]any{"image": "x:1", "port": 80}})

	got := e.create(s, D, map[string]any{"hostname": "api.p.cl", "target_service": "api", "target_port": 80})
	if got["tls_mode"] != "http" || got["status"] != "pending" || got["proxied"] != false || got["cf_record_id"] != nil {
		t.Fatalf("POST: %v", got)
	}
	expect(t, "borrar la app con un dominio", e.do("DELETE", P+"/stages/tmp/apps/api", nil, s), http.StatusConflict, "not_empty")

	httptestutil.Expect(t, e.do("DELETE", D+"/api.p.cl", nil, s), http.StatusNoContent)
	httptestutil.Expect(t, e.do("DELETE", P+"/stages/tmp/apps/api", nil, s), http.StatusNoContent)
	if calls := cf.Calls(); len(calls) != 0 {
		t.Fatalf("sin token no llama a Cloudflare: %v", calls)
	}
}
