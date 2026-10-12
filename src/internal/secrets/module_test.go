package secrets

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/internal/secrets/seedtest"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

const (
	appsPath = "/api/v1/projects/auditfy/stages/staging/apps"
	appEnv   = appsPath + "/api/env"
	stageEnv = "/api/v1/projects/auditfy/stages/staging/env"
	prodEnv  = "/api/v1/projects/auditfy/stages/production/env"
	prodApp  = "/api/v1/projects/auditfy/stages/production/apps/api/env"
)

type testEnv struct {
	db     *sqlite.DB
	client *httptestutil.Client
	clock  *clock.Manual
}

var session = httptestutil.WithSession("s")

// newTestEnv arma el módulo con la base, el cifrado y la auditoría reales, y
// siembra auditfy/staging/api y auditfy/production/api.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	db := sqlitetest.New(t)
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	clk := clock.NewManual(seedtest.T0.Add(time.Hour))
	log := slog.New(slog.NewTextHandler(t.Output(), nil))
	seedtest.Auditfy(t, db)
	seedtest.App(t, db, seedtest.Stage(t, db, seedtest.ProjectID(t, db, "auditfy"), "production"), "api")
	seedtest.Stage(t, db, seedtest.Project(t, db, "otro"), "staging")

	m := New(Deps{DB: db, Cipher: c, Clock: clk, Audit: sqlite.NewAuditRecorder(db, clk, log), Log: log})
	auth := httptestutil.FakeAuth{
		Sessions: map[string]entity.Principal{"s": {UserID: 1, Username: "felipe"}},
		Tokens: map[string]entity.Principal{
			"staging": {Token: &entity.TokenScope{ID: 7, Name: "ci-staging", ProjectSlug: "auditfy", StageSlug: "staging"}},
			"project": {Token: &entity.TokenScope{ID: 8, Name: "ci", ProjectSlug: "auditfy"}},
			"otro":    {Token: &entity.TokenScope{ID: 9, Name: "ci-otro", ProjectSlug: "otro"}},
		},
	}
	return &testEnv{db: db, client: httptestutil.New(t, auth, m.RegisterRoutes), clock: clk}
}

type auditEvent struct {
	actorType, action, target, details string
}

// audit devuelve los eventos del más nuevo al más viejo.
func (e *testEnv) audit(t *testing.T) []auditEvent {
	t.Helper()
	ctx := context.Background()
	rows, err := e.db.Conn(ctx).QueryContext(ctx, `SELECT actor_type, action, target, details FROM audit_events ORDER BY id DESC`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []auditEvent
	for rows.Next() {
		var ev auditEvent
		if err := rows.Scan(&ev.actorType, &ev.action, &ev.target, &ev.details); err != nil {
			t.Fatal(err)
		}
		out = append(out, ev)
	}
	return out
}

// Portado de TestEnvAPI (Tarea 21 ref).
func TestEnvAPI(t *testing.T) {
	e := newTestEnv(t)
	c := e.client

	w := c.Do("PUT", appEnv, map[string]any{"vars": map[string]string{"DATABASE_URL": "postgres://x", "JWT_KEY": "k"}}, session)
	httptestutil.Expect(t, w, http.StatusNoContent)
	w = c.Do("PATCH", appEnv, map[string]any{"set": map[string]string{"LOG_LEVEL": "debug"}, "secret": false, "unset": []string{"JWT_KEY"}}, session)
	httptestutil.Expect(t, w, http.StatusNoContent)

	w = c.Do("GET", appEnv, nil, session)
	httptestutil.Expect(t, w, http.StatusOK)
	body := w.Body.String()
	if strings.Contains(body, "postgres://x") {
		t.Fatalf("un secreto apareció en la respuesta: %s", body)
	}
	vars := httptestutil.Decode[map[string][]map[string]any](t, w)["vars"]
	if len(vars) != 2 || vars[0]["key"] != "DATABASE_URL" || vars[0]["value"] != nil || vars[1]["value"] != "debug" {
		t.Fatalf("vars=%v", vars)
	}

	httptestutil.Expect(t, c.Do("PUT", appEnv, map[string]any{}, session), http.StatusUnprocessableEntity)
	httptestutil.Expect(t, c.Do("PUT", appEnv, map[string]any{"vars": map[string]string{"bad-key": "x"}}, session), http.StatusUnprocessableEntity)
	httptestutil.Expect(t, c.Do("PUT", stageEnv, map[string]any{"vars": map[string]string{"NATS_SERVERS": "nats://nats:4222"}, "secret": false}, session), http.StatusNoContent)
	httptestutil.Expect(t, c.Do("GET", appsPath+"/nope/env", nil, session), http.StatusNotFound)

	// La auditoría guarda claves, nunca valores.
	events := e.audit(t)
	for _, ev := range events {
		if strings.Contains(ev.details, "postgres://x") || strings.Contains(ev.details, "debug") {
			t.Fatalf("valor en la auditoría: %s", ev.details)
		}
	}
	if len(events) != 3 {
		t.Fatalf("events=%+v", events)
	}
	if events[1].action != "env.patch" || events[1].details != `{"set":["LOG_LEVEL"],"unset":["JWT_KEY"]}` ||
		events[1].target != "app:auditfy/staging/api" || events[1].actorType != "user" {
		t.Fatalf("evento=%+v", events[1])
	}
	if events[0].action != "env.replace" || events[0].target != "stage:auditfy/staging" || events[0].details != `{"keys":["NATS_SERVERS"]}` {
		t.Fatalf("evento=%+v", events[0])
	}
}

// La forma exacta de la respuesta de GET: value null en los secretos, updated_at en RFC 3339.
func TestEnvAPIResponseShape(t *testing.T) {
	e := newTestEnv(t)
	c := e.client
	w := c.Do("GET", stageEnv, nil, session)
	httptestutil.Expect(t, w, http.StatusOK)
	if got := strings.TrimSpace(w.Body.String()); got != `{"vars":[]}` {
		t.Fatalf("sin variables: %s", got)
	}

	httptestutil.Expect(t, c.Do("PUT", stageEnv, map[string]any{"vars": map[string]string{"TOKEN": "s3cr3t"}}, session), http.StatusNoContent)
	w = c.Do("GET", stageEnv, nil, session)
	httptestutil.Expect(t, w, http.StatusOK)
	want := `{"vars":[{"key":"TOKEN","value":null,"secret":true,"updated_at":"2026-10-10T13:00:00Z"}]}`
	if got := strings.TrimSpace(w.Body.String()); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}

	// Una no secreta muestra su valor (también el vacío) y "secret":false; nada se omite.
	e.clock.Advance(time.Minute)
	httptestutil.Expect(t, c.Do("PATCH", stageEnv, map[string]any{"set": map[string]string{"EMPTY": ""}, "secret": false}, session), http.StatusNoContent)
	w = c.Do("GET", stageEnv, nil, session)
	httptestutil.Expect(t, w, http.StatusOK)
	want = `{"vars":[{"key":"EMPTY","value":"","secret":false,"updated_at":"2026-10-10T13:01:00Z"},` +
		`{"key":"TOKEN","value":null,"secret":true,"updated_at":"2026-10-10T13:00:00Z"}]}`
	if got := strings.TrimSpace(w.Body.String()); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestEnvAPIErrors(t *testing.T) {
	e := newTestEnv(t)
	c := e.client
	cases := []struct {
		name   string
		method string
		path   string
		body   any
		status int
		code   string
	}{
		{"JSON inválido", "PUT", appEnv, `{"vars":`, 400, "invalid_json"},
		{"campo desconocido", "PUT", appEnv, `{"vars":{},"extra":1}`, 400, "invalid_json"},
		{"clave repetida", "PUT", appEnv, `{"vars":{"A":"1","A":"2"}}`, 400, "invalid_json"},
		{"vars no es un objeto", "PUT", appEnv, `{"vars":["A"]}`, 400, "invalid_json"},
		{"un valor no es texto", "PUT", appEnv, `{"vars":{"A":1}}`, 400, "invalid_json"},
		{"PATCH con un campo de PUT", "PATCH", appEnv, `{"vars":{"A":"1"}}`, 400, "invalid_json"},
		{"vars null", "PUT", appEnv, `{"vars":null}`, 422, "invalid"},
		{"prefijo COMPOSE_", "PUT", stageEnv, map[string]any{"vars": map[string]string{"COMPOSE_FILE": "x"}}, 422, "invalid"},
		{"set y unset a la vez", "PATCH", appEnv, map[string]any{"set": map[string]string{"A": "1"}, "unset": []string{"A"}}, 422, "invalid"},
		{"unset inválido", "PATCH", stageEnv, map[string]any{"unset": []string{"bad-key"}}, 422, "invalid"},
		{"PATCH de un stage inexistente", "PATCH", "/api/v1/projects/auditfy/stages/nope/env", map[string]any{"set": map[string]string{"A": "1"}}, 404, "not_found"},
		{"PUT de una app inexistente", "PUT", appsPath + "/nope/env", map[string]any{"vars": map[string]string{"A": "1"}}, 404, "not_found"},
		{"GET de un proyecto inexistente", "GET", "/api/v1/projects/nope/stages/staging/env", nil, 404, "not_found"},
		// json/v2 rechaza el UTF-8 inválido y los surrogates sueltos al leer el cuerpo.
		{"valor con UTF-8 inválido", "PUT", appEnv, "{\"vars\":{\"A\":\"\xff\"}}", 400, "invalid_json"},
		{"valor con un surrogate suelto", "PATCH", appEnv, `{"set":{"A":"\ud800"}}`, 400, "invalid_json"},
		// El NUL es JSON válido, pero no puede ir en el entorno de un proceso.
		{"valor con NUL", "PUT", appEnv, `{"vars":{"A":"a\u0000b"}}`, 422, "invalid"},
		{"PATCH vacío", "PATCH", appEnv, `{}`, 422, "invalid"},
		// 1 MiB es el límite del spec (§10) para todos los cuerpos.
		{"cuerpo de más de 1 MiB", "PUT", appEnv, `{"vars":{"BIG":"` + strings.Repeat("x", 1<<20) + `"}}`, 400, "invalid_json"},
	}
	for _, tc := range cases {
		w := c.Do(tc.method, tc.path, tc.body, session)
		if w.Code != tc.status || httptestutil.ErrorCode(t, w) != tc.code {
			t.Errorf("%s: %d %s, esperaba %d %s", tc.name, w.Code, w.Body.String(), tc.status, tc.code)
		}
	}

	// Sin sesión ni token.
	w := c.Do("GET", appEnv, nil)
	if w.Code != http.StatusUnauthorized || httptestutil.ErrorCode(t, w) != "unauthenticated" {
		t.Errorf("sin sesión: %d %s", w.Code, w.Body.String())
	}
	if n := seedtest.Count(t, e.db, `SELECT COUNT(*) FROM env_vars`); n != 0 {
		t.Fatalf("las peticiones fallidas dejaron %d variables", n)
	}
	if events := e.audit(t); len(events) != 0 {
		t.Fatalf("las peticiones fallidas no se auditan: %+v", events)
	}
}

// Un body null o sin vars nunca borra las variables: el PUT las reemplazaría
// todas por nada. Lo mismo un PATCH null, que no haría nada y marcaría las apps.
func TestEnvAPINullBodyKeepsVars(t *testing.T) {
	e := newTestEnv(t)
	c := e.client
	httptestutil.Expect(t, c.Do("PUT", appEnv, map[string]any{"vars": map[string]string{"A": "1"}, "secret": false}, session), http.StatusNoContent)

	for _, tc := range []struct {
		method, body string
		status       int
		code         string
	}{
		// DecodeJSON solo acepta objetos: un null ni siquiera llega al caso de uso.
		{"PUT", "null", 400, "invalid_json"},
		{"PATCH", "null", 400, "invalid_json"},
		{"PUT", "[]", 400, "invalid_json"},
		// Un objeto sin vars, o un PATCH vacío, lo rechaza el caso de uso.
		{"PUT", "{}", 422, "invalid"},
		{"PUT", `{"secret":false}`, 422, "invalid"},
		{"PUT", `{"vars":null}`, 422, "invalid"},
		{"PATCH", "{}", 422, "invalid"},
		{"PATCH", `{"set":null,"unset":null}`, 422, "invalid"},
		{"PATCH", `{"set":{},"unset":[]}`, 422, "invalid"},
	} {
		w := c.Do(tc.method, appEnv, tc.body, session)
		if w.Code != tc.status || httptestutil.ErrorCode(t, w) != tc.code {
			t.Errorf("%s %s: %d %s, esperaba %d %s", tc.method, tc.body, w.Code, w.Body.String(), tc.status, tc.code)
		}
	}

	w := c.Do("GET", appEnv, nil, session)
	httptestutil.Expect(t, w, http.StatusOK)
	vars := httptestutil.Decode[map[string][]map[string]any](t, w)["vars"]
	if len(vars) != 1 || vars[0]["key"] != "A" || vars[0]["value"] != "1" {
		t.Fatalf("las variables cambiaron: %v", vars)
	}
	if events := e.audit(t); len(events) != 1 {
		t.Fatalf("solo el primer PUT se audita: %+v", events)
	}
}

// Un valor que no descifra (copiado a otra fila, otra clave maestra o
// alterado) da un 500 genérico: ni el valor ni el error interno llegan al cliente.
func TestEnvAPIDecryptFailure(t *testing.T) {
	e := newTestEnv(t)
	c := e.client
	ctx := context.Background()
	httptestutil.Expect(t, c.Do("PUT", appEnv, map[string]any{"vars": map[string]string{"DATABASE_URL": "postgres://x"}, "secret": false}, session), http.StatusNoContent)

	// Mover la fila de la app a su stage: la AAD ya no coincide.
	stageID := seedtest.StageID(t, e.db, "auditfy", "staging")
	if _, err := e.db.Conn(ctx).ExecContext(ctx, `UPDATE env_vars SET stage_id = ?, app_id = NULL WHERE key = 'DATABASE_URL'`, stageID); err != nil {
		t.Fatal(err)
	}

	w := c.Do("GET", stageEnv, nil, session)
	if w.Code != http.StatusInternalServerError || httptestutil.ErrorCode(t, w) != "internal" {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	for _, leak := range []string{"postgres://x", "DATABASE_URL", "descifr"} {
		if strings.Contains(w.Body.String(), leak) {
			t.Fatalf("la respuesta filtró %q: %s", leak, w.Body.String())
		}
	}
	// La app quedó sin variables y se sigue leyendo bien.
	w = c.Do("GET", appEnv, nil, session)
	httptestutil.Expect(t, w, http.StatusOK)
	if got := strings.TrimSpace(w.Body.String()); got != `{"vars":[]}` {
		t.Fatalf("app: %s", got)
	}
}

// Los límites también se ven por la API: unset de más, y el total de 1 MiB
// por dueño acumulado con varios PATCH, cada uno bajo el límite del cuerpo.
func TestEnvAPILimits(t *testing.T) {
	e := newTestEnv(t)
	c := e.client
	unset := make([]string, 501)
	for i := range unset {
		unset[i] = fmt.Sprintf("K%d", i)
	}
	w := c.Do("PATCH", appEnv, map[string]any{"unset": unset}, session)
	if w.Code != http.StatusUnprocessableEntity || httptestutil.ErrorCode(t, w) != "invalid" || !strings.Contains(w.Body.String(), "unset") {
		t.Fatalf("unset de más: %d %s", w.Code, w.Body.String())
	}

	big := strings.Repeat("x", 64<<10)
	vars := map[string]string{}
	for i := range 15 {
		vars[fmt.Sprintf("K%02d", i)] = big
	}
	httptestutil.Expect(t, c.Do("PUT", appEnv, map[string]any{"vars": vars}, session), http.StatusNoContent)
	w = c.Do("PATCH", appEnv, map[string]any{"set": map[string]string{"MORE": big}}, session)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "1 MiB") {
		t.Fatalf("superar 1 MiB por dueño: %d %s", w.Code, w.Body.String())
	}
	if events := e.audit(t); len(events) != 1 {
		t.Fatalf("solo el PUT se audita: %+v", events)
	}
}

// Un PATCH que no cambia nada (unset de claves que no existen, o set con el
// mismo valor y secret) responde 204, pero no marca apps ni deja auditoría.
func TestEnvAPINoopPatch(t *testing.T) {
	e := newTestEnv(t)
	c := e.client
	appID := seedtest.AppID(t, e.db, "auditfy", "staging", "api")
	httptestutil.Expect(t, c.Do("PATCH", stageEnv, map[string]any{"unset": []string{"NOPE"}}, session), http.StatusNoContent)
	httptestutil.Expect(t, c.Do("PATCH", appEnv, map[string]any{"unset": []string{"NOPE"}}, session), http.StatusNoContent)
	if !seedtest.ConfigChangedAt(t, e.db, appID).Equal(seedtest.T0) {
		t.Fatal("un PATCH sin cambios no marca la app")
	}
	if events := e.audit(t); len(events) != 0 {
		t.Fatalf("un PATCH sin cambios no se audita: %+v", events)
	}

	httptestutil.Expect(t, c.Do("PUT", appEnv, map[string]any{"vars": map[string]string{"A": "1"}}, session), http.StatusNoContent)
	putAt := e.clock.Now()
	e.clock.Advance(time.Minute)
	httptestutil.Expect(t, c.Do("PATCH", appEnv, map[string]any{"set": map[string]string{"A": "1"}, "secret": true}, session), http.StatusNoContent)
	if !seedtest.ConfigChangedAt(t, e.db, appID).Equal(putAt) {
		t.Fatal("un set con el mismo valor y secret no marca la app")
	}
	if events := e.audit(t); len(events) != 1 {
		t.Fatalf("un set sin cambios no se audita: %+v", events)
	}
}

// La única lectura que le queda a PATCH es descifrar las claves de set que ya
// existen, para ver si cambian. Una fila rota no lo bloquea: se puede agregar
// otra variable, y sobrescribirla la arregla.
func TestEnvAPIPatchOverBrokenRow(t *testing.T) {
	e := newTestEnv(t)
	c := e.client
	ctx := context.Background()
	httptestutil.Expect(t, c.Do("PUT", stageEnv, map[string]any{"vars": map[string]string{"BROKEN": "viejo"}, "secret": false}, session), http.StatusNoContent)
	// Mover la fila del stage a la app: con la AAD de la app no descifra.
	appID := seedtest.AppID(t, e.db, "auditfy", "staging", "api")
	if _, err := e.db.Conn(ctx).ExecContext(ctx, `UPDATE env_vars SET app_id = ?, stage_id = NULL WHERE key = 'BROKEN'`, appID); err != nil {
		t.Fatal(err)
	}
	w := c.Do("GET", appEnv, nil, session)
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "viejo") {
		t.Fatalf("GET con una fila rota: %d %s", w.Code, w.Body.String())
	}

	httptestutil.Expect(t, c.Do("PATCH", appEnv, map[string]any{"set": map[string]string{"OTHER": "y"}, "secret": false}, session), http.StatusNoContent)
	// El mismo valor: no descifrar cuenta como cambio, así que se reescribe.
	w = c.Do("PATCH", appEnv, map[string]any{"set": map[string]string{"BROKEN": "viejo"}, "secret": false}, session)
	httptestutil.Expect(t, w, http.StatusNoContent)
	if strings.Contains(w.Body.String(), "viejo") {
		t.Fatalf("la respuesta filtró el valor: %s", w.Body.String())
	}

	w = c.Do("GET", appEnv, nil, session)
	httptestutil.Expect(t, w, http.StatusOK)
	vars := httptestutil.Decode[map[string][]map[string]any](t, w)["vars"]
	if len(vars) != 2 || vars[0]["key"] != "BROKEN" || vars[0]["value"] != "viejo" || vars[1]["value"] != "y" {
		t.Fatalf("vars=%v", vars)
	}
}

// Los valores no secretos vuelven tal cual, con cualquier texto UTF-8.
func TestEnvAPIUnicodeValues(t *testing.T) {
	e := newTestEnv(t)
	c := e.client
	value := "ñandú 🚀 \"comillas\" \\ <b>&\t\n"
	httptestutil.Expect(t, c.Do("PUT", stageEnv, map[string]any{"vars": map[string]string{"TEXT": value}, "secret": false}, session), http.StatusNoContent)
	w := c.Do("GET", stageEnv, nil, session)
	httptestutil.Expect(t, w, http.StatusOK)
	vars := httptestutil.Decode[map[string][]map[string]any](t, w)["vars"]
	if len(vars) != 1 || vars[0]["value"] != value {
		t.Fatalf("vars=%v", vars)
	}
}

// Un token de auditfy/staging no lee ni escribe las variables de
// auditfy/production, ni las del stage ni las de sus apps. Si las rutas no
// usaran {p}/{s}/{a} literales, el middleware las vería como del proyecto y
// el GET pasaría.
func TestEnvAPIStageTokenCannotTouchOtherStage(t *testing.T) {
	e := newTestEnv(t)
	c := e.client
	for _, path := range []string{prodEnv, prodApp} {
		httptestutil.Expect(t, c.Do("PUT", path, map[string]any{"vars": map[string]string{"PROD": "valor-prod"}, "secret": false}, session), http.StatusNoContent)
	}
	before := e.audit(t)

	bodies := map[string]any{
		"GET":   nil,
		"PUT":   map[string]any{"vars": map[string]string{"A": "1"}},
		"PATCH": map[string]any{"set": map[string]string{"A": "1"}, "unset": []string{"PROD"}},
	}
	for _, path := range []string{prodEnv, prodApp} {
		for _, method := range []string{"GET", "PUT", "PATCH"} {
			w := c.Do(method, path, bodies[method], httptestutil.WithBearer("staging"), httptestutil.WithOrigin(""))
			if w.Code != http.StatusForbidden || httptestutil.ErrorCode(t, w) != "out_of_scope" {
				t.Errorf("%s %s: %d %s", method, path, w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "valor-prod") {
				t.Errorf("%s %s filtró un valor: %s", method, path, w.Body.String())
			}
		}
	}

	for _, path := range []string{prodEnv, prodApp} {
		w := c.Do("GET", path, nil, session)
		httptestutil.Expect(t, w, http.StatusOK)
		vars := httptestutil.Decode[map[string][]map[string]any](t, w)["vars"]
		if len(vars) != 1 || vars[0]["key"] != "PROD" || vars[0]["value"] != "valor-prod" {
			t.Fatalf("%s cambió: %v", path, vars)
		}
	}
	if after := e.audit(t); len(after) != len(before) {
		t.Fatalf("las peticiones rechazadas no se auditan: %+v", after)
	}
}

// Los tokens de API pueden leer y escribir variables dentro de su alcance (§10.1 del spec).
func TestEnvAPITokenScopes(t *testing.T) {
	e := newTestEnv(t)
	env := map[string]any{"vars": map[string]string{"A": "1"}}
	patch := map[string]any{"set": map[string]string{"B": "2"}}
	cases := []struct {
		name   string
		token  string
		method string
		path   string
		body   any
		status int
		code   string
	}{
		{"stage: reemplaza env de su app", "staging", "PUT", appEnv, env, 204, ""},
		{"stage: reemplaza env de su stage", "staging", "PUT", stageEnv, env, 204, ""},
		{"stage: modifica env de su app", "staging", "PATCH", appEnv, patch, 204, ""},
		{"stage: lee env de su app", "staging", "GET", appEnv, nil, 200, ""},
		{"stage: lee env de su stage", "staging", "GET", stageEnv, nil, 200, ""},
		{"stage: no toca otro stage", "staging", "PUT", prodApp, env, 403, "out_of_scope"},
		{"stage: no modifica otro stage", "staging", "PATCH", prodEnv, patch, 403, "out_of_scope"},
		{"stage: no lee otro stage", "staging", "GET", prodEnv, nil, 403, "out_of_scope"},
		{"proyecto: toca cualquier stage", "project", "PUT", prodApp, env, 204, ""},
		{"proyecto: modifica cualquier stage", "project", "PATCH", prodEnv, patch, 204, ""},
		{"proyecto: lee cualquier stage", "project", "GET", prodApp, nil, 200, ""},
		{"otro proyecto: no lee", "otro", "GET", stageEnv, nil, 403, "out_of_scope"},
		{"otro proyecto: no escribe", "otro", "PUT", appEnv, env, 403, "out_of_scope"},
		{"otro proyecto: su stage sí", "otro", "PUT", "/api/v1/projects/otro/stages/staging/env", env, 204, ""},
		{"token inventado", "mk_inventado", "GET", appEnv, nil, 401, "unauthenticated"},
	}
	for _, c := range cases {
		// Los tokens no necesitan Origin: no hay cookies que robar.
		w := e.client.Do(c.method, c.path, c.body, httptestutil.WithBearer(c.token), httptestutil.WithOrigin(""))
		if w.Code != c.status {
			t.Errorf("%s: status %d, esperaba %d (%s)", c.name, w.Code, c.status, w.Body.String())
			continue
		}
		if c.code != "" && httptestutil.ErrorCode(t, w) != c.code {
			t.Errorf("%s: código %s, esperaba %s", c.name, httptestutil.ErrorCode(t, w), c.code)
		}
	}

	// Lo de fuera del alcance no se escribió; lo de dentro sí, a nombre del token.
	if n := seedtest.Count(t, e.db, `SELECT COUNT(*) FROM env_vars WHERE key = 'A'`); n != 4 {
		t.Fatalf("variables A: %d, esperaba 4", n)
	}
	events := e.audit(t)
	if len(events) != 6 {
		t.Fatalf("events=%+v", events)
	}
	for _, ev := range events {
		if ev.actorType != "token" {
			t.Fatalf("evento sin el token como actor: %+v", ev)
		}
	}
}
