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
