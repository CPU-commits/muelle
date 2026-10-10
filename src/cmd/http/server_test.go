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
		k.Fail(w, fmt.Errorf("envuelto: %w", common[r.PathValue("code")]))
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

	if w := do(s, "POST", "/api/v1/ping", testOrigin); w.Code != http.StatusOK {
		t.Fatalf("Origin del panel: %d %s", w.Code, w.Body.String())
	}
	if w := do(s, "DELETE", "/api/v1/projects/auditfy", testOrigin, "Cookie", utils.SessionCookie+"=s"); w.Code != http.StatusOK {
		t.Fatalf("DELETE con sesión y Origin del panel: %d %s", w.Code, w.Body.String())
	}
	if w := do(s, "PUT", "/api/v1/projects/auditfy/env", "", "Authorization", "Bearer mk_t"); w.Code != http.StatusOK {
		t.Fatalf("Bearer sin Origin: %d %s", w.Code, w.Body.String())
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
