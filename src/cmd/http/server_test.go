package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
)

const testOrigin = "https://panel.test"

// fakeAuth acepta la sesión "s" y el token "mk_t" (proyecto auditfy).
type fakeAuth struct{}

func (fakeAuth) AuthenticateSession(_ context.Context, token string) (entity.Principal, error) {
	if token == "s" {
		return entity.Principal{UserID: 1, Username: "ana"}, nil
	}
	return entity.Principal{}, entity.ErrUnauthenticated
}

func (fakeAuth) AuthenticateToken(_ context.Context, bearer string) (entity.Principal, error) {
	if bearer == "mk_t" {
		return entity.Principal{Token: &entity.TokenScope{ID: 1, ProjectSlug: "auditfy"}}, nil
	}
	return entity.Principal{}, entity.ErrUnauthenticated
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	s := NewServer(Config{
		PanelOrigin:   testOrigin,
		SecureCookies: true,
		Auth:          fakeAuth{},
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	k := s.Kit()
	ok := func(w http.ResponseWriter, r *http.Request) {
		utils.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
	k.HandlePublic("POST /api/v1/ping", ok)
	k.HandlePublic("GET /api/v1/ping", ok)
	k.HandlePublic("GET /fuera-de-api", ok)
	k.Handle("PUT /api/v1/projects/{p}/env", utils.AccessToken, ok)
	k.Handle("DELETE /api/v1/projects/{p}", utils.AccessSession, ok)

	common := map[string]error{
		"not_found":       repository.ErrNotFound,
		"conflict":        repository.ErrConflict,
		"not_empty":       repository.ErrNotEmpty,
		"unauthenticated": entity.ErrUnauthenticated,
		"forbidden":       entity.ErrForbidden,
	}
	k.HandlePublic("GET /api/v1/fail/{code}", func(w http.ResponseWriter, r *http.Request) {
		k.Fail(w, r, fmt.Errorf("envuelto: %w", common[r.PathValue("code")]))
	})
	return s
}

func do(s *Server, method, path, origin string, header ...string) *httptest.ResponseRecorder {
	var body io.Reader
	if method != http.MethodGet {
		body = strings.NewReader(`{}`)
	}
	r := httptest.NewRequest(method, path, body)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	for i := 0; i+1 < len(header); i += 2 {
		r.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func errorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var b struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatalf("respuesta no es un error JSON (%d): %s", w.Code, w.Body.String())
	}
	return b.Error.Code
}

func TestOriginCheck(t *testing.T) {
	s := newTestServer(t)

	w := do(s, "POST", "/api/v1/ping", "https://evil.example")
	if w.Code != http.StatusForbidden || errorCode(t, w) != "bad_origin" {
		t.Fatalf("Origin ajeno: %d %s", w.Code, w.Body.String())
	}
	w = do(s, "POST", "/api/v1/ping", "")
	if w.Code != http.StatusForbidden || errorCode(t, w) != "bad_origin" {
		t.Fatalf("sin Origin: %d %s", w.Code, w.Body.String())
	}
	// La comparación es literal.
	w = do(s, "POST", "/api/v1/ping", testOrigin+"/")
	if w.Code != http.StatusForbidden {
		t.Fatalf("Origin con barra: %d", w.Code)
	}
	w = do(s, "DELETE", "/api/v1/projects/auditfy", "https://evil.example", "Cookie", utils.SessionCookie+"=s")
	if w.Code != http.StatusForbidden || errorCode(t, w) != "bad_origin" {
		t.Fatalf("DELETE con sesión y Origin ajeno: %d %s", w.Code, w.Body.String())
	}
	// Los navegadores mandan "Origin: null" desde iframes sandbox, data: o file:.
	w = do(s, "POST", "/api/v1/ping", "null")
	if w.Code != http.StatusForbidden || errorCode(t, w) != "bad_origin" {
		t.Fatalf("Origin null: %d %s", w.Code, w.Body.String())
	}

	if w := do(s, "POST", "/api/v1/ping", testOrigin); w.Code != http.StatusOK {
		t.Fatalf("Origin del panel: %d %s", w.Code, w.Body.String())
	}
	if w := do(s, "DELETE", "/api/v1/projects/auditfy", testOrigin, "Cookie", utils.SessionCookie+"=s"); w.Code != http.StatusOK {
		t.Fatalf("DELETE con sesión y Origin del panel: %d %s", w.Code, w.Body.String())
	}
	if w := do(s, "PUT", "/api/v1/projects/auditfy/env", "", "Authorization", "Bearer mk_t"); w.Code != http.StatusOK {
		t.Fatalf("Bearer sin Origin: %d %s", w.Code, w.Body.String())
	}
	// El esquema no distingue mayúsculas (RFC 9110 §11.1).
	if w := do(s, "PUT", "/api/v1/projects/auditfy/env", "", "Authorization", "bearer mk_t"); w.Code != http.StatusOK {
		t.Fatalf("bearer en minúsculas sin Origin: %d %s", w.Code, w.Body.String())
	}
	if w := do(s, "GET", "/api/v1/ping", ""); w.Code != http.StatusOK {
		t.Fatalf("GET sin Origin: %d %s", w.Code, w.Body.String())
	}
	if w := do(s, "GET", "/api/v1/ping", "https://evil.example"); w.Code != http.StatusOK {
		t.Fatalf("GET con Origin ajeno: %d %s", w.Code, w.Body.String())
	}
}

func TestSecurityHeaders(t *testing.T) {
	s := newTestServer(t)
	w := do(s, "GET", "/api/v1/ping", "")
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers en /api/: %v", w.Header())
	}
	// También en las respuestas de error del propio servidor.
	w = do(s, "POST", "/api/v1/ping", "https://evil.example")
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers en bad_origin: %v", w.Header())
	}
	w = do(s, "GET", "/fuera-de-api", "")
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "" {
		t.Fatalf("headers fuera de /api/: %v", w.Header())
	}
}

func TestCommonErrors(t *testing.T) {
	s := newTestServer(t)
	want := map[string]int{
		"not_found":       http.StatusNotFound,
		"conflict":        http.StatusConflict,
		"not_empty":       http.StatusConflict,
		"unauthenticated": http.StatusUnauthorized,
		"forbidden":       http.StatusForbidden,
	}
	for code, status := range want {
		w := do(s, "GET", "/api/v1/fail/"+code, "")
		if w.Code != status || errorCode(t, w) != code {
			t.Errorf("%s: %d %s, esperaba %d", code, w.Code, w.Body.String(), status)
		}
	}
	// Un Authenticator que devuelve ErrUnauthenticated responde 401 sin registrar nada más.
	w := do(s, "DELETE", "/api/v1/projects/auditfy", testOrigin, "Cookie", utils.SessionCookie+"=vencida")
	if w.Code != http.StatusUnauthorized || errorCode(t, w) != "unauthenticated" {
		t.Fatalf("sesión vencida: %d %s", w.Code, w.Body.String())
	}
}

// Un método que la ruta no acepta responde 405 en JSON, con el header Allow del mux.
func TestMethodNotAllowedIsJSON(t *testing.T) {
	s := newTestServer(t)
	w := do(s, "GET", "/api/v1/projects/auditfy/env", "")
	if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("%d %v %s", w.Code, w.Header(), w.Body.String())
	}
	if got := strings.TrimSpace(w.Body.String()); got != `{"error":{"code":"method_not_allowed","message":"método no permitido"}}` {
		t.Fatalf("body=%s", got)
	}
	if allow := w.Header().Get("Allow"); allow != "PUT" {
		t.Fatalf("Allow=%q, esperaba PUT", allow)
	}
	// Los headers de seguridad siguen puestos.
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("headers: %v", w.Header())
	}
}

// Una ruta que no existe responde el mismo JSON de error que el resto de la API.
func TestUnmatchedRouteIsJSON(t *testing.T) {
	s := newTestServer(t)
	for _, path := range []string{"/api/v1/no-existe", "/api/v1/projects", "/otra-cosa"} {
		w := do(s, "GET", path, "")
		if w.Code != http.StatusNotFound || w.Header().Get("Content-Type") != "application/json" {
			t.Errorf("%s: %d %v %s", path, w.Code, w.Header(), w.Body.String())
			continue
		}
		if got := strings.TrimSpace(w.Body.String()); got != `{"error":{"code":"not_found","message":"no existe"}}` {
			t.Errorf("%s: body=%s", path, got)
		}
	}
	// Las rutas registradas siguen respondiendo.
	if w := do(s, "GET", "/api/v1/ping", ""); w.Code != http.StatusOK {
		t.Fatalf("ruta registrada: %d %s", w.Code, w.Body.String())
	}
}

func TestNewServerRequiresConfig(t *testing.T) {
	cases := map[string]Config{
		"Auth":        {PanelOrigin: testOrigin},
		"PanelOrigin": {Auth: fakeAuth{}},
	}
	for field, c := range cases {
		func() {
			defer func() {
				r := recover()
				if msg, _ := r.(string); !strings.Contains(msg, field) {
					t.Errorf("sin %s: recover() = %v, esperaba un panic que lo nombre", field, r)
				}
			}()
			NewServer(c)
		}()
	}
}
