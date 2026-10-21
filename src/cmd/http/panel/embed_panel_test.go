//go:build panel

package panel

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

// Con -tags panel, el handler sirve el build que se copió en build/ (en CI,
// uno de prueba de dos archivos) con la CSP de su csp-header.txt.
func TestEmbedded(t *testing.T) {
	h, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("build/csp-header.txt")
	if err != nil {
		t.Fatal(err)
	}
	shell, err := os.ReadFile("build/200.html")
	if err != nil {
		t.Fatal(err)
	}
	w := serve(h, "GET", "/")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /: %d %s", w.Code, w.Body.String())
	}
	if got, want := w.Header().Get("Content-Security-Policy"), strings.TrimSpace(string(raw)); got != want {
		t.Fatalf("Content-Security-Policy=%q, esperaba %q", got, want)
	}
	if w.Body.String() != string(shell) {
		t.Fatalf("GET /: no es el 200.html del build: %q", w.Body.String())
	}
}
