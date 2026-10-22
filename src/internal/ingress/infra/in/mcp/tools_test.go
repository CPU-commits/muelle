package mcp_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/CPU-commits/muelle/src/cmd/mcp/tools"
	"github.com/CPU-commits/muelle/src/cmd/mcp/tools/toolstest"
	"github.com/CPU-commits/muelle/src/internal/ingress"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/caddy/caddytest"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare/cftest"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

// env es ingress armado sobre SQLite real, con Docker, Caddy y Cloudflare
// falsos (list_domains no llega a ninguno), y sus herramientas en un
// tools.Kit.
type env struct {
	t   *testing.T
	db  *sqlite.DB
	kit *tools.Kit
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := sqlitetest.New(t)
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.DiscardHandler)
	m := ingress.New(ingress.Deps{
		DB: db, Cipher: c, Audit: sqlite.NewAuditRecorder(db, clock.System{}, log), Log: log, Docker: dockertest.New(),
		Version: "v0.1.0", Options: ingress.Options{CloudflareURL: cftest.New(t).URL(), CaddyAdminDial: caddytest.New(t).Dial},
	})
	k := toolstest.NewKit(t, m.RegisterRoutes)
	m.RegisterTools(k)
	return &env{t: t, db: db, kit: k}
}

// domain guarda un dominio de la app como lo deja ingress, con los campos
// que importan para list_domains.
func (e *env) domain(app int64, host, status, errCode, lastError string, certExpires *time.Time) {
	e.t.Helper()
	null := func(s string) any {
		if s == "" {
			return nil
		}
		return s
	}
	var expires any
	if certExpires != nil {
		expires = certExpires.UnixMilli()
	}
	var issuer any
	if certExpires != nil {
		issuer = "Let's Encrypt"
	}
	ctx := context.Background()
	if _, err := e.db.Conn(ctx).ExecContext(ctx, `INSERT INTO domains (app_id, hostname, target_service, target_port, proxied,
		tls_mode, cf_zone_id, cf_record_id, status, last_error_code, last_error, cert_issuer, cert_expires_at, created_at, updated_at)
		VALUES (?, ?, 'api', 3000, 1, 'cloudflare', 'z1', 'r-'||?, ?, ?, ?, ?, ?, 0, 0)`,
		app, host, host, status, null(errCode), null(lastError), issuer, expires); err != nil {
		e.t.Fatal(err)
	}
}

func TestListDomains(t *testing.T) {
	e := newEnv(t)
	api := ingresstest.App(t, e.db, "p1", "s1", "api", "service")
	ingresstest.App(t, e.db, "p1", "s1", "vacia", "service")
	exp := time.Date(2027, 1, 5, 10, 0, 0, 0, time.UTC)
	long := strings.Repeat("é", 400) // 400 caracteres de 2 bytes
	e.domain(api, "www.p1.cl", "error", "cloudflare", long, nil)
	e.domain(api, "api.p1.cl", "active", "", "", &exp)

	s := toolstest.Connect(t, e.kit, toolstest.Token(1, "p1", "s1", true))
	text, isErr := s.Call("list_domains", map[string]any{"project": "p1", "stage": "s1", "app": "api"})
	cut := strings.Repeat("é", 299) + tools.Ellipsis
	want := `{"domains":[` +
		`{"hostname":"api.p1.cl","target_service":"api","target_port":3000,"status":"active","last_error_code":null,` +
		`"last_error":null,"proxied":true,"tls_mode":"cloudflare","cert_issuer":"Let's Encrypt","cert_expires_at":"2027-01-05T10:00:00Z"},` +
		`{"hostname":"www.p1.cl","target_service":"api","target_port":3000,"status":"error","last_error_code":"cloudflare",` +
		`"last_error":"` + cut + `","proxied":true,"tls_mode":"cloudflare","cert_issuer":null,"cert_expires_at":null}],` +
		`"omitted":0,"warning":"` + tools.UntrustedWarning + `"}`
	if isErr || text != want {
		t.Fatalf("list_domains:\n%s\nesperaba\n%s", text, want)
	}
	if n := utf8.RuneCountInString(cut); n != 300 {
		t.Fatalf("el corte mide %d caracteres", n)
	}
	// Los ids de Cloudflare y los registros son de REST: acá no salen.
	if strings.Contains(text, "cf_") || strings.Contains(text, "records") {
		t.Errorf("list_domains muestra campos de más: %s", text)
	}

	if text := toolstest.OK[map[string]any](s, "list_domains", map[string]any{"project": "p1", "stage": "s1", "app": "vacia"}); fmt.Sprint(text["domains"]) != "[]" || text["omitted"] != 0.0 {
		t.Errorf("una app sin dominios: %v", text)
	}
	if code, _ := toolstest.Fail(s, "list_domains", map[string]any{"project": "p1", "stage": "s1", "app": "nada"}); code != "not_found" {
		t.Errorf("una app que no existe: %s", code)
	}
	if code, _ := toolstest.Fail(s, "list_domains", map[string]any{"project": "p1", "stage": "s2", "app": "api"}); code != "out_of_scope" {
		t.Errorf("otro stage: %s", code)
	}
}

// Peor caso de list_domains: dominios de sobra, con hostnames largos y un
// last_error lleno de ESC y comillas, para pasar el tope: se cortan al final
// con omitted, en orden de hostname.
func TestListDomainsWorstCase(t *testing.T) {
	e := newEnv(t)
	api := ingresstest.App(t, e.db, "p1", "s1", "api", "service")
	const n = 120
	errText := strings.Repeat("\x1b[31m\"x\\", 100)
	for i := range n {
		host := fmt.Sprintf("d%03d.%s.p1.cl", i, strings.Repeat("a", 200))
		e.domain(api, host, "error", "certificate", errText, nil)
	}
	s := toolstest.Connect(t, e.kit, toolstest.AllProjects(1))
	text, isErr := s.Call("list_domains", map[string]any{"project": "p1", "stage": "s1", "app": "api"})
	out := toolstest.Decode[struct {
		Domains []map[string]any `json:"domains"`
		Omitted int              `json:"omitted"`
	}](t, text)
	if isErr || len(text) > tools.MaxResultBytes || out.Omitted == 0 || len(out.Domains)+out.Omitted != n ||
		!strings.HasPrefix(fmt.Sprint(out.Domains[0]["hostname"]), "d000.") {
		t.Fatalf("isErr=%v %d bytes, %d dominios, omitted %d", isErr, len(text), len(out.Domains), out.Omitted)
	}
	if len(text) < tools.MaxResultBytes-2500 {
		t.Errorf("solo %d bytes: debió llenar el tope", len(text))
	}
}
