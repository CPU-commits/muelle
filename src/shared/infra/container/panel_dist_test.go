//go:build !panel

package container

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
)

// Sin -tags panel, el servidor de muelle responde la página de "no trae el
// panel" fuera de /api, y no_route en /api. Con el tag y un build en su
// lugar, GET / da 200 con el panel: por eso el archivo es solo de !panel.
func TestPanelNotEmbedded(t *testing.T) {
	c := newTestContainer(t)

	w := httptest.NewRecorder()
	c.Server.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	httptestutil.Expect(t, w, http.StatusNotFound)
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("GET /: Content-Type=%q", ct)
	}
	if !strings.Contains(w.Body.String(), "-tags panel") {
		t.Fatalf("GET /: body=%s", w.Body.String())
	}

	w = httptest.NewRecorder()
	c.Server.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/nada", nil))
	httptestutil.Expect(t, w, http.StatusNotFound)
	if code := httptestutil.ErrorCode(t, w); code != "no_route" {
		t.Fatalf("GET /api/v1/nada: code=%s", code)
	}
}
