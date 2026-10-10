package http_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/cmd/http/utils"
)

func TestInviteFlow(t *testing.T) {
	e := newEnv(t)
	s := httptestutil.WithSession(e.login())

	w := e.do("POST", "/api/v1/invites", nil, s)
	httptestutil.Expect(t, w, http.StatusCreated)
	inv := httptestutil.Decode[map[string]any](t, w)
	url := inv["url"].(string)
	token, ok := strings.CutPrefix(url, httptestutil.PanelOrigin+"/invite/")
	if !ok || token == "" {
		t.Fatalf("url=%s", url)
	}
	if exp, _ := time.Parse(time.RFC3339, inv["expires_at"].(string)); !exp.Equal(e.clock.Now().Add(24 * time.Hour)) {
		t.Fatalf("expires_at=%v", inv["expires_at"])
	}

	w = e.do("POST", "/api/v1/auth/invite/"+token, map[string]string{"username": "ana", "password": testPassword})
	httptestutil.Expect(t, w, http.StatusCreated)
	ana := httptestutil.SessionFrom(t, w)
	httptestutil.Expect(t, e.do("GET", "/api/v1/auth/me", nil, httptestutil.WithSession(ana)), http.StatusOK)

	w = e.do("POST", "/api/v1/auth/invite/"+token, map[string]string{"username": "beto", "password": testPassword})
	expectCode(t, w, http.StatusBadRequest, "invalid_invite")

	w = e.do("GET", "/api/v1/users", nil, s)
	httptestutil.Expect(t, w, http.StatusOK)
	users := httptestutil.Decode[[]map[string]any](t, w)
	if len(users) != 2 || users[0]["username"] != "admin" || users[0]["totp_enabled"] != false || users[1]["username"] != "ana" {
		t.Fatalf("users=%v", users)
	}
	if _, err := time.Parse(time.RFC3339, users[0]["created_at"].(string)); err != nil {
		t.Fatalf("created_at=%v", users[0]["created_at"])
	}
}

// El link de la invitación sale de Deps.PanelOrigin, no del header Origin de
// la petición.
func TestInviteURLUsesConfiguredOrigin(t *testing.T) {
	e := newEnvFull(t, "https://muelle.example", nil)
	w := e.do("POST", "/api/v1/invites", nil, httptestutil.WithSession(e.login()))
	httptestutil.Expect(t, w, http.StatusCreated)
	if url := httptestutil.Decode[map[string]any](t, w)["url"].(string); !strings.HasPrefix(url, "https://muelle.example/invite/") {
		t.Fatalf("url=%s", url)
	}
}

func TestAcceptInviteNameTaken(t *testing.T) {
	e := newEnv(t)
	s := httptestutil.WithSession(e.login())
	w := e.do("POST", "/api/v1/invites", nil, s)
	token, _ := strings.CutPrefix(httptestutil.Decode[map[string]any](t, w)["url"].(string), httptestutil.PanelOrigin+"/invite/")

	w = e.do("POST", "/api/v1/auth/invite/"+token, map[string]string{"username": "admin", "password": testPassword})
	expectCode(t, w, http.StatusConflict, "conflict")
	w = e.do("POST", "/api/v1/auth/invite/"+token, map[string]string{"username": "ana", "password": "corta"})
	expectCode(t, w, http.StatusUnprocessableEntity, "invalid")
	// La invitación sigue disponible.
	w = e.do("POST", "/api/v1/auth/invite/"+token, map[string]string{"username": "ana", "password": testPassword})
	httptestutil.Expect(t, w, http.StatusCreated)
}

func TestDeleteUserAndResetTOTP(t *testing.T) {
	e := newEnv(t)
	s := httptestutil.WithSession(e.login())
	e.user("ana")

	w := e.do("DELETE", "/api/v1/users/admin", nil, s)
	expectCode(t, w, http.StatusConflict, "cannot_delete_self")
	httptestutil.Expect(t, e.do("DELETE", "/api/v1/users/ana/totp", nil, s), http.StatusNoContent)
	httptestutil.Expect(t, e.do("DELETE", "/api/v1/users/ana", nil, s), http.StatusNoContent)
	expectCode(t, e.do("DELETE", "/api/v1/users/ana", nil, s), http.StatusNotFound, "not_found")
	expectCode(t, e.do("DELETE", "/api/v1/users/ana/totp", nil, s), http.StatusNotFound, "not_found")
}

func TestResetTOTPLogsUserOut(t *testing.T) {
	e := newEnv(t)
	s := httptestutil.WithSession(e.login())
	e.user("ana")
	w := e.do("POST", "/api/v1/auth/login", map[string]string{"username": "ana", "password": testPassword})
	ana := httptestutil.WithSession(httptestutil.SessionFrom(t, w))
	httptestutil.Expect(t, e.do("GET", "/api/v1/auth/me", nil, ana), http.StatusOK)

	httptestutil.Expect(t, e.do("DELETE", "/api/v1/users/ana/totp", nil, s), http.StatusNoContent)
	expectCode(t, e.do("GET", "/api/v1/auth/me", nil, ana), http.StatusUnauthorized, "unauthenticated")
	httptestutil.Expect(t, e.do("GET", "/api/v1/auth/me", nil, s), http.StatusOK)
}

func TestTokensAPI(t *testing.T) {
	e := newEnv(t)
	s := httptestutil.WithSession(e.login())
	auditfy := e.exec(`INSERT INTO projects (slug, name, created_at) VALUES ('auditfy', 'Auditfy', 0)`)
	e.exec(`INSERT INTO stages (project_id, slug, created_at) VALUES (?, 'staging', 0)`, auditfy)

	w := e.do("POST", "/api/v1/tokens", map[string]any{"name": "ci", "project": "auditfy", "stage": "staging", "expires_in_days": 30}, s)
	httptestutil.Expect(t, w, http.StatusCreated)
	created := httptestutil.Decode[map[string]any](t, w)
	if tok, _ := created["token"].(string); !strings.HasPrefix(tok, "mk_") || len(tok) != 46 ||
		created["name"] != "ci" || created["project"] != "auditfy" || created["stage"] != "staging" ||
		created["last_used_at"] != nil || created["expires_at"] == nil {
		t.Fatalf("created=%v", created)
	}

	w = e.do("POST", "/api/v1/tokens", map[string]any{"name": "ci-todo", "project": "auditfy"}, s)
	httptestutil.Expect(t, w, http.StatusCreated)
	project := httptestutil.Decode[map[string]any](t, w)
	if _, ok := project["stage"]; ok || project["expires_at"] != nil {
		t.Fatalf("un token de proyecto no trae stage y sin días no vence: %v", project)
	}

	w = e.do("GET", "/api/v1/tokens", nil, s)
	httptestutil.Expect(t, w, http.StatusOK)
	list := httptestutil.Decode[[]map[string]any](t, w)
	if len(list) != 2 || list[0]["id"] != created["id"] || list[1]["id"] != project["id"] {
		t.Fatalf("list=%v", list)
	}
	for _, tok := range list {
		if _, ok := tok["token"]; ok {
			t.Fatalf("el token en claro se muestra una sola vez: %v", tok)
		}
	}

	id := strconv.Itoa(int(created["id"].(float64)))
	httptestutil.Expect(t, e.do("DELETE", "/api/v1/tokens/"+id, nil, s), http.StatusNoContent)
	expectCode(t, e.do("DELETE", "/api/v1/tokens/"+id, nil, s), http.StatusNotFound, "not_found")
	expectCode(t, e.do("DELETE", "/api/v1/tokens/abc", nil, s), http.StatusUnprocessableEntity, "invalid")
}

func TestCreateTokenErrors(t *testing.T) {
	e := newEnv(t)
	s := httptestutil.WithSession(e.login())
	e.exec(`INSERT INTO projects (slug, name, created_at) VALUES ('auditfy', 'Auditfy', 0)`)

	expectCode(t, e.do("POST", "/api/v1/tokens", map[string]any{"name": "", "project": "auditfy"}, s), http.StatusUnprocessableEntity, "invalid")
	expectCode(t, e.do("POST", "/api/v1/tokens", map[string]any{"name": "ci", "project": "auditfy", "expires_in_days": -1}, s), http.StatusUnprocessableEntity, "invalid")
	expectCode(t, e.do("POST", "/api/v1/tokens", map[string]any{"name": "ci", "project": "nope"}, s), http.StatusNotFound, "not_found")
	expectCode(t, e.do("POST", "/api/v1/tokens", map[string]any{"name": "ci", "project": "auditfy", "stage": "nope"}, s), http.StatusNotFound, "not_found")
	expectCode(t, e.do("POST", "/api/v1/tokens", map[string]any{"name": "ci", "project": "auditfy", "scope": "x"}, s), http.StatusBadRequest, "invalid_json")
}

// TestTokenScopes es la matriz de §10.1 (Tarea 22 de la referencia) con el
// Authenticator real. Las rutas de catalog y secrets todavía no existen en
// este módulo, así que se registran stand-ins con el mismo nivel de acceso
// que tendrán; la matriz con las rutas reales se prueba en la integración.
func TestTokenScopes(t *testing.T) {
	ok := func(status int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }
	}
	e := newEnvWith(t, func(k *utils.Kit) {
		k.Handle("GET /api/v1/projects", utils.AccessSession, ok(200))
		k.Handle("GET /api/v1/projects/{p}", utils.AccessToken, ok(200))
		k.Handle("DELETE /api/v1/projects/{p}/stages/{s}", utils.AccessSession, ok(204))
		k.Handle("POST /api/v1/projects/{p}/stages/{s}/apps", utils.AccessSession, ok(201))
		k.Handle("GET /api/v1/projects/{p}/stages/{s}/apps/{a}", utils.AccessToken, ok(200))
		k.Handle("PUT /api/v1/projects/{p}/stages/{s}/env", utils.AccessToken, ok(204))
		k.Handle("PUT /api/v1/projects/{p}/stages/{s}/apps/{a}/env", utils.AccessToken, ok(204))
	})
	s := httptestutil.WithSession(e.login())
	auditfy := e.exec(`INSERT INTO projects (slug, name, created_at) VALUES ('auditfy', 'Auditfy', 0)`)
	e.exec(`INSERT INTO projects (slug, name, created_at) VALUES ('otro', 'Otro', 0)`)
	e.exec(`INSERT INTO stages (project_id, slug, created_at) VALUES (?, 'staging', 0)`, auditfy)
	e.exec(`INSERT INTO stages (project_id, slug, created_at) VALUES (?, 'production', 0)`, auditfy)

	newToken := func(stage string) string {
		w := e.do("POST", "/api/v1/tokens", map[string]any{"name": "ci", "project": "auditfy", "stage": stage}, s)
		httptestutil.Expect(t, w, http.StatusCreated)
		return httptestutil.Decode[map[string]any](t, w)["token"].(string)
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
	}{
		{"stage: lee su app", stagingTok, "GET", "/api/v1/projects/auditfy/stages/staging/apps/api", nil, 200},
		{"stage: reemplaza env de su app", stagingTok, "PUT", "/api/v1/projects/auditfy/stages/staging/apps/api/env", env, 204},
		{"stage: reemplaza env de su stage", stagingTok, "PUT", "/api/v1/projects/auditfy/stages/staging/env", env, 204},
		{"stage: lee el proyecto", stagingTok, "GET", "/api/v1/projects/auditfy", nil, 200},
		{"stage: no toca otro stage", stagingTok, "PUT", "/api/v1/projects/auditfy/stages/production/apps/api/env", env, 403},
		{"stage: no lee otro stage", stagingTok, "GET", "/api/v1/projects/auditfy/stages/production/apps/api", nil, 403},
		{"stage: no toca otro proyecto", stagingTok, "GET", "/api/v1/projects/otro", nil, 403},
		{"stage: no crea apps", stagingTok, "POST", "/api/v1/projects/auditfy/stages/staging/apps", map[string]any{"slug": "x"}, 403},
		{"stage: no lista proyectos", stagingTok, "GET", "/api/v1/projects", nil, 403},
		{"stage: no crea tokens", stagingTok, "POST", "/api/v1/tokens", map[string]any{"name": "x", "project": "auditfy"}, 403},
		{"stage: no lista tokens", stagingTok, "GET", "/api/v1/tokens", nil, 403},
		{"stage: no usa auth/me", stagingTok, "GET", "/api/v1/auth/me", nil, 403},
		{"stage: no invita", stagingTok, "POST", "/api/v1/invites", nil, 403},
		{"proyecto: toca cualquier stage", projectTok, "PUT", "/api/v1/projects/auditfy/stages/production/apps/api/env", env, 204},
		{"proyecto: no toca otro proyecto", projectTok, "GET", "/api/v1/projects/otro", nil, 403},
		{"proyecto: no borra stages", projectTok, "DELETE", "/api/v1/projects/auditfy/stages/production", nil, 403},
		{"proyecto: no borra usuarios", projectTok, "DELETE", "/api/v1/users/admin", nil, 403},
		{"token inventado", "mk_inventado", "GET", "/api/v1/projects/auditfy", nil, 401},
	}
	for _, c := range cases {
		// Los tokens no necesitan Origin: no hay cookies que robar.
		w := e.do(c.method, c.path, c.body, httptestutil.WithBearer(c.token), httptestutil.WithOrigin(""))
		if w.Code != c.status {
			t.Errorf("%s: status %d, esperaba %d (%s)", c.name, w.Code, c.status, w.Body.String())
		}
	}

	// Revocar un token lo invalida.
	w := e.do("GET", "/api/v1/tokens", nil, s)
	tokens := httptestutil.Decode[[]map[string]any](t, w)
	id := strconv.Itoa(int(tokens[0]["id"].(float64)))
	httptestutil.Expect(t, e.do("DELETE", "/api/v1/tokens/"+id, nil, s), http.StatusNoContent)
	httptestutil.Expect(t, e.do("GET", "/api/v1/projects/auditfy", nil, httptestutil.WithBearer(stagingTok)), http.StatusUnauthorized)
}
