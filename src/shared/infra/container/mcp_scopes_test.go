package container

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
)

// mcpOutcome es lo que dio una llamada: "ok", el code de un error de
// ejecución, o "unknown" si el servidor no tiene la herramienta (un error de
// protocolo del SDK).
func mcpCall(t *testing.T, cs *sdk.ClientSession, name string, args map[string]any) (outcome, text string) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		if strings.Contains(err.Error(), fmt.Sprintf("unknown tool %q", name)) {
			return "unknown", err.Error()
		}
		t.Fatalf("%s %v: %v", name, args, err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("%s %v: %d bloques", name, args, len(res.Content))
	}
	text = res.Content[0].(*sdk.TextContent).Text
	if !res.IsError {
		return "ok", text
	}
	var e struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(text), &e); err != nil || e.Error.Code == "" {
		t.Fatalf("%s %v: un error que no es de muelle: %s", name, args, text)
	}
	return e.Error.Code, text
}

// La matriz MCP del contrato §8.3 con muelle completo por HTTP y el cliente
// del SDK: cada herramienta en p1/s1, p1/s2 y p2/s1 con los cinco tipos de
// token (P, PS, P·L, PS·L y T·L). Fuera del alcance es out_of_scope antes de
// leer nada (exista o no el recurso); un token de solo lectura no tiene las
// de escritura (ni en tools/list ni por nombre). Si alguna celda cambia,
// cambió la seguridad.
func TestMCPScopes(t *testing.T) {
	e := newMCPEnv(t)

	// Un deploy en cada app (queda queued: los workers no corren), para que
	// get_deploy y read_deploy_log encuentren algo dentro del alcance.
	deployID := map[string]any{}
	for _, where := range []string{"p1/s1", "p1/s2", "p2/s1"} {
		p, s, _ := strings.Cut(where, "/")
		w := e.do("POST", "/api/v1/projects/"+p+"/stages/"+s+"/apps/a/deploys", map[string]any{}, e.s)
		httptestutil.Expect(t, w, http.StatusAccepted)
		deployID[where] = httptestutil.Decode[map[string]any](t, w)["id"]
	}

	type tool struct {
		name  string
		args  func(p, s, where string) map[string]any
		ok    string // lo que da dentro del alcance: "ok" o un code
		write bool
		noApp bool // get_env sin app
	}
	app := func(extra map[string]any) func(p, s, where string) map[string]any {
		return func(p, s, _ string) map[string]any {
			m := map[string]any{"project": p, "stage": s, "app": "a"}
			for k, v := range extra {
				m[k] = v
			}
			return m
		}
	}
	tools := []tool{
		{name: "list_apps", args: func(p, s, _ string) map[string]any { return map[string]any{"project": p, "stage": s} }, ok: "ok"},
		{name: "get_app", args: app(nil), ok: "ok"},
		{name: "get_env", args: app(nil), ok: "ok"},
		{name: "get_env", args: func(p, s, _ string) map[string]any { return map[string]any{"project": p, "stage": s} }, ok: "ok", noApp: true},
		{name: "get_app_status", args: app(nil), ok: "ok"},
		// Sin el stack en Swarm (el Docker falso), el servicio no existe.
		{name: "get_app_logs", args: app(nil), ok: "not_found"},
		{name: "list_deploys", args: app(nil), ok: "ok"},
		{name: "get_deploy", args: func(p, s, where string) map[string]any {
			return map[string]any{"project": p, "stage": s, "app": "a", "id": deployID[where]}
		}, ok: "ok"},
		{name: "read_deploy_log", args: func(p, s, where string) map[string]any {
			return map[string]any{"project": p, "stage": s, "app": "a", "id": deployID[where]}
		}, ok: "ok"},
		{name: "list_domains", args: app(nil), ok: "ok"},
		// Las de escritura: set_env antes que unset_env; rollback sin un
		// deploy exitoso anterior.
		{name: "deploy", args: app(nil), ok: "ok", write: true},
		{name: "rollback", args: app(nil), ok: "no_previous_deploy", write: true},
		{name: "set_env", args: func(p, s, _ string) map[string]any {
			return map[string]any{"project": p, "stage": s, "app": "a", "vars": map[string]string{"MCP_X": "1"}}
		}, ok: "ok", write: true},
		{name: "unset_env", args: func(p, s, _ string) map[string]any {
			return map[string]any{"project": p, "stage": s, "app": "a", "keys": []string{"MCP_X"}}
		}, ok: "ok", write: true},
	}

	in := func(places ...string) func(string) bool {
		return func(where string) bool { return slices.Contains(places, where) }
	}
	type kind struct {
		name, token string
		reads       func(where string) bool
		writes      func(where string) bool
		readOnly    bool
	}
	kinds := []kind{
		{"P", e.p, in("p1/s1", "p1/s2"), in("p1/s1", "p1/s2"), false},
		{"PS", e.ps, in("p1/s1"), in("p1/s1"), false},
		{"P·L", e.pl, in("p1/s1", "p1/s2"), in(), true},
		{"PS·L", e.psl, in("p1/s1"), in(), true},
		{"T·L", e.tl, in("p1/s1", "p1/s2", "p2/s1"), in(), true},
	}
	all := []string{"deploy", "get_app", "get_app_logs", "get_app_status", "get_deploy", "get_env", "list_apps",
		"list_deploys", "list_domains", "list_projects", "list_stages", "read_deploy_log", "rollback", "set_env", "unset_env"}
	readOnlyTools := slices.DeleteFunc(slices.Clone(all), func(n string) bool {
		return slices.Contains([]string{"deploy", "rollback", "set_env", "unset_env"}, n)
	})

	cells := 0
	for _, k := range kinds {
		cs, err := e.connect(t, k.token, "")
		if err != nil {
			t.Fatalf("%s: %v", k.name, err)
		}
		// tools/list: 15 herramientas, u 11 sin las de escritura.
		res, err := cs.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, tl := range res.Tools {
			names = append(names, tl.Name)
		}
		slices.Sort(names)
		want := all
		if k.readOnly {
			want = readOnlyTools
		}
		if !slices.Equal(names, want) {
			t.Errorf("%s: tools/list %v, esperaba %v", k.name, names, want)
		}

		for _, where := range []string{"p1/s1", "p1/s2", "p2/s1"} {
			p, s, _ := strings.Cut(where, "/")
			for _, tl := range tools {
				want := tl.ok
				switch {
				case tl.write && k.readOnly:
					want = "unknown"
				case tl.write && !k.writes(where), !tl.write && !k.reads(where):
					want = "out_of_scope"
				}
				got, text := mcpCall(t, cs, tl.name, tl.args(p, s, where))
				if got != want {
					t.Errorf("%s: %s en %s (sin app: %v): %s, esperaba %s: %s", k.name, tl.name, where, tl.noApp, got, want, text)
				}
				cells++
			}
		}

		// list_projects filtra; list_stages(p1) filtra, y en p2 es
		// out_of_scope salvo para T·L.
		projects := []string{"p1"}
		if k.name == "T·L" {
			projects = []string{"p1", "p2"}
		}
		got, text := mcpCall(t, cs, "list_projects", nil)
		if got != "ok" || slugsOf(t, text, "projects") != fmt.Sprint(projects) {
			t.Errorf("%s: list_projects: %s", k.name, text)
		}
		stages := []string{"s1", "s2"}
		if strings.HasPrefix(k.name, "PS") {
			stages = []string{"s1"}
		}
		got, text = mcpCall(t, cs, "list_stages", map[string]any{"project": "p1"})
		if got != "ok" || slugsOf(t, text, "stages") != fmt.Sprint(stages) {
			t.Errorf("%s: list_stages(p1): %s", k.name, text)
		}
		got, text = mcpCall(t, cs, "list_stages", map[string]any{"project": "p2"})
		wantP2 := "out_of_scope"
		if k.name == "T·L" {
			wantP2 = "ok"
		}
		if got != wantP2 {
			t.Errorf("%s: list_stages(p2): %s", k.name, text)
		}
		cells += 3

		// Fuera del alcance es out_of_scope aunque la app no exista; dentro,
		// not_found.
		got, _ = mcpCall(t, cs, "get_app", map[string]any{"project": "p3", "stage": "s1", "app": "nada"})
		wantMissing := "out_of_scope"
		if k.name == "T·L" {
			wantMissing = "not_found"
		}
		if got != wantMissing {
			t.Errorf("%s: get_app de un proyecto que no existe: %s, esperaba %s", k.name, got, wantMissing)
		}
		if got, _ := mcpCall(t, cs, "get_app", map[string]any{"project": "p1", "stage": "s1", "app": "nada"}); got != "not_found" {
			t.Errorf("%s: get_app de una app que no existe en su alcance: %s", k.name, got)
		}
	}
	if want := len(kinds) * (3*len(tools) + 3); cells != want {
		t.Fatalf("la matriz tiene %d celdas, esperaba %d", cells, want)
	}
}

// slugsOf es la lista de slugs de la lista key de una salida, como
// fmt.Sprint de un []string.
func slugsOf(t *testing.T, text, key string) string {
	t.Helper()
	var out map[string]json.RawMessage
	var list []struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("%v: %s", err, text)
	}
	if err := json.Unmarshal(out[key], &list); err != nil {
		t.Fatalf("%v: %s", err, text)
	}
	var slugs []string
	for _, s := range list {
		slugs = append(slugs, s.Slug)
	}
	return fmt.Sprint(slugs)
}
