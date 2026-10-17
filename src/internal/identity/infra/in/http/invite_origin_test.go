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

// Sin origen del panel (los ajustes no se pudieron leer: admin lo deja en el
// log) no hay link que sirva: POST /invites responde 500 internal en vez de
// un link relativo, y no crea la invitación ni la audita.
func TestInviteWithoutPanelOrigin(t *testing.T) {
	e := newEnvOrigin(t, func(context.Context) string { return "" }, nil)
	s := httptestutil.WithSession(e.login())
	w := e.do("POST", "/api/v1/invites", nil, s)
	httptestutil.Expect(t, w, http.StatusInternalServerError)
	if got := httptestutil.Decode[map[string]map[string]string](t, w)["error"]["code"]; got != "internal" {
		t.Fatalf("code=%q", got)
	}
	var invites, audits int
	conn := e.db.Conn(context.Background())
	if err := conn.QueryRowContext(context.Background(), `SELECT count(*) FROM invites`).Scan(&invites); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRowContext(context.Background(),
		`SELECT count(*) FROM audit_events WHERE action = 'invite.create'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if invites != 0 || audits != 0 {
		t.Fatalf("invitaciones %d, auditoría %d", invites, audits)
	}
}
