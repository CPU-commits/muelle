package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// bearerAuth es fakeAuth con el error de identity para un token que no
// existe, se revocó o venció (entity.ErrUnauthenticated), y un error de la
// base para "mk_db".
type bearerAuth struct{ fakeAuth }

var errDB = errors.New("la base no responde")

func (a bearerAuth) AuthenticateToken(ctx context.Context, bearer string) (entity.Principal, error) {
	switch bearer {
	case "mk_db":
		return entity.Principal{}, errDB
	case "mk_broken":
		return entity.Principal{UserID: 1, Username: "ana"}, nil
	}
	if p, ok := a.tokens[bearer]; ok {
		return p, nil
	}
	return entity.Principal{}, entity.ErrUnauthenticated
}

func newBearerKit() (*Kit, *http.ServeMux) {
	auth := bearerAuth{fakeAuth{
		sessions: map[string]entity.Principal{"full": {UserID: 1, Username: "ana", SessionHash: "h-full"}},
		tokens:   readOnlyTokens,
	}}
	mux := http.NewServeMux()
	k := NewKit(mux, auth, true, discard())
	echo := func(w http.ResponseWriter, r *http.Request) {
		p, ok := entity.PrincipalFrom(r.Context())
		if !ok || p.Token == nil {
			WriteJSON(w, http.StatusOK, map[string]any{"token": 0})
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"token": p.Token.ID, "read_only": p.Token.ReadOnly})
	}
	k.Handle("POST /api/v1/mcp", AccessBearer, echo)
	k.Handle("GET /api/v1/projects/{p}", AccessToken, echo)
	k.Handle("GET /api/v1/projects/{p}/stages/{s}/env", AccessToken, echo)
	k.Handle("HEAD /api/v1/projects/{p}/stages/{s}/env", AccessToken, echo)
	k.Handle("PUT /api/v1/projects/{p}/stages/{s}/env", AccessToken, echo)
	k.Handle("DELETE /api/v1/projects/{p}", AccessToken, echo)
	return k, mux
}

// AccessBearer (contrato §8.2): solo un token, sin la cookie; los dos 401
// llevan WWW-Authenticate (RFC 6750) y el Principal queda en el ctx. No
// revisa el alcance: lo revisa cada herramienta.
func TestAccessBearer(t *testing.T) {
	_, mux := newBearerKit()
	cases := []struct {
		name, msg, wwwAuth string
		opts               []reqOpt
		status             int
		code               string
		token              float64
	}{
		{name: "sin nada", status: 401, code: "unauthenticated",
			msg: "usa un token de API: Authorization: Bearer mk_…", wwwAuth: `Bearer realm="muelle"`},
		{name: "solo la cookie", opts: []reqOpt{cookie("full")}, status: 401, code: "unauthenticated",
			msg: "usa un token de API: Authorization: Bearer mk_…", wwwAuth: `Bearer realm="muelle"`},
		{name: "token que no existe", opts: []reqOpt{bearer("mk_nope")}, status: 401, code: "unauthenticated",
			msg: "el token no existe, se revocó o venció", wwwAuth: `Bearer realm="muelle", error="invalid_token"`},
		{name: "la base no responde", opts: []reqOpt{bearer("mk_db")}, status: 500, code: "internal"},
		{name: "token sin TokenScope", opts: []reqOpt{bearer("mk_broken")}, status: 500, code: "internal"},
		{name: "token de escritura", opts: []reqOpt{bearer("mk_project")}, status: 200, token: 7},
		{name: "token de solo lectura", opts: []reqOpt{bearer("mk_ro")}, status: 200, token: 10},
		{name: "token de todos los proyectos", opts: []reqOpt{bearer("mk_all")}, status: 200, token: 12},
		{name: "token y cookie", opts: []reqOpt{bearer("mk_stage"), cookie("full")}, status: 200, token: 8},
	}
	for _, c := range cases {
		w := serve(mux, "POST", "/api/v1/mcp", c.opts...)
		if w.Code != c.status {
			t.Errorf("%s: status %d, esperaba %d: %s", c.name, w.Code, c.status, w.Body.String())
			continue
		}
		if got := w.Header().Get("WWW-Authenticate"); got != c.wwwAuth {
			t.Errorf("%s: WWW-Authenticate %q, esperaba %q", c.name, got, c.wwwAuth)
		}
		if c.code != "" {
			b := decodeError(t, w)
			if b.Error.Code != c.code || (c.msg != "" && b.Error.Message != c.msg) {
				t.Errorf("%s: error %+v, esperaba %s %q", c.name, b.Error, c.code, c.msg)
			}
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil || m["token"] != c.token {
			t.Errorf("%s: %v %v, esperaba el token %v en el ctx", c.name, m, err, c.token)
		}
	}
}

// AccessBearer no revisa el alcance con la ruta, así que no puede ir debajo
// de un proyecto: ahí un token pasaría sin Allows.
func TestAccessBearerPatternGuard(t *testing.T) {
	register := func(pattern string) (msg string) {
		defer func() {
			if r := recover(); r != nil {
				msg = fmt.Sprint(r)
			}
		}()
		NewKit(http.NewServeMux(), fakeAuth{}, true, discard()).Handle(pattern, AccessBearer, func(http.ResponseWriter, *http.Request) {})
		return ""
	}
	for _, pattern := range []string{
		"GET /api/v1/projects/{p}",
		"POST /api/v1/projects/{p}/stages/{s}/apps/{a}/deploys",
		"POST /api/v1/projects",
		"POST /api/v1/pr%6Fjects/{p}",
		"POST /api/v1/mcp/{p}",
		"/api/v1/projects/",
	} {
		if msg := register(pattern); msg == "" {
			t.Errorf("%q: Handle con AccessBearer debió hacer panic", pattern)
		}
	}
	if msg := register("POST /api/v1/mcp"); msg != "" {
		t.Errorf("POST /api/v1/mcp: panic inesperado: %s", msg)
	}
}
