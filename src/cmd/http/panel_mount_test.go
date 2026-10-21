package http

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
)

// panelStub responde "panel" y guarda la ruta de cada petición que recibe.
type panelStub struct{ paths []string }

func (p *panelStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.paths = append(p.paths, r.URL.Path)
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "panel")
}

func newPanelServer(t *testing.T, panel http.Handler) *Server {
	t.Helper()
	s := NewServer(Config{
		PanelOrigin: testOrigin,
		Auth:        fakeAuth{},
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Panel:       panel,
	})
	ok := func(w http.ResponseWriter, r *http.Request) {
		utils.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
	s.Kit().HandlePublic("GET /fuera-de-api", ok)
	s.Kit().HandlePublic("GET /api/v1/ping", ok)
	return s
}

// El panel atiende toda ruta fuera de /api sin una ruta registrada; las de
// /api sin ruta siguen siendo no_route, y el Origin se revisa antes.
func TestPanelMount(t *testing.T) {
	stub := &panelStub{}
	s := newPanelServer(t, stub)

	for _, p := range []string{"/x/y", "/", "/apix", "/API/v1", "/_app/immutable/a.js"} {
		stub.paths = nil
		w := do(s, "GET", p, "")
		if w.Code != http.StatusOK || w.Body.String() != "panel" {
			t.Errorf("GET %s: %d %s, esperaba el panel", p, w.Code, w.Body.String())
			continue
		}
		if len(stub.paths) != 1 || stub.paths[0] != p {
			t.Errorf("GET %s: el panel vio %v", p, stub.paths)
		}
		// no-store es solo de /api/: la caché del panel la decide el panel.
		if cc := w.Header().Get("Cache-Control"); cc != "" {
			t.Errorf("GET %s: Cache-Control=%q", p, cc)
		}
	}

	for _, p := range []string{"/api", "/api/", "/api/v1/nada", "/api/v2", "/x/../api/v1/nada", "//api/v1/nada"} {
		stub.paths = nil
		w := do(s, "GET", p, "")
		if w.Code != http.StatusNotFound || errorCode(t, w) != "no_route" {
			t.Errorf("GET %s: %d %s, esperaba 404 no_route", p, w.Code, w.Body.String())
		}
		if len(stub.paths) != 0 {
			t.Errorf("GET %s: el panel vio %v", p, stub.paths)
		}
	}

	stub.paths = nil
	if w := do(s, "GET", "/fuera-de-api", ""); w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"ok":true}` {
		t.Errorf("GET /fuera-de-api: %d %s, esperaba la ruta registrada", w.Code, w.Body.String())
	}
	w := do(s, "POST", "/fuera-de-api", testOrigin)
	if w.Code != http.StatusMethodNotAllowed || errorCode(t, w) != "method_not_allowed" {
		t.Errorf("POST /fuera-de-api: %d %s, esperaba el 405 del mux", w.Code, w.Body.String())
	}
	if allow := w.Header().Get("Allow"); !strings.Contains(allow, "GET") {
		t.Errorf("POST /fuera-de-api: Allow=%q", allow)
	}
	if len(stub.paths) != 0 {
		t.Errorf("las rutas registradas llegaron al panel: %v", stub.paths)
	}

	w = do(s, "POST", "/x", "")
	if w.Code != http.StatusForbidden || errorCode(t, w) != "bad_origin" {
		t.Errorf("POST /x sin Origin: %d %s, esperaba 403 bad_origin", w.Code, w.Body.String())
	}
	if len(stub.paths) != 0 {
		t.Errorf("POST /x sin Origin llegó al panel: %v", stub.paths)
	}
	if w := do(s, "POST", "/x", testOrigin); w.Code != http.StatusOK || w.Body.String() != "panel" {
		t.Errorf("POST /x con el Origin del panel: %d %s, esperaba el panel", w.Code, w.Body.String())
	}
}

// Sin Panel, una ruta fuera de /api sigue siendo 404 no_route.
func TestPanelNilKeepsNoRoute(t *testing.T) {
	s := newPanelServer(t, nil)
	for _, p := range []string{"/otra-cosa", "/"} {
		if w := do(s, "GET", p, ""); w.Code != http.StatusNotFound || errorCode(t, w) != "no_route" {
			t.Errorf("GET %s: %d %s, esperaba 404 no_route", p, w.Code, w.Body.String())
		}
	}
}

func TestIsAPIPath(t *testing.T) {
	cases := map[string]bool{
		"/api": true, "/api/": true, "/api/v1": true, "/api/v1/x": true, "/x/../api/v1": true, "//api": true, "/api/../api": true,
		"/": false, "/apix": false, "/API": false, "/x/api": false, "/api/..": false, "/_app/api": false,
	}
	for p, want := range cases {
		if got := isAPIPath(p); got != want {
			t.Errorf("isAPIPath(%q)=%v, esperaba %v", p, got, want)
		}
	}
}
