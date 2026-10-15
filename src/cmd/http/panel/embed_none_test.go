//go:build !panel

package panel

import (
	"net/http"
	"strings"
	"testing"
)

// Sin -tags panel, toda ruta del panel responde 404 con una página que dice
// por qué, con los mismos headers de seguridad.
func TestNotEmbedded(t *testing.T) {
	h, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ method, path string }{{"GET", "/x"}, {"POST", "/"}, {"HEAD", "/"}, {"GET", "/_app/immutable/x.js"}} {
		w := serve(h, c.method, c.path)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s %s: %d", c.method, c.path, w.Code)
		}
		want := map[string]string{
			"Content-Type":            "text/html; charset=utf-8",
			"Content-Security-Policy": "default-src 'none'",
			"X-Content-Type-Options":  "nosniff",
			"Referrer-Policy":         "same-origin",
			"Cache-Control":           "no-cache",
		}
		for k, v := range want {
			if got := w.Header().Get(k); got != v {
				t.Errorf("%s %s: %s=%q, esperaba %q", c.method, c.path, k, got, v)
			}
		}
		if c.method == "HEAD" {
			if w.Body.Len() != 0 {
				t.Errorf("HEAD %s: %d bytes de cuerpo", c.path, w.Body.Len())
			}
			continue
		}
		if body := w.Body.String(); !strings.Contains(body, "-tags panel") || !strings.Contains(body, "/api/v1") {
			t.Errorf("%s %s: body=%s", c.method, c.path, body)
		}
	}
}
