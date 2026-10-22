package http

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
)

// Con MUELLE_PANEL_DOMAIN y sin MUELLE_PANEL_ORIGIN (lo que escribe install),
// el único origen permitido es el de los ajustes, en cada petición: después
// de cambiar el puerto HTTPS del proxy pasa el nuevo y ya no el viejo.
func TestOriginCheckOnlyProxyOrigin(t *testing.T) {
	var current atomic.Value
	current.Store("https://panel.test:8443")
	s := NewServer(Config{
		Auth:        fakeAuth{},
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		ProxyOrigin: func(context.Context) string { return current.Load().(string) },
	})
	s.Kit().HandlePublic("POST /api/v1/ping", func(w http.ResponseWriter, r *http.Request) {
		utils.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	check := func(origin string, want int) {
		t.Helper()
		w := do(s, "POST", "/api/v1/ping", origin)
		if w.Code != want || (want == http.StatusForbidden && errorCode(t, w) != "bad_origin") {
			t.Fatalf("Origin %q: %d %s, esperaba %d", origin, w.Code, w.Body.String(), want)
		}
	}
	check("https://panel.test:8443", http.StatusOK)
	for _, o := range []string{"", "null", "https://panel.test", "https://panel.test:9443"} {
		check(o, http.StatusForbidden)
	}

	current.Store("https://panel.test")
	check("https://panel.test", http.StatusOK)
	check("https://panel.test:8443", http.StatusForbidden)

	// Sin dominio del panel (ProxyOrigin da "") no pasa ningún Origin.
	current.Store("")
	for _, o := range []string{"", "null", "https://panel.test"} {
		check(o, http.StatusForbidden)
	}
}

// NewServer acepta Config sin PanelOrigin si tiene ProxyOrigin.
func TestNewServerWithoutPanelOrigin(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("sin PanelOrigin y con ProxyOrigin: panic %v", r)
		}
	}()
	NewServer(Config{Auth: fakeAuth{}, ProxyOrigin: func(context.Context) string { return "" }})
}

// En /api/v1/mcp un Origin que viene tiene que ser uno de los permitidos,
// aunque la petición traiga Bearer (el spec MCP lo pide contra el DNS
// rebinding); sin Origin pasa, porque Claude Code no lo manda (contrato
// §0.1 y §8.2). El resto de /api no cambia: un Bearer no necesita Origin.
func TestOriginCheckMCP(t *testing.T) {
	var current atomic.Value
	current.Store("https://panel.example:8443")
	s := NewServer(Config{
		PanelOrigin: testOrigin,
		Auth:        fakeAuth{},
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		ProxyOrigin: func(context.Context) string { return current.Load().(string) },
	})
	ok := func(w http.ResponseWriter, r *http.Request) {
		utils.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
	s.Kit().Handle("POST "+utils.MCPPath, utils.AccessBearer, ok)
	s.Kit().Handle("PUT /api/v1/projects/{p}/env", utils.AccessToken, ok)

	auth := []string{"Authorization", "Bearer mk_t"}
	cases := []struct {
		name, method, path, origin string
		want                       int
	}{
		{"mcp sin Origin", "POST", utils.MCPPath, "", http.StatusOK},
		{"mcp con el origen de MUELLE_PANEL_ORIGIN", "POST", utils.MCPPath, testOrigin, http.StatusOK},
		{"mcp con el origen de los ajustes", "POST", utils.MCPPath, "https://panel.example:8443", http.StatusOK},
		{"mcp con otro origen", "POST", utils.MCPPath, "https://evil.example", http.StatusForbidden},
		{"mcp con Origin null", "POST", utils.MCPPath, "null", http.StatusForbidden},
		{"mcp con el puerto viejo", "POST", utils.MCPPath, "https://panel.example", http.StatusForbidden},
		{"mcp con GET y otro origen", "GET", utils.MCPPath, "https://evil.example", http.StatusForbidden},
		{"otra ruta con Bearer y otro origen", "PUT", "/api/v1/projects/auditfy/env", "https://evil.example", http.StatusOK},
		{"otra ruta con Bearer sin Origin", "PUT", "/api/v1/projects/auditfy/env", "", http.StatusOK},
	}
	for _, c := range cases {
		w := do(s, c.method, c.path, c.origin, auth...)
		if w.Code != c.want || (c.want == http.StatusForbidden && errorCode(t, w) != "bad_origin") {
			t.Errorf("%s: %d %s, esperaba %d", c.name, w.Code, w.Body.String(), c.want)
		}
	}
	// Sin Bearer, el 401 de AccessBearer; con otro Origin, antes el 403.
	if w := do(s, "POST", utils.MCPPath, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("mcp sin Bearer: %d %s", w.Code, w.Body.String())
	}
	if w := do(s, "POST", utils.MCPPath, "https://evil.example"); w.Code != http.StatusForbidden || errorCode(t, w) != "bad_origin" {
		t.Errorf("mcp sin Bearer y otro origen: %d %s", w.Code, w.Body.String())
	}
	// Sigue al puerto de los ajustes, como el resto.
	current.Store("https://panel.example")
	if w := do(s, "POST", utils.MCPPath, "https://panel.example", auth...); w.Code != http.StatusOK {
		t.Errorf("mcp con el puerto nuevo: %d %s", w.Code, w.Body.String())
	}
}
