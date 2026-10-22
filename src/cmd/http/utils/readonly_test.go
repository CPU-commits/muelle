package utils

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

var readOnlyTokens = map[string]entity.Principal{
	"mk_project":    {Token: &entity.TokenScope{ID: 7, Name: "ci", ProjectSlug: "auditfy"}},
	"mk_stage":      {Token: &entity.TokenScope{ID: 8, Name: "ci-staging", ProjectSlug: "auditfy", StageSlug: "staging"}},
	"mk_ro":         {Token: &entity.TokenScope{ID: 10, Name: "ro", ProjectSlug: "auditfy", ReadOnly: true}},
	"mk_stage_ro":   {Token: &entity.TokenScope{ID: 11, Name: "ro-staging", ProjectSlug: "auditfy", StageSlug: "staging", ReadOnly: true}},
	"mk_all":        {Token: &entity.TokenScope{ID: 12, Name: "todos", ReadOnly: true, AllProjects: true}},
	"mk_all_broken": {Token: &entity.TokenScope{ID: 13, Name: "roto", AllProjects: true}},
}

// newReadOnlyKit es un Kit con rutas AccessToken de lectura y de escritura,
// y los tokens de readOnlyTokens.
func newReadOnlyKit() (*Kit, *http.ServeMux) {
	mux := http.NewServeMux()
	k := NewKit(mux, fakeAuth{tokens: readOnlyTokens}, true, discard())
	ok := func(w http.ResponseWriter, r *http.Request) { WriteJSON(w, http.StatusOK, map[string]bool{"ok": true}) }
	k.Handle("GET /api/v1/projects/{p}", AccessToken, ok)
	k.Handle("GET /api/v1/projects/{p}/stages/{s}/env", AccessToken, ok)
	k.Handle("HEAD /api/v1/projects/{p}/stages/{s}/env", AccessToken, ok)
	k.Handle("PUT /api/v1/projects/{p}/stages/{s}/env", AccessToken, ok)
	k.Handle("DELETE /api/v1/projects/{p}", AccessToken, ok)
	return k, mux
}

// Contrato §8.1: un token de solo lectura en una escritura es 403
// read_only_token antes que el alcance (no dice si el recurso existe), y
// uno de todos los proyectos lee cualquier proyecto.
func TestHandleReadOnlyToken(t *testing.T) {
	_, mux := newReadOnlyKit()
	const env = "/api/v1/projects/auditfy/stages/staging/env"
	cases := []struct {
		name, method, path, token string
		status                    int
		code                      string
	}{
		{"solo lectura, leer", "GET", env, "mk_ro", 200, ""},
		{"solo lectura, HEAD", "HEAD", env, "mk_ro", 200, ""},
		{"solo lectura, escribir", "PUT", env, "mk_ro", 403, "read_only_token"},
		{"solo lectura, escribir en otro proyecto", "PUT", "/api/v1/projects/otro/stages/staging/env", "mk_ro", 403, "read_only_token"},
		{"solo lectura, borrar el proyecto", "DELETE", "/api/v1/projects/auditfy", "mk_ro", 403, "read_only_token"},
		{"solo lectura, leer otro proyecto", "GET", "/api/v1/projects/otro", "mk_ro", 403, "out_of_scope"},
		{"stage de solo lectura, su stage", "GET", env, "mk_stage_ro", 200, ""},
		{"stage de solo lectura, otro stage", "GET", "/api/v1/projects/auditfy/stages/production/env", "mk_stage_ro", 403, "out_of_scope"},
		{"stage de solo lectura, escribir en otro stage", "PUT", "/api/v1/projects/auditfy/stages/production/env", "mk_stage_ro", 403, "read_only_token"},
		{"todos los proyectos, leer", "GET", "/api/v1/projects/otro/stages/x/env", "mk_all", 200, ""},
		{"todos los proyectos, leer un proyecto", "GET", "/api/v1/projects/auditfy", "mk_all", 200, ""},
		{"todos los proyectos, escribir", "PUT", env, "mk_all", 403, "read_only_token"},
		{"todos los proyectos sin solo lectura", "GET", env, "mk_all_broken", 403, "out_of_scope"},
		{"escritura, escribir", "PUT", env, "mk_project", 200, ""},
	}
	for _, c := range cases {
		w := serve(mux, c.method, c.path, bearer(c.token))
		if w.Code != c.status {
			t.Errorf("%s: status %d, esperaba %d: %s", c.name, w.Code, c.status, w.Body.String())
			continue
		}
		if c.code != "" {
			b := decodeError(t, w)
			if b.Error.Code != c.code {
				t.Errorf("%s: code %q, esperaba %q", c.name, b.Error.Code, c.code)
			}
			if c.code == "read_only_token" && b.Error.Message != "el token es de solo lectura" {
				t.Errorf("%s: message %q", c.name, b.Error.Message)
			}
		}
	}
}

// Revalidate trata el alcance como lectura: un token de solo lectura, o de
// todos los proyectos, mantiene los streams de su alcance.
func TestRevalidateReadOnlyTokens(t *testing.T) {
	k, _ := newReadOnlyKit()
	req := func(method, p, s, token string) *http.Request {
		r := httptest.NewRequest(method, "/api/v1/projects/"+p+"/stages/"+s+"/apps/api/logs", nil)
		r.SetPathValue("p", p)
		r.SetPathValue("s", s)
		r.SetPathValue("a", "api")
		bearer(token)(r)
		return r
	}
	for _, c := range []struct {
		name string
		r    *http.Request
		want error
	}{
		{"solo lectura", req("GET", "auditfy", "staging", "mk_ro"), nil},
		{"solo lectura, abierto con POST", req("POST", "auditfy", "staging", "mk_ro"), nil},
		{"stage de solo lectura, su stage", req("GET", "auditfy", "staging", "mk_stage_ro"), nil},
		{"stage de solo lectura, otro stage", req("GET", "auditfy", "production", "mk_stage_ro"), entity.ErrForbidden},
		{"todos los proyectos", req("GET", "otro", "production", "mk_all"), nil},
		{"todos los proyectos sin solo lectura", req("GET", "auditfy", "staging", "mk_all_broken"), entity.ErrForbidden},
		{"solo lectura, otro proyecto", req("GET", "otro", "staging", "mk_ro"), entity.ErrForbidden},
	} {
		err := k.Revalidate(c.r, AccessToken)
		switch {
		case c.want == nil && err != nil:
			t.Errorf("%s: %v, esperaba nil", c.name, err)
		case c.want != nil && !errors.Is(err, c.want):
			t.Errorf("%s: %v, esperaba %v", c.name, err, c.want)
		}
	}
}
