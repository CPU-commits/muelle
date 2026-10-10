package catalog

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

type env struct {
	c  *httptestutil.Client
	db *sqlitedb.DB
}

const (
	session    = "sess"
	tokAll     = "tok-auditfy"         // todo el proyecto auditfy
	tokStaging = "tok-auditfy-staging" // solo auditfy/staging
	tokBeta    = "tok-beta"            // otro proyecto
)

func newEnv(t *testing.T) *env {
	t.Helper()
	db := sqlitetest.New(t)
	clk := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := New(Deps{DB: db, Clock: clk, Audit: sqlitedb.NewAuditRecorder(db, clk, log), Log: log})
	auth := httptestutil.FakeAuth{
		Sessions: map[string]entity.Principal{session: {UserID: 1, Username: "ana"}},
		Tokens: map[string]entity.Principal{
			tokAll:     {Token: &entity.TokenScope{ID: 1, Name: "ci", ProjectSlug: "auditfy"}},
			tokStaging: {Token: &entity.TokenScope{ID: 2, Name: "ci-staging", ProjectSlug: "auditfy", StageSlug: "staging"}},
			tokBeta:    {Token: &entity.TokenScope{ID: 3, Name: "ci-beta", ProjectSlug: "beta"}},
		},
	}
	return &env{c: httptestutil.New(t, auth, func(k *utils.Kit) { m.RegisterRoutes(k) }), db: db}
}

var s = httptestutil.WithSession(session)

func (e *env) seedStage(t *testing.T) {
	t.Helper()
	httptestutil.Expect(t, e.c.Do("POST", "/api/v1/projects", map[string]string{"slug": "auditfy", "name": "Auditfy"}, s), http.StatusCreated)
	httptestutil.Expect(t, e.c.Do("POST", "/api/v1/projects/auditfy/stages", map[string]string{"slug": "staging"}, s), http.StatusCreated)
}

func TestProjectsAndStagesAPI(t *testing.T) {
	e := newEnv(t)
	c := e.c
	expect := func(w *httptest.ResponseRecorder, status int) { t.Helper(); httptestutil.Expect(t, w, status) }

	expect(c.Do("GET", "/api/v1/projects", nil), http.StatusUnauthorized)

	expect(c.Do("POST", "/api/v1/projects", map[string]string{"slug": "Auditfy", "name": "Auditfy"}, s), http.StatusUnprocessableEntity)
	expect(c.Do("POST", "/api/v1/projects", map[string]string{"slug": "auditfy", "name": ""}, s), http.StatusUnprocessableEntity)
	w := c.Do("POST", "/api/v1/projects", map[string]string{"slug": "auditfy", "name": "Auditfy"}, s)
	expect(w, http.StatusCreated)
	if p := httptestutil.Decode[map[string]any](t, w); p["slug"] != "auditfy" || p["id"] != nil {
		t.Fatalf("project=%v (el id interno no se expone)", p)
	}
	expect(c.Do("POST", "/api/v1/projects", map[string]string{"slug": "auditfy", "name": "X"}, s), http.StatusConflict)

	w = c.Do("GET", "/api/v1/projects", nil, s)
	expect(w, http.StatusOK)
	if list := httptestutil.Decode[[]map[string]any](t, w); len(list) != 1 {
		t.Fatalf("list=%v", list)
	}
	w = c.Do("PATCH", "/api/v1/projects/auditfy", map[string]string{"name": "Auditfy SpA"}, s)
	expect(w, http.StatusOK)
	if p := httptestutil.Decode[map[string]any](t, w); p["name"] != "Auditfy SpA" {
		t.Fatalf("project=%v", p)
	}
	expect(c.Do("GET", "/api/v1/projects/nope", nil, s), http.StatusNotFound)

	// Stages
	expect(c.Do("POST", "/api/v1/projects/auditfy/stages", map[string]string{"slug": "staging_1"}, s), http.StatusUnprocessableEntity)
	w = c.Do("POST", "/api/v1/projects/auditfy/stages", map[string]string{"slug": "staging"}, s)
	expect(w, http.StatusCreated)
	if st := httptestutil.Decode[map[string]any](t, w); st["project"] != "auditfy" || st["slug"] != "staging" {
		t.Fatalf("stage=%v", st)
	}
	expect(c.Do("POST", "/api/v1/projects/nope/stages", map[string]string{"slug": "staging"}, s), http.StatusNotFound)
	expect(c.Do("GET", "/api/v1/projects/auditfy/stages", nil, s), http.StatusOK)
	expect(c.Do("GET", "/api/v1/projects/auditfy/stages/staging", nil, s), http.StatusOK)

	// No se borra un proyecto con stages.
	w = c.Do("DELETE", "/api/v1/projects/auditfy", nil, s)
	expect(w, http.StatusConflict)
	if code := httptestutil.ErrorCode(t, w); code != "not_empty" {
		t.Fatalf("code=%s", code)
	}
	expect(c.Do("DELETE", "/api/v1/projects/auditfy/stages/staging", nil, s), http.StatusNoContent)
	expect(c.Do("DELETE", "/api/v1/projects/auditfy", nil, s), http.StatusNoContent)

	// Todo quedó en la auditoría.
	want := []string{"project.delete", "stage.delete", "stage.create", "project.update", "project.create"}
	rows, err := e.db.Conn(context.Background()).QueryContext(context.Background(),
		`SELECT actor_type, action FROM audit_events ORDER BY id DESC`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	i := 0
	for rows.Next() {
		var actor, action string
		if err := rows.Scan(&actor, &action); err != nil {
			t.Fatal(err)
		}
		if i >= len(want) || action != want[i] || actor != "user" {
			t.Errorf("evento %d = %s/%s, esperaba user/%v", i, actor, action, want)
		}
		i++
	}
	if i != len(want) {
		t.Fatalf("eventos = %d, esperaba %d", i, len(want))
	}
}

const appsPath = "/api/v1/projects/auditfy/stages/staging/apps"

func TestCreateServiceApp(t *testing.T) {
	e := newEnv(t)
	e.seedStage(t)
	c := e.c
	expect := func(w *httptest.ResponseRecorder, status int) { t.Helper(); httptestutil.Expect(t, w, status) }

	w := c.Do("POST", appsPath, map[string]any{
		"slug": "api", "kind": "service",
		"spec": map[string]any{"image": "ghcr.io/aumi-app/backend-app:staging", "port": 3000,
			"healthcheck": map[string]any{"type": "http", "path": "/health"}},
	}, s)
	expect(w, http.StatusCreated)
	app := httptestutil.Decode[map[string]any](t, w)
	spec := app["spec"].(map[string]any)
	if app["kind"] != "service" || spec["replicas"] != float64(1) || app["pending_changes"] != false || app["deploy_timeout_s"] != float64(300) {
		t.Fatalf("app=%v", app)
	}
	if spec["healthcheck"].(map[string]any)["interval_s"] != float64(10) {
		t.Fatalf("el spec guardado debe venir normalizado: %v", spec)
	}

	ok := map[string]any{"image": "x", "port": 1}
	expect(c.Do("POST", appsPath, map[string]any{"slug": "api", "kind": "service", "spec": ok}, s), http.StatusConflict)
	expect(c.Do("POST", appsPath, map[string]any{"slug": "bad", "kind": "service",
		"spec": map[string]any{"image": "x", "port": 0}}, s), http.StatusUnprocessableEntity)
	expect(c.Do("POST", appsPath, map[string]any{"slug": "bad", "kind": "service"}, s), http.StatusUnprocessableEntity)
	expect(c.Do("POST", appsPath, map[string]any{"slug": "bad", "kind": "service", "compose_yaml": "x", "spec": ok}, s), http.StatusUnprocessableEntity)
	expect(c.Do("POST", appsPath, map[string]any{"slug": "bad", "kind": "lambda"}, s), http.StatusUnprocessableEntity)
	expect(c.Do("POST", "/api/v1/projects/auditfy/stages/nope/apps", map[string]any{"slug": "api", "kind": "service", "spec": ok}, s), http.StatusNotFound)

	// JSON estricto: campos desconocidos y cuerpos inválidos son 400.
	w = c.Do("POST", appsPath, map[string]any{"slug": "z", "kind": "service", "spec": ok, "extra": 1}, s)
	expect(w, http.StatusBadRequest)
	if code := httptestutil.ErrorCode(t, w); code != "invalid_json" {
		t.Fatalf("code=%s", code)
	}
	expect(c.Do("POST", appsPath, `{"slug":"z","kind":"service","spec":{"image":"x","image":"y","port":1}}`, s), http.StatusBadRequest)
	// Un spec con un campo desconocido llega al caso de uso: 422 invalid.
	w = c.Do("POST", appsPath, `{"slug":"z","kind":"service","spec":{"image":"x","port":1,"foo":1}}`, s)
	expect(w, http.StatusUnprocessableEntity)
	// Un healthcheck con metacaracteres de shell se rechaza.
	expect(c.Do("POST", appsPath, map[string]any{"slug": "z", "kind": "service",
		"spec": map[string]any{"image": "x", "port": 1, "healthcheck": map[string]any{"type": "http", "path": "/h;id"}}}, s), http.StatusUnprocessableEntity)
	expect(c.Do("POST", appsPath, map[string]any{"slug": "z", "kind": "service",
		"spec": map[string]any{"image": "${X}", "port": 1}}, s), http.StatusUnprocessableEntity)
}

func TestCreateComposeApp(t *testing.T) {
	e := newEnv(t)
	e.seedStage(t)
	c := e.c
	expect := func(w *httptest.ResponseRecorder, status int) { t.Helper(); httptestutil.Expect(t, w, status) }

	w := c.Do("POST", appsPath, map[string]any{"slug": "nats", "kind": "compose",
		"compose_yaml": "services:\n  nats:\n    image: nats:2\n", "deploy_timeout_s": 60}, s)
	expect(w, http.StatusCreated)
	if app := httptestutil.Decode[map[string]any](t, w); app["compose_yaml"] == "" || app["spec"] != nil || app["deploy_timeout_s"] != float64(60) {
		t.Fatalf("app=%v", app)
	}
	expect(c.Do("POST", appsPath, map[string]any{"slug": "x", "kind": "compose", "compose_yaml": "  "}, s), http.StatusUnprocessableEntity)
	expect(c.Do("POST", appsPath, map[string]any{"slug": "x", "kind": "compose", "compose_yaml": "a: 1",
		"spec": map[string]any{"image": "x"}}, s), http.StatusUnprocessableEntity)
	expect(c.Do("POST", appsPath, map[string]any{"slug": "x", "kind": "compose", "compose_yaml": "a: 1",
		"deploy_timeout_s": 5}, s), http.StatusUnprocessableEntity)
	expect(c.Do("POST", appsPath, map[string]any{"slug": "x", "kind": "compose",
		"compose_yaml": strings.Repeat("a", 256<<10+1)}, s), http.StatusUnprocessableEntity)
}

func TestUpdateListDeleteApp(t *testing.T) {
	e := newEnv(t)
	e.seedStage(t)
	c := e.c
	expect := func(w *httptest.ResponseRecorder, status int) { t.Helper(); httptestutil.Expect(t, w, status) }
	c.Do("POST", appsPath, map[string]any{"slug": "api", "kind": "service", "spec": map[string]any{"image": "x:1", "port": 80}}, s)
	c.Do("POST", appsPath, map[string]any{"slug": "nats", "kind": "compose", "compose_yaml": "a: 1"}, s)

	w := c.Do("PATCH", appsPath+"/api", map[string]any{"spec": map[string]any{"image": "x:2", "port": 80}}, s)
	expect(w, http.StatusOK)
	if app := httptestutil.Decode[map[string]any](t, w); app["spec"].(map[string]any)["image"] != "x:2" {
		t.Fatalf("app=%v", app)
	}
	w = c.Do("PATCH", appsPath+"/api", map[string]any{"compose_yaml": "a: 1"}, s)
	expect(w, http.StatusUnprocessableEntity)
	if code := httptestutil.ErrorCode(t, w); code != "kind_mismatch" {
		t.Fatalf("code=%s", code)
	}
	expect(c.Do("PATCH", appsPath+"/api", map[string]any{"spec": map[string]any{"image": "x", "port": -1}}, s), http.StatusUnprocessableEntity)
	expect(c.Do("PATCH", appsPath+"/nope", map[string]any{"deploy_timeout_s": 60}, s), http.StatusNotFound)

	w = c.Do("GET", appsPath, nil, s)
	expect(w, http.StatusOK)
	if list := httptestutil.Decode[[]map[string]any](t, w); len(list) != 2 || list[0]["slug"] != "api" {
		t.Fatalf("list=%v", list)
	}
	expect(c.Do("DELETE", appsPath+"/nats", nil, s), http.StatusNoContent)
	expect(c.Do("GET", appsPath+"/nats", nil, s), http.StatusNotFound)
}

// Los GET aceptan tokens dentro de su alcance; lo demás exige sesión.
func TestTokenAccess(t *testing.T) {
	e := newEnv(t)
	e.seedStage(t)
	c := e.c
	httptestutil.Expect(t, c.Do("POST", "/api/v1/projects", map[string]string{"slug": "beta", "name": "Beta"}, s), http.StatusCreated)
	httptestutil.Expect(t, c.Do("POST", "/api/v1/projects/auditfy/stages", map[string]string{"slug": "production"}, s), http.StatusCreated)
	httptestutil.Expect(t, c.Do("POST", appsPath, map[string]any{"slug": "api", "kind": "service",
		"spec": map[string]any{"image": "x", "port": 80}}, s), http.StatusCreated)

	reads := []string{
		"/api/v1/projects/auditfy",
		"/api/v1/projects/auditfy/stages",
		"/api/v1/projects/auditfy/stages/staging",
		appsPath,
		appsPath + "/api",
	}
	for _, path := range reads {
		httptestutil.Expect(t, c.Do("GET", path, nil, httptestutil.WithBearer(tokAll)), http.StatusOK)
		httptestutil.Expect(t, c.Do("GET", path, nil, httptestutil.WithBearer(tokStaging)), http.StatusOK)
		// Un token de otro proyecto recibe 403.
		w := c.Do("GET", path, nil, httptestutil.WithBearer(tokBeta))
		httptestutil.Expect(t, w, http.StatusForbidden)
		if code := httptestutil.ErrorCode(t, w); code != "out_of_scope" {
			t.Fatalf("%s: code=%s", path, code)
		}
	}
	// Un token de un stage no ve otro stage del mismo proyecto.
	for _, path := range []string{
		"/api/v1/projects/auditfy/stages/production",
		"/api/v1/projects/auditfy/stages/production/apps",
	} {
		w := c.Do("GET", path, nil, httptestutil.WithBearer(tokStaging))
		httptestutil.Expect(t, w, http.StatusForbidden)
		if code := httptestutil.ErrorCode(t, w); code != "out_of_scope" {
			t.Fatalf("%s: code=%s", path, code)
		}
		httptestutil.Expect(t, c.Do("GET", path, nil, httptestutil.WithBearer(tokAll)), http.StatusOK)
	}

	// Escribir, y listar todos los proyectos, es solo de sesión: un token recibe 403 aunque sea del proyecto.
	writes := []struct {
		method, path string
		body         any
	}{
		{"GET", "/api/v1/projects", nil},
		{"POST", "/api/v1/projects", map[string]string{"slug": "gamma", "name": "G"}},
		{"PATCH", "/api/v1/projects/auditfy", map[string]string{"name": "X"}},
		{"DELETE", "/api/v1/projects/auditfy", nil},
		{"POST", "/api/v1/projects/auditfy/stages", map[string]string{"slug": "qa"}},
		{"DELETE", "/api/v1/projects/auditfy/stages/staging", nil},
		{"POST", appsPath, map[string]any{"slug": "x", "kind": "compose", "compose_yaml": "a: 1"}},
		{"PATCH", appsPath + "/api", map[string]any{"deploy_timeout_s": 60}},
		{"DELETE", appsPath + "/api", nil},
	}
	for _, wr := range writes {
		for _, tok := range []string{tokAll, tokStaging} {
			w := c.Do(wr.method, wr.path, wr.body, httptestutil.WithBearer(tok))
			httptestutil.Expect(t, w, http.StatusForbidden)
			if code := httptestutil.ErrorCode(t, w); code != "token_not_allowed" {
				t.Fatalf("%s %s: code=%s", wr.method, wr.path, code)
			}
		}
	}
	// Nada de eso cambió el catálogo.
	httptestutil.Expect(t, c.Do("GET", appsPath+"/api", nil, s), http.StatusOK)
}

func TestSessionRequired(t *testing.T) {
	e := newEnv(t)
	for _, path := range []string{"/api/v1/projects", "/api/v1/projects/auditfy", appsPath} {
		httptestutil.Expect(t, e.c.Do("GET", path, nil), http.StatusUnauthorized)
	}
	httptestutil.Expect(t, e.c.Do("POST", "/api/v1/projects", map[string]string{"slug": "a", "name": "A"}), http.StatusUnauthorized)
}
