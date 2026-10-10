package container

import (
	"bytes"
	"context"
	jsonv2 "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
)

const testPassword = "una-contraseña-larga"

// api hace peticiones al servidor real del contenedor, con los cuatro módulos.
type api struct {
	t *testing.T
	c *Container
}

// do envía la petición con Origin: PanelOrigin por defecto; body se serializa a JSON si no es nil.
func (a api) do(method, path string, body any, opts ...httptestutil.Opt) *httptest.ResponseRecorder {
	a.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := jsonv2.Marshal(body, jsonv2.Deterministic(true))
		if err != nil {
			a.t.Fatalf("serializando el body de %s %s: %v", method, path, err)
		}
		reader = bytes.NewReader(raw)
	}
	r := httptest.NewRequest(method, path, reader)
	r.Header.Set("Origin", httptestutil.PanelOrigin)
	if reader != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	for _, o := range opts {
		o(r)
	}
	w := httptest.NewRecorder()
	a.c.Server.ServeHTTP(w, r)
	return w
}

// setup crea el primer usuario con el link de setup, como lo haría quien
// instala muelle, y devuelve la opción con su sesión.
func (a api) setup() httptestutil.Opt {
	a.t.Helper()
	token, created, err := a.c.Identity.EnsureSetupInvite(context.Background())
	if err != nil || !created {
		a.t.Fatalf("EnsureSetupInvite: created=%v err=%v", created, err)
	}
	w := a.do("POST", "/api/v1/auth/setup", map[string]string{"token": token, "username": "admin", "password": testPassword})
	httptestutil.Expect(a.t, w, http.StatusCreated)
	return httptestutil.WithSession(httptestutil.SessionFrom(a.t, w))
}

// TestTokenScopes es la matriz de permisos de §10.1 con el servidor real y
// los cuatro módulos: qué puede hacer un token según su alcance. Si alguna
// fila cambia, cambió la seguridad.
func TestTokenScopes(t *testing.T) {
	e := api{t: t, c: newTestContainer(t)}
	s := e.setup()

	for _, req := range []struct {
		path string
		body any
	}{
		{"/api/v1/projects", map[string]string{"slug": "auditfy", "name": "Auditfy"}},
		{"/api/v1/projects/auditfy/stages", map[string]string{"slug": "staging"}},
		{"/api/v1/projects/auditfy/stages", map[string]string{"slug": "production"}},
		{"/api/v1/projects", map[string]string{"slug": "otro", "name": "Otro"}},
		{"/api/v1/projects/auditfy/stages/staging/apps",
			map[string]any{"slug": "api", "kind": "service", "spec": map[string]any{"image": "x", "port": 80}}},
		{"/api/v1/projects/auditfy/stages/production/apps",
			map[string]any{"slug": "api", "kind": "service", "spec": map[string]any{"image": "x", "port": 80}}},
	} {
		httptestutil.Expect(t, e.do("POST", req.path, req.body, s), http.StatusCreated)
	}

	newToken := func(stage string) string {
		t.Helper()
		w := e.do("POST", "/api/v1/tokens", map[string]any{"name": "ci", "project": "auditfy", "stage": stage}, s)
		httptestutil.Expect(t, w, http.StatusCreated)
		tok, _ := httptestutil.Decode[map[string]any](t, w)["token"].(string)
		if !strings.HasPrefix(tok, "mk_") {
			t.Fatalf("token=%s", tok)
		}
		return tok
	}
	stagingTok := newToken("staging")
	projectTok := newToken("")

	env := map[string]any{"vars": map[string]string{"A": "1"}}
	cases := []struct {
		name   string
		token  string
		method string
		path   string
		body   any
		status int
		code   string // código de error esperado ("" si no es un error)
	}{
		{"stage: lee su app", stagingTok, "GET", "/api/v1/projects/auditfy/stages/staging/apps/api", nil, 200, ""},
		{"stage: reemplaza env de su app", stagingTok, "PUT", "/api/v1/projects/auditfy/stages/staging/apps/api/env", env, 204, ""},
		{"stage: reemplaza env de su stage", stagingTok, "PUT", "/api/v1/projects/auditfy/stages/staging/env", env, 204, ""},
		{"stage: lee el proyecto", stagingTok, "GET", "/api/v1/projects/auditfy", nil, 200, ""},
		{"stage: no toca otro stage", stagingTok, "PUT", "/api/v1/projects/auditfy/stages/production/apps/api/env", env, 403, "out_of_scope"},
		{"stage: no lee otro stage", stagingTok, "GET", "/api/v1/projects/auditfy/stages/production/apps/api", nil, 403, "out_of_scope"},
		{"stage: no toca otro proyecto", stagingTok, "GET", "/api/v1/projects/otro", nil, 403, "out_of_scope"},
		{"stage: no crea apps", stagingTok, "POST", "/api/v1/projects/auditfy/stages/staging/apps", map[string]any{"slug": "x"}, 403, "token_not_allowed"},
		{"stage: no lista proyectos", stagingTok, "GET", "/api/v1/projects", nil, 403, "token_not_allowed"},
		{"stage: no crea tokens", stagingTok, "POST", "/api/v1/tokens", map[string]any{"name": "x", "project": "auditfy"}, 403, "token_not_allowed"},
		{"stage: no usa auth/me", stagingTok, "GET", "/api/v1/auth/me", nil, 403, "token_not_allowed"},
		{"proyecto: toca cualquier stage", projectTok, "PUT", "/api/v1/projects/auditfy/stages/production/apps/api/env", env, 204, ""},
		{"proyecto: no toca otro proyecto", projectTok, "GET", "/api/v1/projects/otro", nil, 403, "out_of_scope"},
		{"proyecto: no borra stages", projectTok, "DELETE", "/api/v1/projects/auditfy/stages/production", nil, 403, "token_not_allowed"},
		{"token inventado", "mk_inventado", "GET", "/api/v1/projects/auditfy", nil, 401, "unauthenticated"},
	}
	for _, c := range cases {
		// Los tokens no necesitan Origin: no hay cookies que robar.
		w := e.do(c.method, c.path, c.body, httptestutil.WithBearer(c.token), httptestutil.WithOrigin(""))
		if w.Code != c.status {
			t.Errorf("%s: status %d, esperaba %d (%s)", c.name, w.Code, c.status, w.Body.String())
			continue
		}
		if c.code != "" {
			if code := httptestutil.ErrorCode(t, w); code != c.code {
				t.Errorf("%s: code %s, esperaba %s", c.name, code, c.code)
			}
		}
	}

	// Revocar un token lo invalida. Los tokens se listan por id: el primero es el de staging.
	w := e.do("GET", "/api/v1/tokens", nil, s)
	httptestutil.Expect(t, w, http.StatusOK)
	tokens := httptestutil.Decode[[]map[string]any](t, w)
	if len(tokens) != 2 || tokens[0]["stage"] != "staging" {
		t.Fatalf("tokens=%v", tokens)
	}
	id := strconv.Itoa(int(tokens[0]["id"].(float64)))
	httptestutil.Expect(t, e.do("DELETE", "/api/v1/tokens/"+id, nil, s), http.StatusNoContent)
	httptestutil.Expect(t, e.do("GET", "/api/v1/projects/auditfy", nil, httptestutil.WithBearer(stagingTok)), http.StatusUnauthorized)
	// El otro token sigue sirviendo.
	httptestutil.Expect(t, e.do("GET", "/api/v1/projects/auditfy", nil, httptestutil.WithBearer(projectTok)), http.StatusOK)
}
