package container

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
	_ "time/tzdata" // America/Santiago aunque la máquina no tenga zoneinfo

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
)

// apiTime es el formato de todas las fechas de la API: RFC 3339 en UTC, con a
// lo más 3 decimales (los milisegundos que guarda SQLite).
var apiTime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{1,3})?Z$`)

// timeKeys devuelve, ordenadas, las claves *_at (y "at") de m.
func timeKeys(m map[string]any) []string {
	var keys []string
	for k := range m {
		if k == "at" || strings.HasSuffix(k, "_at") {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

// checkTimes revisa que cada fecha de m que no es null tenga el formato de
// apiTime, y que haya al menos una.
func checkTimes(t *testing.T, where string, m map[string]any) {
	t.Helper()
	n := 0
	for _, k := range timeKeys(m) {
		if m[k] == nil {
			continue
		}
		s, _ := m[k].(string)
		if !apiTime.MatchString(s) {
			t.Errorf("%s: %s=%v, esperaba UTC con a lo más milisegundos (…Z)", where, k, m[k])
		}
		n++
	}
	if n == 0 {
		t.Errorf("%s: no trae ninguna fecha: %v", where, m)
	}
}

// sameTimes revisa el formato de las fechas de created y que got (el GET del
// mismo recurso) traiga exactamente las mismas.
func sameTimes(t *testing.T, where string, created, got map[string]any) {
	t.Helper()
	checkTimes(t, where+" (create)", created)
	for _, k := range timeKeys(created) {
		if fmt.Sprint(created[k]) != fmt.Sprint(got[k]) {
			t.Errorf("%s: %s del create=%v, del GET=%v", where, k, created[k], got[k])
		}
	}
}

// Las fechas de la API salen siempre en UTC con milisegundos, también con
// TZ distinto de UTC: la respuesta de un create es idéntica a la del GET del
// mismo recurso (que lee lo guardado en SQLite).
func TestTimestampsAreUTCMillis(t *testing.T) {
	santiago, err := time.LoadLocation("America/Santiago")
	if err != nil {
		t.Fatal(err)
	}
	// TZ solo se lee la primera vez que se usa time.Local, así que además se
	// reemplaza time.Local (este paquete no corre tests en paralelo).
	t.Setenv("TZ", "America/Santiago")
	prev := time.Local
	time.Local = santiago
	t.Cleanup(func() { time.Local = prev })
	if _, offset := time.Now().Zone(); offset == 0 {
		t.Fatal("el test necesita un time.Local distinto de UTC")
	}

	e := api{t: t, c: newTestContainer(t)}
	s := e.setup()
	create := func(path string, body any) map[string]any {
		t.Helper()
		w := e.do("POST", path, body, s)
		httptestutil.Expect(t, w, http.StatusCreated)
		return httptestutil.Decode[map[string]any](t, w)
	}
	get := func(path string) map[string]any {
		t.Helper()
		w := e.do("GET", path, nil, s)
		httptestutil.Expect(t, w, http.StatusOK)
		return httptestutil.Decode[map[string]any](t, w)
	}

	const P = "/api/v1/projects/p"
	sameTimes(t, "proyecto", create("/api/v1/projects", map[string]string{"slug": "p", "name": "P"}), get(P))
	sameTimes(t, "stage", create(P+"/stages", map[string]string{"slug": "st"}), get(P+"/stages/st"))
	app := create(P+"/stages/st/apps", map[string]any{"slug": "a", "kind": "service", "spec": map[string]any{"image": "x", "port": 80}})
	sameTimes(t, "app", app, get(P+"/stages/st/apps/a"))
	if app["config_changed_at"] == nil || app["created_at"] == nil {
		t.Errorf("app sin config_changed_at o created_at: %v", app)
	}

	// Las invitaciones no se listan todavía: solo se revisa el formato.
	checkTimes(t, "invitación", create("/api/v1/invites", nil))

	token := create("/api/v1/tokens", map[string]any{"name": "ci", "project": "p", "expires_in_days": 30})
	if token["expires_at"] == nil {
		t.Fatalf("el token con expires_in_days no trae expires_at: %v", token)
	}
	findToken := func() map[string]any {
		t.Helper()
		w := e.do("GET", "/api/v1/tokens", nil, s)
		httptestutil.Expect(t, w, http.StatusOK)
		for _, tk := range httptestutil.Decode[[]map[string]any](t, w) {
			if tk["id"] == token["id"] {
				return tk
			}
		}
		t.Fatalf("GET /tokens no trae el token %v", token["id"])
		return nil
	}
	sameTimes(t, "token", token, findToken())
	// Usarlo llena last_used_at.
	httptestutil.Expect(t, e.bearer(token["token"].(string), "GET", P, nil), http.StatusOK)
	if used := findToken(); used["last_used_at"] == nil {
		t.Errorf("last_used_at sigue en null después de usar el token: %v", used)
	} else {
		checkTimes(t, "token usado", used)
	}

	httptestutil.Expect(t, e.do("PUT", P+"/stages/st/env", map[string]any{"vars": map[string]string{"K": "v"}}, s), http.StatusNoContent)
	for _, v := range get(P + "/stages/st/env")["vars"].([]any) {
		checkTimes(t, "variable", v.(map[string]any))
	}
	for _, path := range []string{"/api/v1/users", "/api/v1/audit"} {
		w := e.do("GET", path, nil, s)
		httptestutil.Expect(t, w, http.StatusOK)
		for _, item := range httptestutil.Decode[[]map[string]any](t, w) {
			checkTimes(t, path, item)
		}
	}
}
