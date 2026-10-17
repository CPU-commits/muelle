package deploy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	depsqlite "github.com/CPU-commits/muelle/src/internal/deploy/infra/out/repository/sqlite"
)

const (
	apiPath   = "/api/v1/projects/auditfy/stages/staging/apps/api"
	infraPath = "/api/v1/projects/auditfy/stages/staging/apps/infra"
	nadaPath  = "/api/v1/projects/auditfy/stages/staging/apps/nada"
)

// seed siembra por SQL (las tablas son de catalog) auditfy/staging con api
// (formulario) e infra (compose con web y worker), y otro/prod/web.
func (h *httpTest) seed(t *testing.T) (api, infra, other int64) {
	t.Helper()
	h.exec(t, `INSERT INTO projects (slug, name, created_at) VALUES ('auditfy', 'Auditfy', 0), ('otro', 'Otro', 0)`)
	h.exec(t, `INSERT INTO stages (project_id, slug, created_at) SELECT id, CASE slug WHEN 'auditfy' THEN 'staging' ELSE 'prod' END, 0 FROM projects`)
	api = h.exec(t, `INSERT INTO apps (stage_id, slug, kind, spec, config_changed_at, created_at)
		SELECT s.id, 'api', 'service', '{"image":"ghcr.io/x/api:1","port":3000}', 0, 0 FROM stages s WHERE s.slug = 'staging'`)
	infra = h.exec(t, `INSERT INTO apps (stage_id, slug, kind, compose_yaml, config_changed_at, created_at)
		SELECT s.id, 'infra', 'compose', 'services:
  web:
    image: ghcr.io/x/web:1
  worker:
    image: ghcr.io/x/worker:1
', 0, 0 FROM stages s WHERE s.slug = 'staging'`)
	other = h.exec(t, `INSERT INTO apps (stage_id, slug, kind, spec, config_changed_at, created_at)
		SELECT s.id, 'web', 'service', '{"image":"nginx","port":80}', 0, 0 FROM stages s WHERE s.slug = 'prod'`)
	return api, infra, other
}

type queuedJSON struct {
	ID        int64  `json:"id"`
	Status    string `json:"status"`
	StatusURL string `json:"status_url"`
}

func expectQueued(t *testing.T, w *httptest.ResponseRecorder, appPath string) queuedJSON {
	t.Helper()
	httptestutil.Expect(t, w, http.StatusAccepted)
	q := httptestutil.Decode[queuedJSON](t, w)
	want := fmt.Sprintf("%s/deploys/%d", appPath, q.ID)
	if q.ID <= 0 || q.Status != "queued" || q.StatusURL != want || w.Header().Get("Location") != want {
		t.Fatalf("202: %s (Location %q)", w.Body, w.Header().Get("Location"))
	}
	return q
}

// POST …/deploys (contrato §2.2): el cuerpo es opcional, y el orden de los
// errores es app (404), cuerpo (400) y validación (422).
func TestCreateDeployHTTP(t *testing.T) {
	h := newHTTPTest(t)
	h.seed(t)

	for _, body := range []any{nil, "", "  \n", "{}", map[string]string{"image": "ghcr.io/x/api:sha-3f9c2e1"}} {
		expectQueued(t, h.c.Do("POST", apiPath+"/deploys", body, session()), apiPath)
	}
	expectQueued(t, h.c.Do("POST", infraPath+"/deploys", map[string]any{"images": map[string]string{"web": "ghcr.io/x/web:2"}}, session()), infraPath)

	for _, c := range []struct {
		path    string
		body    any
		status  int
		code    string
		message string
	}{
		{apiPath, `{"x":1}`, 400, "invalid_json", `JSON inválido: el campo "x" no existe`},
		{apiPath, `[]`, 400, "invalid_json", ""},
		{apiPath, `{"image":`, 400, "invalid_json", ""},
		{apiPath, strings.Repeat(" ", 1<<20+1), 400, "invalid_json", "el cuerpo supera 1 MiB"},
		{apiPath, `{"image":"` + strings.Repeat("a", 1<<20) + `"}`, 400, "invalid_json", "el cuerpo supera 1 MiB"},
		{apiPath, `{"image":"ghcr.io/x/api:2","images":{}}`, 422, "invalid", "image: manda image o images, no los dos"},
		{infraPath, `{"image":"ghcr.io/x/web:2"}`, 422, "invalid", "image: una app compose recibe images, por servicio"},
		{apiPath, `{"image":"-mala"}`, 422, "invalid", "image: no es una referencia de imagen válida"},
		{apiPath, `{"images":{"web":"nginx"}}`, 422, "invalid", "images.web: el servicio no existe en la app"},
		{infraPath, `{"images":{"web":"Nginx"}}`, 422, "invalid", "images.web: no es una referencia válida de Docker"},
		// El 404 gana al 400.
		{nadaPath, nil, 404, "not_found", ""},
		{nadaPath, `{"x":`, 404, "not_found", ""},
		{nadaPath, `[]`, 404, "not_found", ""},
		{"/api/v1/projects/nada/stages/staging/apps/api", `{}`, 404, "not_found", ""},
	} {
		expectError(t, h.c.Do("POST", c.path+"/deploys", c.body, session()), c.status, c.code, c.message)
	}
}

// El rollback de una app que no existe es 404 aunque el cuerpo esté mal.
func TestRollbackHTTPOrder(t *testing.T) {
	h := newHTTPTest(t)
	h.seed(t)
	expectError(t, h.c.Do("POST", nadaPath+"/rollback", `[]`, session()), 404, "not_found", "")
	expectError(t, h.c.Do("POST", nadaPath+"/rollback", `{"x":`, session()), 404, "not_found", "")
}

// finish lleva un deploy queued a succeeded (o failed) con esas imágenes
// resueltas, pedidas tal cual.
func (h *httpTest) finish(t *testing.T, id int64, status entity.Status, images map[string]string) {
	t.Helper()
	h.finishWith(t, id, status, images, images)
}

// finishWith es finish con las refs pedidas aparte de las resueltas.
func (h *httpTest) finishWith(t *testing.T, id int64, status entity.Status, requested, resolved map[string]string) {
	t.Helper()
	ctx := context.Background()
	r := depsqlite.NewDeployRepository(h.db)
	if ok, err := r.Claim(ctx, id, h.clock.Now()); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := r.SetImages(ctx, id, requested, requested, resolved); err != nil {
		t.Fatal(err)
	}
	var msg *string
	if status == entity.StatusFailed {
		msg = new(string)
		*msg = "render: compose_yaml: algo"
	}
	if err := r.Finish(ctx, id, status, msg, h.clock.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
}

// GET …/deploys y …/deploys/{id} (contrato §2.1, §2.3 y §2.4): todos los
// campos con su tipo, null cuando corresponde.
func TestDeploysListAndGet(t *testing.T) {
	h := newHTTPTest(t)
	h.seed(t)
	w := h.c.Do("GET", apiPath+"/deploys", nil, httptestutil.WithBearer("tp"))
	httptestutil.Expect(t, w, http.StatusOK)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("sin deploys: %s", w.Body)
	}

	d1 := expectQueued(t, h.c.Do("POST", apiPath+"/deploys", nil, httptestutil.WithBearer("tp")), apiPath)
	h.clock.Advance(180 * time.Millisecond)
	ref := "ghcr.io/x/api:1@sha256:" + strings.Repeat("a", 64)
	h.finishWith(t, d1.ID, entity.StatusSucceeded, map[string]string{"api": "ghcr.io/x/api:1"}, map[string]string{"api": ref})
	d2 := expectQueued(t, h.c.Do("POST", apiPath+"/deploys", map[string]string{"image": "ghcr.io/x/api:2"}, session()), apiPath)
	d3 := expectQueued(t, h.c.Do("POST", apiPath+"/deploys", nil, session()), apiPath)
	x := expectQueued(t, h.c.Do("POST", infraPath+"/deploys", nil, session()), infraPath)

	w = h.c.Do("GET", apiPath+"/deploys", nil, httptestutil.WithBearer("tp"))
	httptestutil.Expect(t, w, http.StatusOK)
	list := httptestutil.Decode[[]map[string]any](t, w)
	if len(list) != 3 || list[0]["id"] != float64(d3.ID) || list[1]["id"] != float64(d2.ID) || list[2]["id"] != float64(d1.ID) {
		t.Fatalf("lista: %s", w.Body)
	}
	for _, d := range list {
		if len(d) != 13 {
			t.Fatalf("faltan o sobran campos (contrato §2.1): %v", d)
		}
	}
	// El succeeded: imágenes resueltas y las pedidas, el actor del token, las
	// tres fechas.
	want := map[string]any{
		"id": float64(d1.ID), "status": "succeeded", "trigger": "api", "images": map[string]any{"api": ref},
		"requested_images": map[string]any{"api": "ghcr.io/x/api:1"}, "error": nil, "superseded_by": nil,
		"actor_type": "token", "actor_id": float64(4), "actor_name": "github-actions",
		"created_at": "2026-10-10T14:19:49.12Z", "started_at": "2026-10-10T14:19:49.3Z", "finished_at": "2026-10-10T14:19:50.3Z",
	}
	assertJSON(t, list[2], want)
	// El reemplazado: superseded_by, finished_at y sin started_at; los
	// overrides en images mientras estuvo queued, y nada pedido (no llegó a
	// resolver).
	assertJSON(t, list[1], map[string]any{
		"id": float64(d2.ID), "status": "superseded", "trigger": "panel", "images": map[string]any{"api": "ghcr.io/x/api:2"},
		"requested_images": map[string]any{}, "error": nil, "superseded_by": float64(d3.ID),
		"actor_type": "user", "actor_id": float64(1), "actor_name": "felipe",
		"created_at": "2026-10-10T14:19:49.3Z", "started_at": nil, "finished_at": "2026-10-10T14:19:49.3Z",
	})
	// El queued: images y requested_images {} (nunca null).
	assertJSON(t, list[0], map[string]any{
		"id": float64(d3.ID), "status": "queued", "trigger": "panel", "images": map[string]any{},
		"requested_images": map[string]any{}, "error": nil, "superseded_by": nil,
		"actor_type": "user", "actor_id": float64(1), "actor_name": "felipe",
		"created_at": "2026-10-10T14:19:49.3Z", "started_at": nil, "finished_at": nil,
	})

	w = h.c.Do("GET", fmt.Sprintf("%s/deploys/%d", apiPath, d1.ID), nil, session())
	httptestutil.Expect(t, w, http.StatusOK)
	assertJSON(t, httptestutil.Decode[map[string]any](t, w), want)

	// Un failed trae su error.
	h.finish(t, d3.ID, entity.StatusFailed, map[string]string{})
	w = h.c.Do("GET", fmt.Sprintf("%s/deploys/%d", apiPath, d3.ID), nil, session())
	if got := httptestutil.Decode[map[string]any](t, w); got["status"] != "failed" || got["error"] != "render: compose_yaml: algo" {
		t.Fatalf("failed: %s", w.Body)
	}

	// Un id de otra app, uno que no existe o uno que no es un número: 404.
	for _, id := range []string{fmt.Sprint(x.ID), "99999", "abc", "-1", "0", "1.5"} {
		expectError(t, h.c.Do("GET", apiPath+"/deploys/"+id, nil, session()), 404, "not_found", "")
	}
	expectError(t, h.c.Do("GET", nadaPath+"/deploys", nil, session()), 404, "not_found", "")
}

func assertJSON(t *testing.T, got, want map[string]any) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("JSON:\n got %v\nwant %v", got, want)
	}
}

type logJSON struct {
	Chunk        string `json:"chunk"`
	NextOffset   int    `json:"next_offset"`
	Status       string `json:"status"`
	SupersededBy *int64 `json:"superseded_by"`
}

// El polling del log (contrato §2.5).
func TestLogPolling(t *testing.T) {
	h := newHTTPTest(t)
	h.seed(t)
	d := expectQueued(t, h.c.Do("POST", apiPath+"/deploys", nil, session()), apiPath)
	logPath := fmt.Sprintf("%s/deploys/%d/log", apiPath, d.ID)
	r := depsqlite.NewDeployRepository(h.db)
	ctx := context.Background()
	line := func(text string) string { return service.FormatLine(service.Masker{}, h.clock.Now(), text) }
	l1, l2 := line("deploy #1 de auditfy/staging/api (panel · felipe)"), line("resolviendo imágenes")
	if err := r.AppendLog(ctx, d.ID, []string{l1, l2}, false, ""); err != nil {
		t.Fatal(err)
	}

	read := func(query string) logJSON {
		t.Helper()
		w := h.c.Do("GET", logPath+query, nil, httptestutil.WithBearer("tp"))
		httptestutil.Expect(t, w, http.StatusOK)
		if body := w.Body.String(); !strings.Contains(body, `"superseded_by":`) {
			t.Fatalf("falta superseded_by: %s", body)
		}
		return httptestutil.Decode[logJSON](t, w)
	}
	if got := read(""); got.Chunk != l1+l2 || got.NextOffset != len(l1+l2) || got.Status != "queued" || got.SupersededBy != nil {
		t.Fatalf("sin offset: %+v", got)
	}
	if got := read(fmt.Sprintf("?offset=%d", len(l1))); got.Chunk != l2 || got.NextOffset != len(l1+l2) {
		t.Fatalf("desde l1: %+v", got)
	}
	// Al final: chunk "" y next_offset igual.
	if got := read(fmt.Sprintf("?offset=%d", len(l1+l2))); got.Chunk != "" || got.NextOffset != len(l1+l2) {
		t.Fatalf("al final: %+v", got)
	}

	for _, q := range []string{"?offset=-1", "?offset=abc", "?offset=", "?offset=1.5", "?offset=+3", "?offset=99999999999999999999"} {
		expectError(t, h.c.Do("GET", logPath+q, nil, session()), 422, "invalid", "offset: tiene que ser un entero ≥ 0")
	}
	for _, offset := range []string{fmt.Sprint(len(l1+l2) + 1), "9223372036854775807"} {
		expectError(t, h.c.Do("GET", logPath+"?offset="+offset, nil, session()), 422, "invalid", "offset: pasa el largo del log")
	}
	// El 404 gana al 422.
	expectError(t, h.c.Do("GET", apiPath+"/deploys/99999/log?offset=abc", nil, session()), 404, "not_found", "")

	// Superseded: el estado y quién lo reemplazó; el cliente termina con un
	// estado final y chunk "".
	d2 := expectQueued(t, h.c.Do("POST", apiPath+"/deploys", nil, session()), apiPath)
	got := read(fmt.Sprintf("?offset=%d", len(l1+l2)))
	if got.Status != "superseded" || got.SupersededBy == nil || *got.SupersededBy != d2.ID || got.Chunk != "" {
		t.Fatalf("superseded: %+v", got)
	}
}

// POST …/rollback (contrato §2.7): sin cuerpo (como lo manda el panel), {} o
// espacios; otra cosa es 400.
func TestRollbackHTTP(t *testing.T) {
	h := newHTTPTest(t)
	h.seed(t)
	expectError(t, h.c.Do("POST", apiPath+"/rollback", nil, session()), 409, "no_previous_deploy", "no hay un deploy exitoso anterior al actual")

	a := "ghcr.io/x/api:1@sha256:" + strings.Repeat("a", 64)
	d1 := expectQueued(t, h.c.Do("POST", apiPath+"/deploys", nil, session()), apiPath)
	h.finish(t, d1.ID, entity.StatusSucceeded, map[string]string{"api": a})
	expectError(t, h.c.Do("POST", apiPath+"/rollback", nil, session()), 409, "no_previous_deploy", "")
	d2 := expectQueued(t, h.c.Do("POST", apiPath+"/deploys", nil, session()), apiPath)
	h.finish(t, d2.ID, entity.StatusSucceeded, map[string]string{"api": "ghcr.io/x/api:1@sha256:" + strings.Repeat("b", 64)})

	for _, body := range []any{nil, "{}", "  \n"} {
		q := expectQueued(t, h.c.Do("POST", apiPath+"/rollback", body, httptestutil.WithBearer("tp")), apiPath)
		w := h.c.Do("GET", fmt.Sprintf("%s/deploys/%d", apiPath, q.ID), nil, session())
		got := httptestutil.Decode[map[string]any](t, w)
		if got["trigger"] != "rollback" || fmt.Sprint(got["images"]) != fmt.Sprint(map[string]any{"api": a}) || got["actor_name"] != "github-actions" {
			t.Fatalf("rollback: %s", w.Body)
		}
	}
	for _, body := range []string{`{"x":1}`, `[]`, `null`, strings.Repeat("\n", 1<<20+1)} {
		expectError(t, h.c.Do("POST", apiPath+"/rollback", body, session()), 400, "invalid_json", "")
	}
}

// Los tokens (contrato §0.1): el del proyecto crea deploys y los lee; el de
// otro proyecto, 403 out_of_scope.
func TestTokenScopes(t *testing.T) {
	h := newHTTPTest(t)
	h.seed(t)
	q := expectQueued(t, h.c.Do("POST", apiPath+"/deploys", nil, httptestutil.WithBearer("tp")), apiPath)
	for _, path := range []string{apiPath + "/deploys", fmt.Sprintf("%s/deploys/%d", apiPath, q.ID), fmt.Sprintf("%s/deploys/%d/log", apiPath, q.ID)} {
		httptestutil.Expect(t, h.c.Do("GET", path, nil, httptestutil.WithBearer("tp")), http.StatusOK)
		expectError(t, h.c.Do("GET", path, nil, httptestutil.WithBearer("tx")), 403, "out_of_scope", "")
	}
	for _, path := range []string{apiPath + "/deploys", apiPath + "/rollback"} {
		expectError(t, h.c.Do("POST", path, nil, httptestutil.WithBearer("tx")), 403, "out_of_scope", "")
	}
	// Y el de otro proyecto sí despliega en el suyo.
	expectQueued(t, h.c.Do("POST", "/api/v1/projects/otro/stages/prod/apps/web/deploys", nil, httptestutil.WithBearer("tx")),
		"/api/v1/projects/otro/stages/prod/apps/web")
}

// Mientras una app se borra (RemoveApp, contrato §1.1), POST …/deploys y
// …/rollback son 409 conflict; las otras apps siguen, y el bloqueo vence a
// los 2 min (lo que tarda catalog en borrar la fila).
func TestRemoveAppBlocksDeploysHTTP(t *testing.T) {
	h := newHTTPTest(t)
	h.seed(t)
	if err := h.m.RemoveApp(context.Background(), "auditfy", "staging", "api"); err != nil {
		t.Fatal(err)
	}
	expectError(t, h.c.Do("POST", apiPath+"/deploys", nil, session()), http.StatusConflict, "conflict", "la app se está borrando")
	expectError(t, h.c.Do("POST", apiPath+"/rollback", nil, session()), http.StatusConflict, "conflict", "la app se está borrando")
	expectQueued(t, h.c.Do("POST", infraPath+"/deploys", nil, session()), infraPath)
	h.clock.Advance(2 * time.Minute)
	expectQueued(t, h.c.Do("POST", apiPath+"/deploys", nil, session()), apiPath)
}

// ErrWorkerBusy (RemoveApp no pudo detener el worker) es 409 conflict con su
// mensaje: el Kit es uno solo, así que el DELETE de catalog lo responde igual.
func TestWorkerBusyIsConflict(t *testing.T) {
	m, _ := newTestModule(t)
	c := httptestutil.New(t, testAuth, func(k *utils.Kit) {
		m.RegisterRoutes(k)
		k.Handle("DELETE /api/v1/prueba", utils.AccessSession, func(w http.ResponseWriter, r *http.Request) {
			k.Fail(w, r, fmt.Errorf("deploy: %w", service.ErrWorkerBusy))
		})
	})
	expectError(t, c.Do("DELETE", "/api/v1/prueba", nil, session()), http.StatusConflict, "conflict",
		"un deploy de la app todavía no se detiene; vuelve a intentar en unos segundos")
}
