package container

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/cmd/mcp"
	"github.com/CPU-commits/muelle/src/cmd/mcp/tools"
)

// bearerTransport pone el Bearer (y los headers de más) en cada pedido del
// cliente del SDK, como el --header de claude mcp add.
type bearerTransport struct {
	token  string
	header http.Header
	last   *http.Response
}

func (b *bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	if b.token != "" {
		r.Header.Set("Authorization", "Bearer "+b.token)
	}
	for k, vs := range b.header {
		r.Header[k] = vs
	}
	res, err := http.DefaultTransport.RoundTrip(r)
	if err == nil {
		b.last = res
	}
	return res, err
}

// mcpEnv es muelle completo detrás de un httptest.Server, con una sesión y
// los tokens de la matriz (newScopeFixture).
type mcpEnv struct {
	scopeFixture
	srv *httptest.Server
}

func newMCPEnv(t *testing.T) *mcpEnv {
	t.Helper()
	e := &mcpEnv{scopeFixture: newScopeFixture(t)}
	e.srv = httptest.NewUnstartedServer(e.c.Server)
	e.srv.Start()
	t.Cleanup(e.srv.Close)
	return e
}

// connect conecta un cliente del SDK con el token y la versión de protocolo
// (vacía: la que prueba primero el cliente, 2026-07-28).
func (e *mcpEnv) connect(t *testing.T, token, version string) (*sdk.ClientSession, error) {
	t.Helper()
	transport := &sdk.StreamableClientTransport{
		Endpoint:   e.srv.URL + "/api/v1/mcp",
		HTTPClient: &http.Client{Transport: &bearerTransport{token: token}},
		MaxRetries: -1,
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "test"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cs, err := client.Connect(ctx, transport, &sdk.ClientSessionOptions{ProtocolVersion: version})
	if err == nil {
		t.Cleanup(func() { cs.Close() })
	}
	return cs, err
}

// post hace un POST a /api/v1/mcp con los headers de un cliente MCP y los
// que se pasen en pares (un valor "" borra el header).
func (e *mcpEnv) post(t *testing.T, body string, header ...string) *http.Response {
	t.Helper()
	return e.request(t, "POST", body, header...)
}

func (e *mcpEnv) request(t *testing.T, method, body string, header ...string) *http.Response {
	t.Helper()
	r, err := http.NewRequest(method, e.srv.URL+"/api/v1/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("MCP-Protocol-Version", "2025-11-25")
	for i := 0; i+1 < len(header); i += 2 {
		if header[i+1] == "" {
			r.Header.Del(header[i])
			continue
		}
		r.Header.Set(header[i], header[i+1])
	}
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func apiErrorCode(t *testing.T, body string) string {
	t.Helper()
	var b struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &b); err != nil {
		t.Fatalf("no es un error de la API: %s", body)
	}
	return b.Error.Code
}

const toolsList = `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`

// Lo que revisa muelle antes del SDK (contrato §8.2): el Origin, el Bearer y
// su validez, con los WWW-Authenticate; el método; y lo que revisa el SDK:
// el tamaño del cuerpo, el Content-Type y el Accept.
func TestMCPHTTP(t *testing.T) {
	e := newMCPEnv(t)

	// Sin token, el cliente del SDK no conecta: 401 con WWW-Authenticate.
	tr := &bearerTransport{}
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "test"}, nil)
	_, err := client.Connect(context.Background(), &sdk.StreamableClientTransport{
		Endpoint: e.srv.URL + "/api/v1/mcp", HTTPClient: &http.Client{Transport: tr}, MaxRetries: -1,
	}, nil)
	if err == nil || tr.last == nil || tr.last.StatusCode != 401 || tr.last.Header.Get("WWW-Authenticate") != `Bearer realm="muelle"` {
		t.Fatalf("sin token: err=%v res=%+v", err, tr.last)
	}

	res := e.post(t, toolsList)
	if body := readBody(t, res); res.StatusCode != 401 || apiErrorCode(t, body) != "unauthenticated" ||
		!strings.Contains(body, "usa un token de API") || res.Header.Get("WWW-Authenticate") != `Bearer realm="muelle"` {
		t.Errorf("sin token: %d %s %v", res.StatusCode, body, res.Header)
	}
	// Solo la cookie de sesión: 401, aunque la sesión sea válida.
	res = e.post(t, toolsList, "Cookie", sessionCookieHeader(t, e), "Origin", httptestutil.PanelOrigin)
	if res.StatusCode != 401 || res.Header.Get("WWW-Authenticate") != `Bearer realm="muelle"` {
		t.Errorf("con cookie: %d %v", res.StatusCode, res.Header)
	}
	// Un token revocado: 401 con error="invalid_token".
	revokedID, revoked := e.newToken(e.s, "p1", "")
	httptestutil.Expect(t, e.do("DELETE", "/api/v1/tokens/"+revokedID, nil, e.s), http.StatusNoContent)
	res = e.post(t, toolsList, "Authorization", "Bearer "+revoked)
	if body := readBody(t, res); res.StatusCode != 401 || !strings.Contains(body, "el token no existe, se revocó o venció") ||
		res.Header.Get("WWW-Authenticate") != `Bearer realm="muelle", error="invalid_token"` {
		t.Errorf("revocado: %d %s %v", res.StatusCode, body, res.Header)
	}
	if _, err := e.connect(t, revoked, ""); err == nil {
		t.Error("el cliente conectó con un token revocado")
	}

	auth := "Bearer " + e.pl
	// El Origin: uno ajeno es 403 aunque traiga Bearer; el del panel pasa.
	res = e.post(t, toolsList, "Authorization", auth, "Origin", "https://evil.example")
	if body := readBody(t, res); res.StatusCode != 403 || apiErrorCode(t, body) != "bad_origin" {
		t.Errorf("Origin ajeno: %d %s", res.StatusCode, body)
	}
	res = e.post(t, toolsList, "Authorization", auth, "Origin", httptestutil.PanelOrigin)
	if body := readBody(t, res); res.StatusCode != 200 || !strings.Contains(body, `"tools":[`) {
		t.Errorf("Origin del panel: %d %s", res.StatusCode, body)
	}
	// Solo POST: 405 con Allow: POST (el mux).
	for _, m := range []string{"GET", "DELETE"} {
		res = e.request(t, m, "", "Authorization", auth)
		if body := readBody(t, res); res.StatusCode != 405 || res.Header.Get("Allow") != "POST" || apiErrorCode(t, body) != "method_not_allowed" {
			t.Errorf("%s: %d %s %v", m, res.StatusCode, body, res.Header)
		}
	}
	// Más de 1 MiB: 413.
	big := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"x":"` + strings.Repeat("a", 1<<20) + `"}}}`
	if res = e.post(t, big, "Authorization", auth); res.StatusCode != 413 {
		t.Errorf("cuerpo de más de 1 MiB: %d", res.StatusCode)
	}
	// Sin los dos tipos en Accept: 400. Otro Content-Type: 415.
	if res = e.post(t, toolsList, "Authorization", auth, "Accept", "application/json"); res.StatusCode != 400 {
		t.Errorf("Accept sin text/event-stream: %d", res.StatusCode)
	}
	if res = e.post(t, toolsList, "Authorization", auth, "Content-Type", "text/plain"); res.StatusCode != 415 {
		t.Errorf("Content-Type: %d", res.StatusCode)
	}
	// Los headers de la API.
	res = e.post(t, toolsList, "Authorization", auth)
	if res.Header.Get("Cache-Control") != "no-store" || res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("headers: %v", res.Header)
	}
}

// El curl del contrato §8.2: con MCP-Protocol-Version 2025-11-25, sin
// initialize, tools/list responde.
func TestMCPCurlExample(t *testing.T) {
	e := newMCPEnv(t)
	res := e.post(t, toolsList, "Authorization", "Bearer "+e.p)
	body := readBody(t, res)
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("%d %v %s", res.StatusCode, res.Header, body)
	}
	var out struct {
		Result struct {
			Tools []map[string]any `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	if res.Header.Get("Mcp-Session-Id") != "" {
		t.Errorf("stateless no asigna Mcp-Session-Id: %v", res.Header)
	}
}

// La negociación del cliente del SDK: 2026-07-28 en stateless, o la que se
// le fije (2025-11-25). Las dos dan las instrucciones, la versión y las
// capacidades exactamente {"tools":{}}, con un token de escritura y con uno
// de solo lectura.
func TestMCPNegotiationAndServerInfo(t *testing.T) {
	e := newMCPEnv(t)
	for _, c := range []struct {
		version, want string
	}{{"", "2026-07-28"}, {"2025-11-25", "2025-11-25"}} {
		for _, token := range []string{e.p, e.tl} {
			cs, err := e.connect(t, token, c.version)
			if err != nil {
				t.Fatalf("versión %q: %v", c.version, err)
			}
			res := cs.InitializeResult()
			caps, _ := json.Marshal(res.Capabilities)
			if res.ProtocolVersion != c.want || string(caps) != `{"tools":{}}` || res.Instructions != mcp.Instructions ||
				res.ServerInfo == nil || res.ServerInfo.Name != "muelle" || res.ServerInfo.Version != "dev" {
				t.Errorf("versión %q: %s caps=%s %+v", c.version, res.ProtocolVersion, caps, res.ServerInfo)
			}
			// Una llamada de punta a punta con la versión negociada.
			if _, err := cs.ListTools(context.Background(), nil); err != nil {
				t.Errorf("versión %q: tools/list: %v", c.version, err)
			}
		}
	}
}

// El ReadTimeout del servidor cancelaría el ctx de una llamada larga: el
// handler lleva el plazo de lectura a 60 s después de autenticar.
func TestMCPReadDeadline(t *testing.T) {
	e := &mcpEnv{scopeFixture: newScopeFixture(t)}
	type waitIn struct {
		Project string `json:"project"`
		Stage   string `json:"stage"`
		WaitMs  int    `json:"wait_ms"`
	}
	tools.Register(e.c.Tools, tools.Tool{
		Name: "test_wait", Title: "Esperar", Description: "Espera wait_ms.", Kind: tools.Read,
		Bounds: map[string]tools.Range{"wait_ms": {Min: 0, Max: 5000}},
	}, func(in waitIn) *tools.Scope { return &tools.Scope{Project: in.Project, Stage: in.Stage} },
		func(ctx context.Context, in waitIn) (any, error) {
			select {
			case <-time.After(time.Duration(in.WaitMs) * time.Millisecond):
				return map[string]string{"ok": "esperé"}, nil
			case <-ctx.Done():
				return nil, errors.New("se canceló el ctx de la llamada")
			}
		})
	e.srv = httptest.NewUnstartedServer(e.c.Server)
	e.srv.Config.ReadTimeout = time.Second
	e.srv.Start()
	t.Cleanup(e.srv.Close)

	cs, err := e.connect(t, e.p, "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "test_wait",
		Arguments: map[string]any{"project": "p1", "stage": "s1", "wait_ms": 2000}})
	if err != nil || res.IsError || len(res.Content) != 1 || res.Content[0].(*sdk.TextContent).Text != `{"ok":"esperé"}` {
		t.Fatalf("res=%+v err=%v", res, err)
	}

	// Lo mismo con get_deploy y wait_s: el deploy queda queued (los workers
	// no corren), y la espera termina en timed_out, no con el ctx cancelado.
	w := e.do("POST", "/api/v1/projects/p1/stages/s1/apps/a/deploys", map[string]any{}, e.s)
	httptestutil.Expect(t, w, http.StatusAccepted)
	id := httptestutil.Decode[map[string]any](t, w)["id"]
	got, text := mcpCall(t, cs, "get_deploy", map[string]any{"project": "p1", "stage": "s1", "app": "a", "id": id, "wait_s": 2})
	var out struct {
		Status   string `json:"status"`
		WaitedS  int    `json:"waited_s"`
		TimedOut bool   `json:"timed_out"`
	}
	if err := json.Unmarshal([]byte(text), &out); got != "ok" || err != nil || out.Status != "queued" || !out.TimedOut || out.WaitedS != 2 {
		t.Fatalf("get_deploy con wait_s: %s %s", got, text)
	}
}

// sessionCookieHeader es el header Cookie de la sesión de e.
func sessionCookieHeader(t *testing.T, e *mcpEnv) string {
	t.Helper()
	r := httptest.NewRequest("GET", "/", nil)
	e.s(r)
	return r.Header.Get("Cookie")
}

// De punta a punta (contrato §8.6): deploy y set_env con un token de
// escritura dejan el deploy con el nombre del token y trigger api, y la
// auditoría con el token como actor y "via": "mcp"; con un token de solo
// lectura, deploy no existe.
func TestMCPWritesEndToEnd(t *testing.T) {
	e := newMCPEnv(t)
	cs, err := e.connect(t, e.ps, "")
	if err != nil {
		t.Fatal(err)
	}
	got, text := mcpCall(t, cs, "deploy", map[string]any{"project": "p1", "stage": "s1", "app": "a"})
	var q struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(text), &q); got != "ok" || err != nil || q.ID == 0 || q.Status != "queued" {
		t.Fatalf("deploy: %s %s", got, text)
	}
	if got, text := mcpCall(t, cs, "set_env", map[string]any{"project": "p1", "stage": "s1", "app": "a",
		"vars": map[string]string{"LOG_LEVEL": "debug"}}); got != "ok" || text != `{"set":["LOG_LEVEL"],"note":"Se aplican en el próximo deploy; get_app y list_apps dicen si quedaron cambios pendientes."}` {
		t.Fatalf("set_env: %s %s", got, text)
	}

	w := e.do("GET", fmt.Sprintf("/api/v1/projects/p1/stages/s1/apps/a/deploys/%d", q.ID), nil, e.s)
	httptestutil.Expect(t, w, http.StatusOK)
	if d := httptestutil.Decode[map[string]any](t, w); d["actor_name"] != "ps" || d["trigger"] != "api" {
		t.Errorf("el deploy: actor_name=%v trigger=%v", d["actor_name"], d["trigger"])
	}

	w = e.do("GET", "/api/v1/audit", nil, e.s)
	httptestutil.Expect(t, w, http.StatusOK)
	seen := map[string]bool{}
	for _, ev := range httptestutil.Decode[[]map[string]any](t, w) {
		action, _ := ev["action"].(string)
		if ev["target"] != "app:p1/s1/a" || (action != "deploy.create" && action != "env.patch") || ev["actor_name"] != "ps" {
			continue
		}
		details, _ := ev["details"].(map[string]any)
		if ev["actor_type"] != "token" || details["via"] != "mcp" {
			t.Errorf("%s: actor_type=%v details=%v", action, ev["actor_type"], details)
		}
		seen[action] = true
	}
	if !seen["deploy.create"] || !seen["env.patch"] {
		t.Errorf("faltan eventos del token en la auditoría: %v", seen)
	}

	ro, err := e.connect(t, e.psl, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, text := mcpCall(t, ro, "deploy", map[string]any{"project": "p1", "stage": "s1", "app": "a"}); got != "unknown" {
		t.Errorf("deploy con un token de solo lectura: %s %s", got, text)
	}
}
