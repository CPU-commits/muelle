package container

import (
	"bytes"
	"context"
	jsonv2 "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
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

// newToken crea un token de API con la sesión s y devuelve su id y el token en claro.
func (a api) newToken(s httptestutil.Opt, project, stage string) (id, token string) {
	a.t.Helper()
	w := a.do("POST", "/api/v1/tokens", map[string]any{"name": "ci", "project": project, "stage": stage}, s)
	httptestutil.Expect(a.t, w, http.StatusCreated)
	res := httptestutil.Decode[map[string]any](a.t, w)
	token, _ = res["token"].(string)
	n, _ := res["id"].(float64)
	if !strings.HasPrefix(token, "mk_") || n <= 0 {
		a.t.Fatalf("token=%v", res)
	}
	return fmt.Sprint(int64(n)), token
}

// bearer hace la petición con un token de API. Los tokens no necesitan
// Origin: no hay cookies que robar.
func (a api) bearer(token, method, path string, body any) *httptest.ResponseRecorder {
	a.t.Helper()
	return a.do(method, path, body, httptestutil.WithBearer(token), httptestutil.WithOrigin(""))
}

// expect revisa el status y, si code no es "", el código de error. Usa
// t.Errorf para que una tabla muestre todas las filas que fallan.
func expect(t *testing.T, name string, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if w.Code != status {
		t.Errorf("%s: status %d, esperaba %d (%s)", name, w.Code, status, w.Body.String())
		return
	}
	if code != "" {
		if got := httptestutil.ErrorCode(t, w); got != code {
			t.Errorf("%s: code %s, esperaba %s", name, got, code)
		}
	}
}

// TestTokenScopes es la matriz de permisos de §10.1 con el servidor real, los
// cuatro módulos y tokens de verdad. Recorre las 11 rutas que aceptan tokens
// (AccessToken) con cada clase de token: del proyecto, de un stage (en su
// stage y en otro), de otro proyecto y revocado. Si alguna fila cambia,
// cambió la seguridad.
func TestTokenScopes(t *testing.T) {
	e := api{t: t, c: newTestContainer(t)}
	s := e.setup()

	// auditfy con staging y production, y otro proyecto con su staging; cada
	// stage con una app "api".
	app := map[string]any{"slug": "api", "kind": "service", "spec": map[string]any{"image": "x", "port": 80}}
	for _, req := range []struct {
		path string
		body any
	}{
		{"/api/v1/projects", map[string]string{"slug": "auditfy", "name": "Auditfy"}},
		{"/api/v1/projects/auditfy/stages", map[string]string{"slug": "staging"}},
		{"/api/v1/projects/auditfy/stages", map[string]string{"slug": "production"}},
		{"/api/v1/projects", map[string]string{"slug": "otro", "name": "Otro"}},
		{"/api/v1/projects/otro/stages", map[string]string{"slug": "staging"}},
		{"/api/v1/projects/auditfy/stages/staging/apps", app},
		{"/api/v1/projects/auditfy/stages/production/apps", app},
		{"/api/v1/projects/otro/stages/staging/apps", app},
	} {
		httptestutil.Expect(t, e.do("POST", req.path, req.body, s), http.StatusCreated)
	}

	// production tiene variables visibles (secret false) con "prod" en el
	// valor: si una respuesta al token de staging las trae, se nota.
	const P = "/api/v1/projects/auditfy"
	for path, vars := range map[string]map[string]string{
		P + "/stages/production/env":          {"STAGE_PUBLIC": "prod-stage"},
		P + "/stages/production/apps/api/env": {"PUBLIC_URL": "https://prod.example"},
	} {
		httptestutil.Expect(t, e.do("PUT", path, map[string]any{"vars": vars, "secret": false}, s), http.StatusNoContent)
		// Con permiso se leen: la revisión de abajo puede detectar una fuga.
		w := e.do("GET", path, nil, s)
		httptestutil.Expect(t, w, http.StatusOK)
		if !strings.Contains(w.Body.String(), "prod") {
			t.Fatalf("GET %s con sesión: %s", path, w.Body.String())
		}
	}

	_, stagingTok := e.newToken(s, "auditfy", "staging")
	_, projectTok := e.newToken(s, "auditfy", "")
	_, otherTok := e.newToken(s, "otro", "")
	revokedID, revokedTok := e.newToken(s, "auditfy", "")
	httptestutil.Expect(t, e.do("DELETE", "/api/v1/tokens/"+revokedID, nil, s), http.StatusNoContent)

	// Las 11 rutas AccessToken: 2 del proyecto (sin {s}) y 9 de un stage.
	type route struct {
		method, path string // en las rutas de un stage, path va después de …/stages/{s}
		body         any
		ok           int // status con permiso
	}
	put := map[string]any{"vars": map[string]string{"A": "1"}}
	patch := map[string]any{"set": map[string]string{"B": "2"}}
	projectRoutes := []route{
		{"GET", "", nil, 200},
		{"GET", "/stages", nil, 200},
	}
	stageRoutes := []route{
		{"GET", "", nil, 200},
		{"GET", "/apps", nil, 200},
		{"GET", "/apps/api", nil, 200},
		{"GET", "/env", nil, 200},
		{"PUT", "/env", put, 204},
		{"PATCH", "/env", patch, 204},
		{"GET", "/apps/api/env", nil, 200},
		{"PUT", "/apps/api/env", put, 204},
		{"PATCH", "/apps/api/env", patch, 204},
	}

	// want da el status y el código esperados en una ruta de auditfy; stage
	// es "" en las rutas del proyecto. El token de staging va primero: los
	// PUT del token del proyecto, al final, reemplazan las variables de
	// production que delatarían una fuga.
	allowed := func(_ string, ok int) (int, string) { return ok, "" }
	tokens := []struct {
		name, token string
		want        func(stage string, ok int) (int, string)
	}{
		{"staging", stagingTok, func(stage string, ok int) (int, string) {
			if stage == "production" {
				return 403, "out_of_scope"
			}
			return ok, "" // su stage, y leer el proyecto
		}},
		{"otro proyecto", otherTok, func(string, int) (int, string) { return 403, "out_of_scope" }},
		{"revocado", revokedTok, func(string, int) (int, string) { return 401, "unauthenticated" }},
		{"proyecto", projectTok, allowed},
	}
	rows := 0
	for _, tk := range tokens {
		for _, stage := range []string{"", "staging", "production"} {
			routes, base := projectRoutes, P
			if stage != "" {
				routes, base = stageRoutes, P+"/stages/"+stage
			}
			for _, r := range routes {
				name := fmt.Sprintf("token %s: %s %s", tk.name, r.method, base+r.path)
				status, code := tk.want(stage, r.ok)
				w := e.bearer(tk.token, r.method, base+r.path, r.body)
				expect(t, name, w, status, code)
				if tk.token == stagingTok && strings.Contains(w.Body.String(), "prod") {
					t.Errorf("%s: el token de staging vio datos de production: %s", name, w.Body.String())
				}
				rows++
			}
		}
	}
	if want := len(tokens) * (len(projectRoutes) + 2*len(stageRoutes)); rows != want || len(projectRoutes)+len(stageRoutes) != 11 {
		t.Fatalf("la matriz tiene %d filas, esperaba %d", rows, want)
	}

	// El 403 del token de otro proyecto es por alcance: en el suyo puede.
	for _, path := range []string{"/api/v1/projects/otro", "/api/v1/projects/otro/stages/staging/apps/api/env"} {
		expect(t, "token otro proyecto en el suyo: GET "+path, e.bearer(otherTok, "GET", path, nil), 200, "")
	}

	// Listar los stages: el token de staging solo ve staging.
	slugs := func(token string) []string {
		t.Helper()
		w := e.bearer(token, "GET", P+"/stages", nil)
		httptestutil.Expect(t, w, http.StatusOK)
		var out []string
		for _, st := range httptestutil.Decode[[]map[string]any](t, w) {
			out = append(out, fmt.Sprint(st["slug"]))
		}
		slices.Sort(out)
		return out
	}
	if got := slugs(stagingTok); !slices.Equal(got, []string{"staging"}) {
		t.Errorf("el token de staging lista los stages %v, esperaba solo staging", got)
	}
	if got := slugs(projectTok); !slices.Equal(got, []string{"production", "staging"}) {
		t.Errorf("el token del proyecto lista los stages %v", got)
	}

	// Lo que no es AccessToken exige sesión, aunque esté debajo de un proyecto.
	for _, c := range []struct {
		token, method, path string
		body                any
	}{
		{stagingTok, "POST", P + "/stages/staging/apps", map[string]any{"slug": "x"}},
		{stagingTok, "GET", "/api/v1/projects", nil},
		{stagingTok, "POST", "/api/v1/tokens", map[string]any{"name": "x", "project": "auditfy"}},
		{stagingTok, "GET", "/api/v1/auth/me", nil},
		{projectTok, "GET", "/api/v1/projects", nil},
		{projectTok, "POST", "/api/v1/projects", map[string]string{"slug": "x", "name": "X"}},
		{projectTok, "PATCH", P, map[string]string{"name": "x"}},
		{projectTok, "DELETE", P, nil},
		{projectTok, "POST", P + "/stages", map[string]string{"slug": "x"}},
		{projectTok, "DELETE", P + "/stages/production", nil},
		{projectTok, "POST", P + "/stages/staging/apps", map[string]any{"slug": "x"}},
		{projectTok, "PATCH", P + "/stages/staging/apps/api", map[string]any{"deploy_timeout_s": 10}},
		{projectTok, "DELETE", P + "/stages/staging/apps/api", nil},
		{projectTok, "GET", "/api/v1/settings", nil},
		{projectTok, "PATCH", "/api/v1/settings", map[string]any{}},
		{projectTok, "GET", "/api/v1/audit", nil},
		{projectTok, "GET", "/api/v1/users", nil},
		{projectTok, "GET", "/api/v1/tokens", nil},
		{projectTok, "POST", "/api/v1/tokens", map[string]any{"name": "x", "project": "auditfy"}},
		{projectTok, "DELETE", "/api/v1/tokens/" + revokedID, nil},
		{projectTok, "POST", "/api/v1/invites", nil},
		{projectTok, "GET", "/api/v1/auth/me", nil},
		{projectTok, "POST", "/api/v1/auth/logout", nil},
	} {
		name := fmt.Sprintf("token %s: %s %s", map[string]string{stagingTok: "staging", projectTok: "proyecto"}[c.token], c.method, c.path)
		expect(t, name, e.bearer(c.token, c.method, c.path, c.body), 403, "token_not_allowed")
	}

	expect(t, "token inventado", e.bearer("mk_inventado", "GET", P, nil), 401, "unauthenticated")
}

// newTokenWith crea un token con el cuerpo body (contrato §8.1) y devuelve el
// token en claro.
func (a api) newTokenWith(s httptestutil.Opt, body map[string]any) string {
	a.t.Helper()
	w := a.do("POST", "/api/v1/tokens", body, s)
	httptestutil.Expect(a.t, w, http.StatusCreated)
	token, _ := httptestutil.Decode[map[string]any](a.t, w)["token"].(string)
	if !strings.HasPrefix(token, "mk_") {
		a.t.Fatalf("token=%q", token)
	}
	return token
}

// scopeFixture deja p1 (stages s1 y s2) y p2 (stage s1), cada stage con la
// app de formulario "a", y los cinco tipos de token de la matriz del
// contrato §8.1: P (p1), PS (p1/s1), P·L, PS·L (los mismos, de solo lectura) y
// T·L (todos los proyectos, solo lectura).
type scopeFixture struct {
	api
	s                  httptestutil.Opt
	p, ps, pl, psl, tl string
}

func newScopeFixture(t *testing.T) scopeFixture {
	t.Helper()
	e := api{t: t, c: newTestContainer(t)}
	s := e.setup()
	app := map[string]any{"slug": "a", "kind": "service", "spec": map[string]any{"image": "nginx:alpine", "port": 80}}
	for _, req := range []struct {
		path string
		body any
	}{
		{"/api/v1/projects", map[string]string{"slug": "p1", "name": "P1"}},
		{"/api/v1/projects/p1/stages", map[string]string{"slug": "s1"}},
		{"/api/v1/projects/p1/stages", map[string]string{"slug": "s2"}},
		{"/api/v1/projects", map[string]string{"slug": "p2", "name": "P2"}},
		{"/api/v1/projects/p2/stages", map[string]string{"slug": "s1"}},
		{"/api/v1/projects/p1/stages/s1/apps", app},
		{"/api/v1/projects/p1/stages/s2/apps", app},
		{"/api/v1/projects/p2/stages/s1/apps", app},
	} {
		httptestutil.Expect(t, e.do("POST", req.path, req.body, s), http.StatusCreated)
	}
	return scopeFixture{
		api: e, s: s,
		p:   e.newTokenWith(s, map[string]any{"name": "p", "project": "p1"}),
		ps:  e.newTokenWith(s, map[string]any{"name": "ps", "project": "p1", "stage": "s1"}),
		pl:  e.newTokenWith(s, map[string]any{"name": "pl", "project": "p1", "read_only": true}),
		psl: e.newTokenWith(s, map[string]any{"name": "psl", "project": "p1", "stage": "s1", "read_only": true}),
		tl:  e.newTokenWith(s, map[string]any{"name": "tl", "all_projects": true, "read_only": true}),
	}
}

// La matriz REST del contrato §8.1 con el servidor real: cada ruta · token de
// lectura y de escritura (catalog, secrets y deploy) en p1/s1, p1/s2 y p2/s1,
// con los cinco tipos de token. Un token de solo lectura en una escritura es
// read_only_token antes que el alcance (en p2 también); uno de todos los
// proyectos lee cualquier proyecto; las rutas de sesión siguen siendo
// token_not_allowed para todos.
func TestReadOnlyTokenScopes(t *testing.T) {
	e := newScopeFixture(t)

	type route struct {
		method, path string // path va después de /api/v1/projects/{p}, o de …/stages/{s}
		body         any
		status       int // con permiso
		code         string
	}
	projectRoutes := []route{
		{"GET", "", nil, 200, ""},
		{"GET", "/stages", nil, 200, ""},
	}
	put := map[string]any{"vars": map[string]string{"A": "1"}}
	patch := map[string]any{"set": map[string]string{"B": "2"}}
	stageRoutes := []route{
		{"GET", "", nil, 200, ""},
		{"GET", "/apps", nil, 200, ""},
		{"GET", "/apps/a", nil, 200, ""},
		{"GET", "/env", nil, 200, ""},
		{"GET", "/apps/a/env", nil, 200, ""},
		{"GET", "/apps/a/deploys", nil, 200, ""},
		{"GET", "/apps/a/status", nil, 200, ""},
		{"GET", "/apps/a/domains", nil, 200, ""}, // D12: un token lee los dominios de sus apps
		{"PUT", "/env", put, 204, ""},
		{"PATCH", "/env", patch, 204, ""},
		{"PUT", "/apps/a/env", put, 204, ""},
		{"PATCH", "/apps/a/env", patch, 204, ""},
		{"POST", "/apps/a/deploys", map[string]any{}, 202, ""},
		{"POST", "/apps/a/rollback", nil, 409, "no_previous_deploy"},
	}

	// where es "p1", "p1/s1", "p1/s2" o "p2/s1".
	type kind struct {
		name, token string
		reads       func(where string) bool
		writes      func(where string) bool // solo para los de escritura
		readOnly    bool
	}
	in := func(places ...string) func(string) bool {
		return func(where string) bool { return slices.Contains(places, where) }
	}
	kinds := []kind{
		{"P·L", e.pl, in("p1", "p1/s1", "p1/s2"), nil, true},
		{"PS·L", e.psl, in("p1", "p1/s1"), nil, true},
		{"T·L", e.tl, in("p1", "p1/s1", "p1/s2", "p2", "p2/s1"), nil, true},
		{"P", e.p, in("p1", "p1/s1", "p1/s2"), in("p1/s1", "p1/s2"), false},
		{"PS", e.ps, in("p1", "p1/s1"), in("p1/s1"), false},
	}
	rows := 0
	for _, k := range kinds {
		check := func(where, base string, r route) {
			write := r.method != "GET"
			status, code := r.status, r.code
			switch {
			case write && k.readOnly:
				status, code = 403, "read_only_token"
			case write && !k.writes(where), !write && !k.reads(where):
				status, code = 403, "out_of_scope"
			}
			name := fmt.Sprintf("%s: %s %s", k.name, r.method, base+r.path)
			expect(t, name, e.bearer(k.token, r.method, base+r.path, r.body), status, code)
			rows++
		}
		for _, p := range []string{"p1", "p2"} {
			for _, r := range projectRoutes {
				check(p, "/api/v1/projects/"+p, r)
			}
		}
		for _, where := range []string{"p1/s1", "p1/s2", "p2/s1"} {
			p, s, _ := strings.Cut(where, "/")
			for _, r := range stageRoutes {
				check(where, "/api/v1/projects/"+p+"/stages/"+s, r)
			}
		}
	}
	if want := len(kinds) * (2*len(projectRoutes) + 3*len(stageRoutes)); rows != want {
		t.Fatalf("la matriz tiene %d filas, esperaba %d", rows, want)
	}

	// Los tokens de stage listan solo su stage; los demás, los dos.
	stages := func(token string) []string {
		t.Helper()
		w := e.bearer(token, "GET", "/api/v1/projects/p1/stages", nil)
		httptestutil.Expect(t, w, http.StatusOK)
		var out []string
		for _, st := range httptestutil.Decode[[]map[string]any](t, w) {
			out = append(out, fmt.Sprint(st["slug"]))
		}
		slices.Sort(out)
		return out
	}
	for token, want := range map[string][]string{e.p: {"s1", "s2"}, e.ps: {"s1"}, e.pl: {"s1", "s2"}, e.psl: {"s1"}, e.tl: {"s1", "s2"}} {
		if got := stages(token); !slices.Equal(got, want) {
			t.Errorf("GET /projects/p1/stages: %v, esperaba %v", got, want)
		}
	}

	// Lo que es de sesión sigue siéndolo para todo token, también para uno
	// de todos los proyectos (GET /projects incluido).
	for _, k := range kinds {
		for _, c := range []struct {
			method, path string
			body         any
		}{
			{"GET", "/api/v1/projects", nil},
			{"GET", "/api/v1/tokens", nil},
			{"POST", "/api/v1/tokens", map[string]any{"name": "x", "all_projects": true, "read_only": true}},
			{"GET", "/api/v1/registries", nil},
			{"GET", "/api/v1/settings", nil},
			{"GET", "/api/v1/audit", nil},
			{"GET", "/api/v1/proxy", nil},
			{"POST", "/api/v1/projects/p1/stages/s1/apps", map[string]any{"slug": "x"}},
			// De los dominios, un token solo lee la lista (D12).
			{"POST", "/api/v1/projects/p1/stages/s1/apps/a/domains", map[string]any{"hostname": "a.p1.cl", "target_service": "a", "target_port": 80}},
			{"PATCH", "/api/v1/projects/p1/stages/s1/apps/a/domains/a.p1.cl", map[string]any{"proxied": true}},
			{"DELETE", "/api/v1/projects/p1/stages/s1/apps/a/domains/a.p1.cl", nil},
			{"POST", "/api/v1/projects/p1/stages/s1/apps/a/domains/a.p1.cl/adopt", nil},
			{"POST", "/api/v1/projects/p1/stages/s1/apps/a/domains/a.p1.cl/retry", nil},
		} {
			expect(t, fmt.Sprintf("%s: %s %s", k.name, c.method, c.path), e.bearer(k.token, c.method, c.path, c.body), 403, "token_not_allowed")
		}
	}
}
