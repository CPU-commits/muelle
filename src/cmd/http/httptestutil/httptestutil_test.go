package httptestutil

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

func newPingClient(t *testing.T) *Client {
	auth := FakeAuth{
		Sessions: map[string]entity.Principal{"s": {UserID: 1, Username: "ana"}},
		Tokens:   map[string]entity.Principal{"mk_t": {Token: &entity.TokenScope{ID: 3, ProjectSlug: "auditfy"}}},
	}
	return New(t, auth, func(k *utils.Kit) {
		k.Handle("GET /api/v1/ping", utils.AccessSession, func(w http.ResponseWriter, r *http.Request) {
			p, _ := entity.PrincipalFrom(r.Context())
			utils.WriteJSON(w, http.StatusOK, map[string]string{"user": p.Username})
		})
		k.Handle("PUT /api/v1/projects/{p}/env", utils.AccessToken, func(w http.ResponseWriter, r *http.Request) {
			var in struct {
				Vars map[string]string `json:"vars"`
			}
			if !utils.DecodeJSON(w, r, &in) {
				return
			}
			utils.WriteJSON(w, http.StatusOK, in)
		})
		k.HandlePublic("POST /api/v1/login", func(w http.ResponseWriter, r *http.Request) {
			k.SetSessionCookie(w, "nueva", time.Now().Add(time.Hour))
			w.WriteHeader(http.StatusNoContent)
		})
	})
}

func TestPing(t *testing.T) {
	c := newPingClient(t)

	w := c.Do("GET", "/api/v1/ping", nil)
	Expect(t, w, http.StatusUnauthorized)
	if code := ErrorCode(t, w); code != "unauthenticated" {
		t.Fatalf("code=%s", code)
	}

	// Una sesión que FakeAuth no conoce da ErrUnauthenticated, que el servidor traduce a 401.
	w = c.Do("GET", "/api/v1/ping", nil, WithSession("otra"))
	Expect(t, w, http.StatusUnauthorized)
	if code := ErrorCode(t, w); code != "unauthenticated" {
		t.Fatalf("code=%s", code)
	}

	w = c.Do("GET", "/api/v1/ping", nil, WithSession("s"))
	Expect(t, w, http.StatusOK)
	if got := Decode[map[string]string](t, w); got["user"] != "ana" {
		t.Fatalf("got=%v", got)
	}
}

func TestDoBodyOriginAndBearer(t *testing.T) {
	c := newPingClient(t)
	body := map[string]any{"vars": map[string]string{"A": "1"}}

	// Con Bearer pasa aunque no lleve Origin.
	w := c.Do("PUT", "/api/v1/projects/auditfy/env", body, WithBearer("mk_t"), WithOrigin(""))
	Expect(t, w, http.StatusOK)
	if got := Decode[map[string]map[string]string](t, w); got["vars"]["A"] != "1" {
		t.Fatalf("got=%v", got)
	}

	// Un string se manda tal cual, para probar JSON inválido.
	w = c.Do("PUT", "/api/v1/projects/auditfy/env", `{"vars":{},"extra":1}`, WithBearer("mk_t"))
	Expect(t, w, http.StatusBadRequest)
	if code := ErrorCode(t, w); code != "invalid_json" {
		t.Fatalf("code=%s", code)
	}

	w = c.Do("PUT", "/api/v1/projects/otro/env", body, WithBearer("mk_t"))
	Expect(t, w, http.StatusForbidden)
	if code := ErrorCode(t, w); code != "out_of_scope" {
		t.Fatalf("code=%s", code)
	}

	// Sin Bearer, el Origin por defecto es el del panel; otro Origin o ninguno da 403.
	w = c.Do("POST", "/api/v1/login", nil)
	Expect(t, w, http.StatusNoContent)
	for _, origin := range []string{"", "https://evil.example"} {
		w = c.Do("POST", "/api/v1/login", nil, WithOrigin(origin))
		Expect(t, w, http.StatusForbidden)
		if code := ErrorCode(t, w); code != "bad_origin" {
			t.Fatalf("origin %q: code=%s", origin, code)
		}
	}
}

func TestSessionFrom(t *testing.T) {
	c := newPingClient(t)
	w := c.Do("POST", "/api/v1/login", nil)
	if got := SessionFrom(t, w); got != "nueva" {
		t.Fatalf("SessionFrom=%q", got)
	}
	// El servidor de prueba usa cookies seguras.
	if ck := w.Result().Cookies()[0]; !ck.Secure {
		t.Fatalf("cookie=%+v", ck)
	}
}

// recorder es un testing.TB que anota los Fatalf en vez de cortar el test.
type recorder struct {
	testing.TB
	fatals []string
}

func (r *recorder) Fatalf(format string, args ...any) {
	r.fatals = append(r.fatals, fmt.Sprintf(format, args...))
}

func TestHelpersReportFailures(t *testing.T) {
	w := httptest.NewRecorder()
	w.WriteHeader(http.StatusTeapot)
	w.WriteString("no es JSON")

	r := &recorder{TB: t}
	Expect(r, w, http.StatusOK)
	ErrorCode(r, w)
	Decode[map[string]any](r, w)
	SessionFrom(r, w)
	if len(r.fatals) != 4 {
		t.Fatalf("esperaba 4 fallos, hubo %d: %q", len(r.fatals), r.fatals)
	}
	if want := "no es JSON"; !strings.Contains(r.fatals[0], want) {
		t.Fatalf("Expect debe mostrar el body: %q", r.fatals[0])
	}
}
