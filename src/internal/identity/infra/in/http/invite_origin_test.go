package http_test

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
)

// El origen de los links se pregunta en cada invitación: sigue al puerto
// HTTPS de los ajustes (el contenedor pasa MUELLE_PANEL_ORIGIN si está o, si
// no, el origen de admin).
func TestInviteURLFollowsPanelOrigin(t *testing.T) {
	var origin atomic.Value
	origin.Store("https://panel.test:8443")
	e := newEnvOrigin(t, func(context.Context) string { return origin.Load().(string) }, nil)
	s := httptestutil.WithSession(e.login())
	for _, want := range []string{"https://panel.test:8443", "https://panel.test"} {
		origin.Store(want)
		w := e.do("POST", "/api/v1/invites", nil, s)
		httptestutil.Expect(t, w, http.StatusCreated)
		if url := httptestutil.Decode[map[string]any](t, w)["url"].(string); !strings.HasPrefix(url, want+"/invite/") {
			t.Fatalf("con el origen %s: url=%s", want, url)
		}
	}
}
