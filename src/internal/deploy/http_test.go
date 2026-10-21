package deploy

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

var t0 = time.Date(2026, 10, 10, 14, 19, 49, 120_000_000, time.UTC)

// httpTest es el módulo completo detrás del servidor real de cmd/http, con
// una base real, un reloj manual y un FakeAuth con una sesión ("s") y dos
// tokens: "tp" del proyecto auditfy y "tx" de otro proyecto.
type httpTest struct {
	m     *Module
	db    *sqlite.DB
	clock *clock.Manual
	f     *dockertest.Fake
	c     *httptestutil.Client
}

var testAuth = httptestutil.FakeAuth{
	Sessions: map[string]entity.Principal{"s": {UserID: 1, Username: "felipe"}},
	Tokens: map[string]entity.Principal{
		"tp": {Token: &entity.TokenScope{ID: 4, Name: "github-actions", ProjectSlug: "auditfy"}},
		"tx": {Token: &entity.TokenScope{ID: 5, Name: "otro", ProjectSlug: "otro"}},
	},
}

func newHTTPTest(t *testing.T) *httpTest {
	t.Helper()
	h := newModuleTest(t, fakeEnv{})
	h.c = httptestutil.New(t, testAuth, h.m.RegisterRoutes)
	return h
}

// newModuleTest arma el módulo sin registrar sus rutas (así un test puede
// cambiar sus límites de SSE antes), con env como las variables de las apps.
func newModuleTest(t *testing.T, env service.EnvResolver) *httpTest {
	t.Helper()
	db := sqlitetest.New(t)
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	clk := clock.NewManual(t0)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := dockertest.New()
	m := New(Deps{DB: db, Cipher: c, Clock: clk, Audit: sqlite.NewAuditRecorder(db, clk, log), Log: log,
		Docker: f, Env: env, Ingress: fakeIngress{}})
	return &httpTest{m: m, db: db, clock: clk, f: f}
}

func (h *httpTest) exec(t *testing.T, query string, args ...any) int64 {
	t.Helper()
	ctx := context.Background()
	res, err := h.db.Conn(ctx).ExecContext(ctx, query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func session() httptestutil.Opt { return httptestutil.WithSession("s") }

func expectError(t *testing.T, w *httptest.ResponseRecorder, status int, code, message string) {
	t.Helper()
	httptestutil.Expect(t, w, status)
	if got := httptestutil.ErrorCode(t, w); got != code {
		t.Fatalf("code %q, esperaba %q: %s", got, code, w.Body)
	}
	if message != "" {
		body := httptestutil.Decode[struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}](t, w)
		if !strings.HasPrefix(body.Error.Message, message) {
			t.Fatalf("message %q, esperaba que empiece con %q", body.Error.Message, message)
		}
	}
}

type registryJSON struct {
	Host      string     `json:"host"`
	Username  string     `json:"username"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

// El CRUD de registries (contrato §2.10): la contraseña nunca vuelve.
func TestRegistriesHTTP(t *testing.T) {
	h := newHTTPTest(t)
	const p1, p2, p3 = "s3cr3t-ghcr-pass", "s3cr3t-local-pass", "s3cr3t-nueva-pass"
	var bodies []string
	do := func(method, path string, body any) *httptest.ResponseRecorder {
		w := h.c.Do(method, path, body, session())
		bodies = append(bodies, w.Body.String())
		return w
	}

	w := do("GET", "/api/v1/registries", nil)
	httptestutil.Expect(t, w, http.StatusOK)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("sin registries: %s", w.Body)
	}

	w = do("POST", "/api/v1/registries", map[string]string{"host": "ghcr.io", "username": "bot", "password": p1})
	httptestutil.Expect(t, w, http.StatusCreated)
	got := httptestutil.Decode[map[string]any](t, w)
	if len(got) != 4 || got["host"] != "ghcr.io" || got["username"] != "bot" || got["created_at"] != "2026-10-10T14:19:49.12Z" || got["updated_at"] != "2026-10-10T14:19:49.12Z" {
		t.Fatalf("POST: %s", w.Body)
	}
	expectError(t, do("POST", "/api/v1/registries", map[string]string{"host": "ghcr.io", "username": "x", "password": "y-y-y-y-y"}),
		http.StatusConflict, "conflict", "")
	h.clock.Advance(time.Second)
	httptestutil.Expect(t, do("POST", "/api/v1/registries", map[string]string{"host": "registry.example.com:5000", "username": "ci", "password": p2}), http.StatusCreated)

	for _, c := range []struct {
		body    any
		status  int
		code    string
		message string
	}{
		{map[string]string{"host": "index.docker.io", "username": "bot", "password": p1}, 422, "invalid", "host: para Docker Hub usa docker.io"},
		{map[string]string{"host": "GHCR.io", "username": "bot", "password": p1}, 422, "invalid", "host: "},
		{map[string]string{"host": "quay.io", "username": "", "password": p1}, 422, "invalid", "username: "},
		{map[string]string{"host": "quay.io", "username": "bot"}, 422, "invalid", "password: "},
		{map[string]string{"host": "quay.io", "username": "bot", "password": p1, "x": "1"}, 400, "invalid_json", ""},
		{"[]", 400, "invalid_json", ""},
	} {
		expectError(t, do("POST", "/api/v1/registries", c.body), c.status, c.code, c.message)
	}

	w = do("GET", "/api/v1/registries", nil)
	httptestutil.Expect(t, w, http.StatusOK)
	list := httptestutil.Decode[[]registryJSON](t, w)
	if len(list) != 2 || list[0].Host != "ghcr.io" || list[1].Host != "registry.example.com:5000" || list[1].Username != "ci" {
		t.Fatalf("GET: %s", w.Body)
	}

	// El host va url-encoded en la ruta.
	h.clock.Advance(time.Hour)
	w = do("PATCH", "/api/v1/registries/registry.example.com%3A5000", map[string]string{"username": "ci2", "password": p3})
	httptestutil.Expect(t, w, http.StatusOK)
	reg := httptestutil.Decode[registryJSON](t, w)
	if reg.Host != "registry.example.com:5000" || reg.Username != "ci2" || !reg.CreatedAt.Equal(t0.Add(time.Second)) || !reg.UpdatedAt.Equal(t0.Add(time.Hour+time.Second)) {
		t.Fatalf("PATCH: %s", w.Body)
	}
	expectError(t, do("PATCH", "/api/v1/registries/quay.io", map[string]string{"username": "x", "password": p3}), 404, "not_found", "")
	expectError(t, do("PATCH", "/api/v1/registries/ghcr.io", map[string]string{"username": "x"}), 422, "invalid", "password: ")
	expectError(t, do("PATCH", "/api/v1/registries/ghcr.io", map[string]string{"host": "x", "username": "x", "password": p3}), 400, "invalid_json", "")

	httptestutil.Expect(t, do("DELETE", "/api/v1/registries/ghcr.io", nil), http.StatusNoContent)
	expectError(t, do("DELETE", "/api/v1/registries/ghcr.io", nil), 404, "not_found", "")
	if list := httptestutil.Decode[[]registryJSON](t, do("GET", "/api/v1/registries", nil)); len(list) != 1 {
		t.Fatalf("después del DELETE: %+v", list)
	}

	for _, b := range bodies {
		for _, p := range []string{p1, p2, p3, `"password"`} {
			if strings.Contains(b, p) {
				t.Fatalf("una respuesta trae %q: %s", p, b)
			}
		}
	}
}

// /registries es solo con sesión: un token recibe 403 token_not_allowed.
func TestRegistriesRejectTokens(t *testing.T) {
	h := newHTTPTest(t)
	for _, req := range []struct{ method, path string }{
		{"GET", "/api/v1/registries"}, {"POST", "/api/v1/registries"},
		{"PATCH", "/api/v1/registries/ghcr.io"}, {"DELETE", "/api/v1/registries/ghcr.io"},
	} {
		w := h.c.Do(req.method, req.path, map[string]string{}, httptestutil.WithBearer("tp"))
		expectError(t, w, http.StatusForbidden, "token_not_allowed", "")
	}
}
