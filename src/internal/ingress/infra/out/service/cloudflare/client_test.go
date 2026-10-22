package cloudflare

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json/v2"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare/cftest"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

const token = "cf-token-secreto-123"

// noToken falla si el texto del error trae el token.
func noToken(t *testing.T, err error) {
	t.Helper()
	if err != nil && strings.Contains(err.Error(), token) {
		t.Fatalf("el error trae el token: %q", err)
	}
}

// capture es un httptest propio que guarda la última petición y responde body.
type capture struct {
	mu     sync.Mutex
	header http.Header
	method string
	uri    string
	body   []byte
}

func newCapture(t *testing.T, status int, body string) (*capture, string) {
	t.Helper()
	c := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.header, c.method, c.uri, c.body = r.Header.Clone(), r.Method, r.URL.RequestURI(), b
		c.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return c, srv.URL + "/client/v4"
}

func (c *capture) bodyMap(t *testing.T) map[string]any {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	var m map[string]any
	if err := json.Unmarshal(c.body, &m); err != nil {
		t.Fatalf("cuerpo %q: %v", c.body, err)
	}
	return m
}

func TestNewDefaultURL(t *testing.T) {
	if c := New(""); c.base != DefaultBaseURL || DefaultBaseURL != "https://api.cloudflare.com/client/v4" {
		t.Fatalf("base: %q", c.base)
	}
	if c := New("http://x/client/v4/"); c.base != "http://x/client/v4" {
		t.Fatalf("base con / final: %q", c.base)
	}
	if New("").http.Timeout != 15*time.Second {
		t.Fatal("el cliente tiene un plazo de 15 s")
	}
}

func TestVerifyToken(t *testing.T) {
	cf := cftest.New(t)
	cf.AddToken(token)
	cf.SetTokenStatus("desactivado-"+token, "disabled")
	cf.SetTokenStatus("vencido-"+token, "expired")
	c := New(cf.URL())
	ctx := context.Background()

	if err := c.VerifyToken(ctx, token); err != nil {
		t.Fatalf("activo: %v", err)
	}
	err := c.VerifyToken(ctx, "otro-"+token)
	noToken(t, err)
	var de *service.DNSError
	if !errors.As(err, &de) || de.Status != 401 || de.Code != 1000 || !strings.Contains(err.Error(), "Invalid API Token") {
		t.Fatalf("rechazado: %v", err)
	}
	if errors.Is(err, entity.ErrCloudflareUnavailable) || errors.Is(err, service.ErrDNSNotFound) {
		t.Fatalf("un rechazo no es 'no responde' ni 'no existe': %v", err)
	}
	// verify responde 200 también con un token desactivado o vencido (D31).
	for tok, word := range map[string]string{"desactivado-" + token: "desactivado", "vencido-" + token: "vencido"} {
		err := c.VerifyToken(ctx, tok)
		noToken(t, err)
		if !errors.As(err, &de) || de.Status != 200 || !strings.Contains(err.Error(), word) {
			t.Errorf("%s: %v", word, err)
		}
	}
	if calls := cf.Calls(); calls[0] != "GET /user/tokens/verify" {
		t.Fatalf("ruta: %q", calls)
	}

	// Un estado desconocido sale tal cual; el header es Authorization: Bearer.
	cp, url := newCapture(t, 200, `{"success":true,"errors":[],"result":{"id":"x","status":"raro"}}`)
	err = New(url).VerifyToken(ctx, token)
	if !errors.As(err, &de) || de.Status != 200 || !strings.Contains(err.Error(), "raro") {
		t.Fatalf("estado raro: %v", err)
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	if got := cp.header.Get("Authorization"); got != "Bearer "+token {
		t.Fatalf("Authorization: %q", got)
	}
	if cp.method != "GET" || cp.uri != "/client/v4/user/tokens/verify" {
		t.Fatalf("petición: %s %s", cp.method, cp.uri)
	}
}

// Un token con espacios o saltos de línea no sale en un header: se rechaza
// sin llamar a Cloudflare, sin traer el token en el error.
func TestBadTokenNotSent(t *testing.T) {
	cf := cftest.New(t)
	c := New(cf.URL())
	for _, tok := range []string{"", "a b", "a\r\nX-Otro: 1", "ñandú"} {
		err := c.VerifyToken(context.Background(), tok)
		var de *service.DNSError
		if !errors.As(err, &de) || (tok != "" && strings.Contains(err.Error(), tok)) {
			t.Errorf("%q: %v", tok, err)
		}
	}
	if calls := cf.Calls(); len(calls) != 0 {
		t.Fatalf("llamó a Cloudflare: %q", calls)
	}
}

func TestUnavailable(t *testing.T) {
	ctx := context.Background()
	check := func(name string, err error) {
		t.Helper()
		noToken(t, err)
		if !errors.Is(err, entity.ErrCloudflareUnavailable) {
			t.Errorf("%s: %v, esperaba ErrCloudflareUnavailable", name, err)
		}
		var de *service.DNSError
		if errors.As(err, &de) {
			t.Errorf("%s: no es un rechazo: %v", name, err)
		}
	}

	cf := cftest.New(t)
	cf.AddToken(token)
	cf.AddZone("z1", "x.cl")
	c := New(cf.URL())
	cf.SetDown(true)
	check("503", c.VerifyToken(ctx, token))
	_, err := c.Zones(ctx, token)
	check("503 en zonas", err)
	cf.SetDown(false)

	cf.FailNext("GET", "/zones", 429, 10000, "Rate limited")
	_, err = c.Zones(ctx, token)
	check("429", err)
	cf.FailNext("POST", "/dns_records", 500, 10001, "boom")
	_, err = c.CreateRecord(ctx, token, "z1", service.Record{Type: "A", Name: "a.x.cl", Content: "203.0.113.24"})
	check("500", err)

	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	check("sin red", New(closed.URL).VerifyToken(ctx, token))

	for name, body := range map[string]string{
		"no es JSON":         `<html>hola</html>`,
		"sin result":         `{"success":true,"errors":[]}`,
		"result ilegible":    `{"success":true,"errors":[],"result":"no es un objeto"}`,
		"más de 1 MiB":       `{"success":true,"errors":[],"result":{"status":"active","x":"` + strings.Repeat("a", 1<<20) + `"}}`,
		"404 que no es JSON": "",
	} {
		status := 200
		if name == "404 que no es JSON" {
			status, body = 404, "<html>no</html>"
		}
		_, url := newCapture(t, status, body)
		check(name, New(url).VerifyToken(ctx, token))
	}

	cf.Hang(time.Second)
	dctx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	err = c.VerifyToken(dctx, token)
	check("ctx vencido", err)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ctx vencido: también es DeadlineExceeded: %v", err)
	}
}

// Un redirect no se sigue: el token nunca va a otro lado.
func TestNoRedirects(t *testing.T) {
	var hit sync.Once
	reached := false
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit.Do(func() { reached = true }) }))
	t.Cleanup(other.Close)
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/robado", http.StatusFound)
	}))
	t.Cleanup(redirect.Close)
	err := New(redirect.URL).VerifyToken(context.Background(), token)
	if !errors.Is(err, entity.ErrCloudflareUnavailable) || reached {
		t.Fatalf("redirect: %v (siguió: %v)", err, reached)
	}
}

func TestZonesPaginates(t *testing.T) {
	cf := cftest.New(t)
	cf.AddToken(token)
	for i := range 120 {
		cf.AddZone(fmt.Sprintf("z%03d", i), fmt.Sprintf("zona%03d.cl", i))
	}
	zones, err := New(cf.URL()).Zones(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if len(zones) != 120 || zones[0].ID != "z000" || zones[0].Name != "zona000.cl" || zones[119].ID != "z119" {
		t.Fatalf("%d zonas: %+v…", len(zones), zones[:min(3, len(zones))])
	}
	want := []string{"GET /zones?page=1&per_page=50", "GET /zones?page=2&per_page=50", "GET /zones?page=3&per_page=50"}
	if got := cf.Calls(); !reflect.DeepEqual(got, want) {
		t.Fatalf("llamadas: %q", got)
	}

	// Sin zonas: [] en una sola llamada.
	empty := cftest.New(t)
	empty.AddToken(token)
	if zones, err := New(empty.URL()).Zones(context.Background(), token); err != nil || zones == nil || len(zones) != 0 {
		t.Fatalf("sin zonas: %#v %v", zones, err)
	}
}

func TestRecordsByName(t *testing.T) {
	cf := cftest.New(t)
	cf.AddToken(token)
	cf.AddZone("z1", "x.cl")
	want := []service.Record{
		{ID: cf.PutRecord("z1", service.Record{Type: "A", Name: "a.x.cl", Content: "203.0.113.1", Proxied: true}), Type: "A", Name: "a.x.cl", Content: "203.0.113.1", Proxied: true},
		{ID: cf.PutRecord("z1", service.Record{Type: "AAAA", Name: "a.x.cl", Content: "2001:db8::1"}), Type: "AAAA", Name: "a.x.cl", Content: "2001:db8::1"},
		{ID: cf.PutRecord("z1", service.Record{Type: "TXT", Name: "a.x.cl", Content: "v=spf1"}), Type: "TXT", Name: "a.x.cl", Content: "v=spf1"},
	}
	cf.PutRecord("z1", service.Record{Type: "CNAME", Name: "b.x.cl", Content: "a.x.cl"})
	cf.PutRecord("z1", service.Record{Type: "A", Name: "x.cl", Content: "203.0.113.9"})
	cname := cf.PutRecord("z1", service.Record{Type: "CNAME", Name: "www.a.x.cl", Content: "a.x.cl"})

	got, err := New(cf.URL()).Records(context.Background(), token, "z1", "a.x.cl")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("registros:\n%+v\nesperaba\n%+v", got, want)
	}
	if calls := cf.Calls(); !reflect.DeepEqual(calls, []string{"GET /zones/z1/dns_records?name.exact=a.x.cl&page=1&per_page=100"}) {
		t.Fatalf("llamadas: %q", calls)
	}
	got, err = New(cf.URL()).Records(context.Background(), token, "z1", "www.a.x.cl")
	if err != nil || len(got) != 1 || got[0].ID != cname || got[0].Type != "CNAME" {
		t.Fatalf("CNAME: %+v %v", got, err)
	}
	if got, err := New(cf.URL()).Records(context.Background(), token, "z1", "nada.x.cl"); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("sin registros: %#v %v", got, err)
	}
	if _, err := New(cf.URL()).Records(context.Background(), token, "nada", "a.x.cl"); !errors.Is(err, service.ErrDNSNotFound) {
		t.Fatalf("zona que no existe: %v", err)
	}
}

func TestCreateRecord(t *testing.T) {
	cp, url := newCapture(t, 200, `{"success":true,"errors":[],"result":{"id":"r1","type":"A","name":"a.x.cl","content":"203.0.113.24","proxied":true,"ttl":1,"comment":"muelle"}}`)
	got, err := New(url).CreateRecord(context.Background(), token, "z1", service.Record{Type: "A", Name: "a.x.cl", Content: "203.0.113.24", Proxied: true})
	if err != nil {
		t.Fatal(err)
	}
	if got != (service.Record{ID: "r1", Type: "A", Name: "a.x.cl", Content: "203.0.113.24", Proxied: true}) {
		t.Fatalf("creado: %+v", got)
	}
	want := map[string]any{"type": "A", "name": "a.x.cl", "content": "203.0.113.24", "ttl": float64(1), "proxied": true, "comment": "muelle"}
	if body := cp.bodyMap(t); !reflect.DeepEqual(body, want) {
		t.Fatalf("cuerpo: %v", body)
	}
	cp.mu.Lock()
	if cp.method != "POST" || cp.uri != "/client/v4/zones/z1/dns_records" || cp.header.Get("Content-Type") != "application/json" {
		t.Errorf("petición: %s %s %q", cp.method, cp.uri, cp.header.Get("Content-Type"))
	}
	cp.mu.Unlock()
	// proxied false también va (no se omite).
	if _, err := New(url).CreateRecord(context.Background(), token, "z1", service.Record{Type: "AAAA", Name: "a.x.cl", Content: "2001:db8::1"}); err != nil {
		t.Fatal(err)
	}
	if body := cp.bodyMap(t); body["proxied"] != false {
		t.Fatalf("proxied false: %v", body)
	}

	// Contra el falso: el id y el comentario quedan guardados.
	cf := cftest.New(t)
	cf.AddToken(token)
	cf.AddZone("z1", "x.cl")
	rec, err := New(cf.URL()).CreateRecord(context.Background(), token, "z1", service.Record{Type: "A", Name: "a.x.cl", Content: "203.0.113.24"})
	if err != nil || rec.ID == "" {
		t.Fatalf("creado: %+v %v", rec, err)
	}
	if c := cf.Comment("z1", rec.ID); c != RecordComment || RecordComment != "muelle" {
		t.Fatalf("comentario: %q", c)
	}
	// Un registro que Cloudflare no acepta (ya hay un CNAME) es un rechazo.
	cf.PutRecord("z1", service.Record{Type: "CNAME", Name: "c.x.cl", Content: "otro.cl"})
	_, err = New(cf.URL()).CreateRecord(context.Background(), token, "z1", service.Record{Type: "A", Name: "c.x.cl", Content: "203.0.113.24"})
	var de *service.DNSError
	if !errors.As(err, &de) || de.Status != 400 || de.Code != 81053 {
		t.Fatalf("conflicto: %v", err)
	}
}

func TestUpdateRecord(t *testing.T) {
	str := func(s string) *string { return &s }
	cp, url := newCapture(t, 200, `{"success":true,"errors":[],"result":{"id":"r1","type":"A","name":"a.x.cl","content":"203.0.113.7","proxied":false}}`)
	if _, err := New(url).UpdateRecord(context.Background(), token, "z1", "r1", service.RecordChange{Content: str("203.0.113.7")}); err != nil {
		t.Fatal(err)
	}
	if body := cp.bodyMap(t); !reflect.DeepEqual(body, map[string]any{"content": "203.0.113.7"}) {
		t.Fatalf("solo content: %v", body)
	}
	cp.mu.Lock()
	if cp.method != "PATCH" || cp.uri != "/client/v4/zones/z1/dns_records/r1" {
		t.Errorf("petición: %s %s", cp.method, cp.uri)
	}
	cp.mu.Unlock()
	f := false
	if _, err := New(url).UpdateRecord(context.Background(), token, "z1", "r1", service.RecordChange{Proxied: &f}); err != nil {
		t.Fatal(err)
	}
	if body := cp.bodyMap(t); !reflect.DeepEqual(body, map[string]any{"proxied": false}) {
		t.Fatalf("solo proxied: %v", body)
	}

	// Un CNAME pasa a A con el mismo proxied (adoptar, contrato §3.2).
	cf := cftest.New(t)
	cf.AddToken(token)
	cf.AddZone("z1", "x.cl")
	id := cf.PutRecord("z1", service.Record{Type: "CNAME", Name: "a.x.cl", Content: "otro.cl", Proxied: true})
	got, err := New(cf.URL()).UpdateRecord(context.Background(), token, "z1", id, service.RecordChange{Type: str("A"), Content: str("203.0.113.24")})
	if err != nil {
		t.Fatal(err)
	}
	want := service.Record{ID: id, Type: "A", Name: "a.x.cl", Content: "203.0.113.24", Proxied: true}
	if got != want || !reflect.DeepEqual(cf.Records("z1"), []service.Record{want}) {
		t.Fatalf("CNAME a A: %+v / %+v", got, cf.Records("z1"))
	}
}

func TestRecordNotFound(t *testing.T) {
	cf := cftest.New(t)
	cf.AddToken(token)
	cf.AddZone("z1", "x.cl")
	id := cf.PutRecord("z1", service.Record{Type: "A", Name: "a.x.cl", Content: "203.0.113.24"})
	c := New(cf.URL())
	ctx := context.Background()

	if got, err := c.Record(ctx, token, "z1", id); err != nil || got.ID != id || got.Content != "203.0.113.24" {
		t.Fatalf("Record: %+v %v", got, err)
	}
	if err := c.DeleteRecord(ctx, token, "z1", id); err != nil {
		t.Fatal(err)
	}
	check := func(name string, err error) {
		t.Helper()
		noToken(t, err)
		var de *service.DNSError
		if !errors.Is(err, service.ErrDNSNotFound) || !errors.As(err, &de) || de.Status != 404 {
			t.Errorf("%s: %v, esperaba ErrDNSNotFound con el *DNSError", name, err)
		}
	}
	_, err := c.Record(ctx, token, "z1", id)
	check("Record", err)
	content := "203.0.113.25"
	_, err = c.UpdateRecord(ctx, token, "z1", id, service.RecordChange{Content: &content})
	check("UpdateRecord", err)
	check("DeleteRecord", c.DeleteRecord(ctx, token, "z1", id))
	// Un id con caracteres raros va escapado en el path.
	check("id raro", c.DeleteRecord(ctx, token, "z1", "../../user/tokens/verify"))
	if calls := cf.Calls(); !strings.Contains(calls[len(calls)-1], "..%2F..%2F") {
		t.Fatalf("id sin escapar: %q", calls[len(calls)-1])
	}
}

func TestSuccessFalse(t *testing.T) {
	cf := cftest.New(t)
	cf.AddToken(token)
	cf.AddZone("z1", "x.cl")
	cf.FailNext("GET", "/zones", 200, 9999, "algo salió mal")
	_, err := New(cf.URL()).Zones(context.Background(), token)
	noToken(t, err)
	var de *service.DNSError
	if !errors.As(err, &de) || de.Status != 200 || de.Code != 9999 || de.Message != "algo salió mal" {
		t.Fatalf("success false: %v", err)
	}
	if errors.Is(err, entity.ErrCloudflareUnavailable) {
		t.Fatalf("success false es un rechazo: %v", err)
	}
}

// Un mensaje de Cloudflare que trajera el token lo trae como ***.
func TestErrorRedactsToken(t *testing.T) {
	_, url := newCapture(t, 403, `{"success":false,"errors":[{"code":10000,"message":"token `+token+` sin permiso"}]}`)
	err := New(url).VerifyToken(context.Background(), token)
	noToken(t, err)
	var de *service.DNSError
	if !errors.As(err, &de) || de.Status != 403 || !strings.Contains(de.Message, "token *** sin permiso") {
		t.Fatalf("403: %v", err)
	}
}

// Zones trae el status y los permissions de cada zona (§3.3 del diseño de
// Origin CA): permissions está deprecado y puede no venir.
func TestZonesStatusAndPermissions(t *testing.T) {
	cf := cftest.New(t)
	cf.AddToken(token)
	cf.AddZone("z1", "a.cl")
	cf.AddZone("z2", "b.cl")
	cf.GrantSSL(token, "z1")
	cf.SetZoneStatus("z2", "pending")
	zones, err := New(cf.URL()).Zones(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if len(zones) != 2 || zones[0].Status != "active" || zones[1].Status != "pending" {
		t.Fatalf("status: %+v", zones)
	}
	if !slices.Contains(zones[0].Permissions, "#ssl:edit") || slices.Contains(zones[1].Permissions, "#ssl:edit") ||
		!slices.Contains(zones[1].Permissions, "#dns_records:edit") {
		t.Fatalf("permissions: %+v", zones)
	}
	cf.OmitZonePermissions()
	zones, err = New(cf.URL()).Zones(context.Background(), token)
	if err != nil || len(zones) != 2 || zones[0].Permissions != nil {
		t.Fatalf("sin permissions: %+v %v", zones, err)
	}
}

// CreateOriginCert manda el cuerpo exacto de POST /certificates
// (origin-ecc, un hostname, la validez y el CSR) y devuelve el id y el PEM.
func TestCreateOriginCert(t *testing.T) {
	cp, url := newCapture(t, 200, `{"success":true,"errors":[],"result":{"id":"328578533902268680212849205732770752308931942346","certificate":"-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n","hostnames":["api.x.cl"],"request_type":"origin-ecc","requested_validity":90,"expires_on":"2027-01-19 12:00:00 +0000 UTC","csr":"CSR"}}`)
	got, err := New(url).CreateOriginCert(context.Background(), token, "api.x.cl", "-----BEGIN CERTIFICATE REQUEST-----\nMIIB\n-----END CERTIFICATE REQUEST-----\n", 90)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "328578533902268680212849205732770752308931942346" || got.CertPEM != "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n" {
		t.Fatalf("emitido: %+v", got)
	}
	want := map[string]any{
		"hostnames": []any{"api.x.cl"}, "request_type": "origin-ecc", "requested_validity": float64(90),
		"csr": "-----BEGIN CERTIFICATE REQUEST-----\nMIIB\n-----END CERTIFICATE REQUEST-----\n",
	}
	if body := cp.bodyMap(t); !reflect.DeepEqual(body, want) {
		t.Fatalf("cuerpo: %v", body)
	}
	cp.mu.Lock()
	if cp.method != "POST" || cp.uri != "/client/v4/certificates" || cp.header.Get("Authorization") != "Bearer "+token {
		t.Errorf("petición: %s %s", cp.method, cp.uri)
	}
	cp.mu.Unlock()

	// Sin id o sin certificado, la respuesta no sirve.
	for _, body := range []string{
		`{"success":true,"errors":[],"result":{"id":"","certificate":"PEM"}}`,
		`{"success":true,"errors":[],"result":{"id":"1","certificate":""}}`,
	} {
		_, url := newCapture(t, 200, body)
		if _, err := New(url).CreateOriginCert(context.Background(), token, "api.x.cl", "CSR", 90); !errors.Is(err, entity.ErrCloudflareUnavailable) {
			t.Errorf("%s: %v", body, err)
		}
	}
}

// Contra el Origin CA falso: con el permiso, la hoja va firmada por su raíz;
// sin él, 400 con 10000; fuera de las zonas, 1010; un CSR que no se lee,
// 1011. Revocar: bien, 404 (ErrDNSNotFound) o 1014 (ya revocado).
func TestOriginCertAgainstFake(t *testing.T) {
	ctx := context.Background()
	cf := cftest.New(t)
	cf.AddToken(token)
	cf.AddZone("z1", "x.cl")
	cf.GrantSSL(token, "z1")
	cf.AddZone("z2", "y.cl")
	c := New(cf.URL())

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: "api.x.cl"}, DNSNames: []string{"api.x.cl"}}, key)
	if err != nil {
		t.Fatal(err)
	}
	csr := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
	got, err := c.CreateOriginCert(ctx, token, "api.x.cl", csr, 90)
	if err != nil {
		t.Fatal(err)
	}
	if is := cf.Issued(); len(is) != 1 || is[0].ID != got.ID || is[0].CertPEM != got.CertPEM || is[0].Validity != 90 || is[0].RequestType != "origin-ecc" {
		t.Fatalf("emitido: %+v %+v", got, is)
	}

	code := func(name string, err error, status, want int) {
		t.Helper()
		noToken(t, err)
		de, ok := errors.AsType[*service.DNSError](err)
		if !ok || de.Code != want || de.Status != status || errors.Is(err, entity.ErrCloudflareUnavailable) {
			t.Errorf("%s: %v, esperaba %d con el código %d", name, err, status, want)
		}
	}
	_, err = c.CreateOriginCert(ctx, token, "api.y.cl", csr, 90)
	code("sin el permiso", err, 400, 10000)
	_, err = c.CreateOriginCert(ctx, token, "api.otra.cl", csr, 90)
	code("fuera de las zonas", err, 400, 1010)
	_, err = c.CreateOriginCert(ctx, token, "api.x.cl", "-----BEGIN CERTIFICATE REQUEST-----\nAAAA\n-----END CERTIFICATE REQUEST-----\n", 90)
	code("CSR ilegible", err, 400, 1011)

	if err := c.RevokeOriginCert(ctx, token, got.ID); err != nil {
		t.Fatal(err)
	}
	if !cf.Revoked(got.ID) {
		t.Fatal("no quedó revocado")
	}
	code("ya revocado", c.RevokeOriginCert(ctx, token, got.ID), 400, 1014)
	if err := c.RevokeOriginCert(ctx, token, "999"); !errors.Is(err, service.ErrDNSNotFound) {
		t.Fatalf("desconocido: %v", err)
	}
	if calls := cf.Calls(); !slices.Contains(calls, "DELETE /certificates/"+got.ID) {
		t.Fatalf("llamadas: %q", calls)
	}

	// 429 y 5xx: Cloudflare no responde.
	cf.FailNext("POST", "/certificates", 429, 971, "Please wait and consider throttling your request speed")
	_, err = c.CreateOriginCert(ctx, token, "api.x.cl", csr, 90)
	if !errors.Is(err, entity.ErrCloudflareUnavailable) {
		t.Fatalf("429: %v", err)
	}
	cf.FailNext("DELETE", "/certificates/", 502, 1100, "boom")
	if err := c.RevokeOriginCert(ctx, token, got.ID); !errors.Is(err, entity.ErrCloudflareUnavailable) {
		t.Fatalf("502: %v", err)
	}
	// Un mensaje de /certificates que trajera el token, también como ***.
	cf.FailNext("POST", "/certificates", 400, 1010, "Failed to validate requested hostname "+token)
	_, err = c.CreateOriginCert(ctx, token, "api.x.cl", csr, 90)
	code("token en el mensaje", err, 400, 1010)
}
