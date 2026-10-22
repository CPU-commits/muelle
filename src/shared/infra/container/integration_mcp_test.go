//go:build integration

// La prueba del servidor MCP contra el Swarm real de este nodo (diseño del
// MCP, §11.6): muelle completo (container.New, con Run como en main) sirve
// HTTP en 127.0.0.1, en el primer puerto libre de 25500 a 25599, y el cliente
// del SDK despliega nginx:alpine y lee su estado y sus logs por MCP:
//
//	go test -tags integration -race -count=1 -timeout 25m -run IntegrationMCP -v ./src/shared/infra/container/
//
// Todo lo que crea se llama it8<id>… (id: 4 hex al azar): el proyecto, y con
// él su stack, su servicio y la red del stage. Sin dominio del panel (el
// origen es una IP), el proxy está deshabilitado y no hay muelle-caddy; los
// nombres de ingress son igual de la prueba (it8-<id>-caddy…), así nunca se
// conecta el muelle-caddy de verdad a la red del stage. Se borra en
// t.Cleanup, también si la prueba falla; un -timeout que vence no corre los
// t.Cleanup, y lo barre el TestMain de la corrida siguiente
// (integration_main_test.go). Los digests salen de las imágenes del nodo
// (e2eDigests), así Swarm no baja nada; sin nginx:alpine en el nodo, la prueba
// se salta.
package container

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/ingress"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare/cftest"
	"github.com/CPU-commits/muelle/src/settings"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

const (
	it8Prefix   = "it8"
	it8PortFrom = 25500
	it8PortTo   = 25599 // nunca 25080, que además queda fuera
	it8Stage    = "s"
	it8App      = "web"
)

// it8Env es el muelle de la prueba, servido por HTTP de verdad.
type it8Env struct {
	t       *testing.T
	docker  *docker.Client
	id      string
	project string
	c       *Container
	base    string // http://127.0.0.1:<puerto>, también MUELLE_PANEL_ORIGIN
	session string
	// texts junta todo lo que devolvió el MCP, para buscar el secreto.
	mu    sync.Mutex
	texts []string
}

// it8Listen escucha en el primer puerto libre de 25500 a 25599.
func it8Listen(t *testing.T) net.Listener {
	t.Helper()
	for port := it8PortFrom; port <= it8PortTo; port++ {
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			return ln
		}
	}
	t.Fatalf("no hay un puerto libre de %d a %d", it8PortFrom, it8PortTo)
	return nil
}

func newIt8Env(t *testing.T) *it8Env {
	t.Helper()
	cli, err := docker.New(docker.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cli.Close() })
	ctx := context.Background()
	info, err := cli.Info(ctx)
	if err != nil {
		t.Skipf("sin Docker: %v", err)
	}
	if info.SwarmState != docker.SwarmActive || !info.IsManager {
		t.Skipf("el nodo no es manager de un Swarm activo (%s)", info.SwarmState)
	}
	if img, err := cli.InspectImage(ctx, e2eNginx); err != nil || len(img.RepoDigests) == 0 {
		t.Skipf("falta la imagen %s con RepoDigests en el nodo (%v); bájala con docker pull", e2eNginx, err)
	}
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	id := hex.EncodeToString(b)
	e := &it8Env{t: t, docker: cli, id: id, project: it8Prefix + id}
	ln := it8Listen(t)
	e.base = "http://" + ln.Addr().String()
	t.Cleanup(e.cleanupDocker)
	t.Logf("it8: proyecto %s en %s", e.project, e.base)

	dir := t.TempDir()
	name := func(suffix string) string { return it8Prefix + "-" + id + "-" + suffix }
	s := settings.Settings{
		DataDir:       filepath.Join(dir, "data"),
		MasterKeyFile: writeKey(t, dir, "key"),
		PanelOrigin:   e.base,
	}
	log := slog.New(slog.NewTextHandler(t.Output(), &slog.HandlerOptions{Level: slog.LevelInfo}))
	c, err := New(ctx, s, log, WithDocker(e2eDigests{cli}), WithIngress(ingress.Options{
		CaddyContainer: name("caddy"), PanelNetwork: e.project + "_panel",
		CaddyDataVolume: name("caddy-data"), CaddyConfigVolume: name("caddy-config"), CaddyAdminVolume: name("caddy-admin"),
		CaddyAdminDial: func(context.Context) (net.Conn, error) { return nil, errors.New("it8: sin muelle-caddy") },
		NetworkPrefix:  e.project,
		CloudflareURL:  cftest.New(t).URL(),
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("cerrando el contenedor: %v", err)
		}
	})
	e.c = c
	// Los plazos de main (newHTTPServer): el ReadTimeout de 30 s incluido.
	srv := &http.Server{Handler: c.Server, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		IdleTimeout: 3 * time.Minute, MaxHeaderBytes: 64 << 10}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	// Run como en main (los workers de deploy), hasta que la prueba termina.
	bgCtx, stop := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() { c.Run(bgCtx) })
	t.Cleanup(func() {
		stop()
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(60 * time.Second):
			t.Error("Run no volvió 60 s después de cancelarlo")
		}
	})

	token, created, err := c.Identity.EnsureSetupInvite(ctx)
	if err != nil || !created {
		t.Fatalf("EnsureSetupInvite: created=%v err=%v", created, err)
	}
	resp, body := e.send("POST", "/api/v1/auth/setup", map[string]string{"token": token, "username": "admin", "password": testPassword})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /auth/setup: %d %s", resp.StatusCode, body)
	}
	for _, ck := range resp.Cookies() {
		if ck.Name == utils.SessionCookie {
			e.session = ck.Value
		}
	}
	if e.session == "" {
		t.Fatalf("POST /auth/setup no dejó la cookie %s", utils.SessionCookie)
	}
	// Corre antes que el apagado del servidor y de Run (t.Cleanup es LIFO).
	t.Cleanup(e.cleanupREST)
	return e
}

// send hace la petición con la sesión (si hay) y el Origin del panel, como
// el panel, y devuelve la respuesta ya leída y su cuerpo.
func (e *it8Env) send(method, path string, body any) (*http.Response, []byte) {
	e.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := jsonv2.Marshal(body, jsonv2.Deterministic(true))
		if err != nil {
			e.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, e.base+path, reader)
	if err != nil {
		e.t.Fatal(err)
	}
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Origin", e.base)
	if e.session != "" {
		req.AddCookie(&http.Cookie{Name: utils.SessionCookie, Value: e.session})
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatal(err)
	}
	return resp, b
}

// call hace la petición con la sesión y exige want.
func (e *it8Env) call(method, path string, body any, want int) []byte {
	e.t.Helper()
	resp, b := e.send(method, path, body)
	if resp.StatusCode != want {
		e.t.Fatalf("%s %s: %d, esperaba %d: %s", method, path, resp.StatusCode, want, b)
	}
	return b
}

// connect conecta el cliente del SDK con el token, como Claude Code con
// --header "Authorization: Bearer …".
func (e *it8Env) connect(token string) *sdk.ClientSession {
	e.t.Helper()
	client := sdk.NewClient(&sdk.Implementation{Name: "it8", Version: "test"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cs, err := client.Connect(ctx, &sdk.StreamableClientTransport{
		Endpoint: e.base + "/api/v1/mcp", HTTPClient: &http.Client{Transport: &bearerTransport{token: token}}, MaxRetries: -1,
	}, nil)
	if err != nil {
		e.t.Fatalf("conectando el cliente MCP: %v", err)
	}
	e.t.Cleanup(func() { cs.Close() })
	return cs
}

// tool llama a la herramienta y devuelve lo que dio ("ok", el code o
// "unknown") y el texto, que guarda para buscar el secreto al final.
func (e *it8Env) tool(cs *sdk.ClientSession, name string, args map[string]any) (string, string) {
	e.t.Helper()
	got, text := mcpCall(e.t, cs, name, args)
	e.mu.Lock()
	e.texts = append(e.texts, text)
	e.mu.Unlock()
	return got, text
}

// ok es tool de una llamada que tiene que salir bien, con la salida en out.
func (e *it8Env) ok(cs *sdk.ClientSession, name string, args map[string]any, out any) {
	e.t.Helper()
	got, text := e.tool(cs, name, args)
	if got != "ok" {
		e.t.Fatalf("%s %v: %s %s", name, args, got, text)
	}
	if out != nil {
		if err := json.Unmarshal([]byte(text), out); err != nil {
			e.t.Fatalf("%s: %v: %s", name, err, text)
		}
	}
}

// cleanupREST borra la app (docker stack rm), el stage y el proyecto por REST,
// con la sesión, mientras muelle sigue sirviendo. Nunca corta la prueba: lo
// que quede lo borra cleanupDocker.
func (e *it8Env) cleanupREST() {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	P := "/api/v1/projects/" + e.project
	for _, path := range []string{P + "/stages/" + it8Stage + "/apps/" + it8App, P + "/stages/" + it8Stage, P} {
		var last string
		if !e2eRetry(ctx, 90*time.Second, func() bool {
			req, err := http.NewRequestWithContext(ctx, "DELETE", e.base+path, nil)
			if err != nil {
				last = err.Error()
				return false
			}
			req.Header.Set("Origin", e.base)
			req.AddCookie(&http.Cookie{Name: utils.SessionCookie, Value: e.session})
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				last = err.Error()
				return false
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			last = resp.Status + " " + string(b)
			return resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound
		}) {
			e.t.Errorf("limpieza: DELETE %s no terminó: %s", path, last)
		}
	}
}

// cleanupDocker borra por Docker lo que haya quedado del proyecto: su stack
// (esperando a que Swarm baje sus contenedores) y sus redes.
func (e *it8Env) cleanupDocker() {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	byProject := map[string]string{naming.LabelProject: e.project}
	if svcs, err := e.docker.ListServices(ctx, byProject); err == nil {
		stacks := map[string]bool{}
		for _, s := range svcs {
			if st := s.Labels[naming.LabelStackNamespace]; strings.HasPrefix(st, e.project+"_") {
				stacks[st] = true
			}
		}
		for st := range stacks {
			if out, err := e.docker.RemoveStack(ctx, st); err != nil {
				e.t.Errorf("limpieza: docker stack rm %s: %v\n%s", st, err, out)
			}
		}
	} else {
		e.t.Errorf("limpieza: listando servicios: %v", err)
	}
	if !e2eRetry(ctx, 90*time.Second, func() bool {
		svcs, err := e.docker.ListServices(ctx, byProject)
		return err == nil && len(svcs) == 0 && len(e.stackContainers(ctx)) == 0
	}) {
		e.t.Errorf("limpieza: quedaron servicios o contenedores del proyecto %s", e.project)
	}
	if nets, err := e.docker.ListNetworks(ctx, nil); err == nil {
		for _, n := range nets {
			if !strings.HasPrefix(n.Name, e.project+"_") {
				continue
			}
			if !e2eRetry(ctx, 60*time.Second, func() bool {
				err := e.docker.RemoveNetwork(ctx, n.Name)
				return err == nil || errors.Is(err, docker.ErrNotFound)
			}) {
				e.t.Errorf("limpieza: no se pudo borrar la red %s", n.Name)
			}
		}
	} else {
		e.t.Errorf("limpieza: listando redes: %v", err)
	}
}

// stackContainers son los contenedores (corriendo o no) de los stacks del
// proyecto.
func (e *it8Env) stackContainers(ctx context.Context) []string {
	return (&e2eEnv{t: e.t}).stackContainers(ctx, e.project+"_")
}

// TestIntegrationMCP, con el cliente del SDK por HTTP:
//  1. con la sesión: el proyecto, el stage y la app de formulario (nginx,
//     que imprime una variable secreta al arrancar);
//  2. un token de escritura del stage (it8-rw) y uno de todos los proyectos
//     de solo lectura (it8-ro);
//  3. con it8-rw: deploy, y get_deploy con wait_s hasta que termina;
//  4. get_app_status: la tarea corriendo con la imagen con digest;
//  5. get_app_logs: la línea de la app con el secreto enmascarado, y
//     contains no sirve para adivinarlo;
//  6. set_env de una variable no secreta deja cambios pendientes; sobre la
//     secreta, invalid;
//  7. con it8-ro: sin las de escritura, list_projects, get_app_status y
//     list_domains;
//  8. la auditoría: el token y "via": "mcp".
//
// El secreto no aparece en nada de lo que devolvió el MCP.
func TestIntegrationMCP(t *testing.T) {
	e := newIt8Env(t)
	secret := "it8-secreto-" + e.id + "-largo"
	P := "/api/v1/projects/" + e.project
	S := P + "/stages/" + it8Stage
	A := S + "/apps/" + it8App

	// 1.
	e.call("POST", "/api/v1/projects", map[string]string{"slug": e.project, "name": "it8 " + e.id}, http.StatusCreated)
	e.call("POST", P+"/stages", map[string]string{"slug": it8Stage}, http.StatusCreated)
	e.call("POST", S+"/apps", map[string]any{"slug": it8App, "kind": "service", "spec": map[string]any{
		"image": e2eNginx, "port": 80, "replicas": 1,
		"command": []string{"sh", "-c", "echo arranca con $IT8_SECRET; exec nginx -g 'daemon off;'"},
	}}, http.StatusCreated)
	e.call("PUT", A+"/env", map[string]any{"vars": map[string]string{"IT8_SECRET": secret}, "secret": true}, http.StatusNoContent)

	// 2.
	newToken := func(body map[string]any) string {
		t.Helper()
		var out struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(e.call("POST", "/api/v1/tokens", body, http.StatusCreated), &out); err != nil || !strings.HasPrefix(out.Token, "mk_") {
			t.Fatalf("token %v: %v %q", body["name"], err, out.Token)
		}
		return out.Token
	}
	rw := e.connect(newToken(map[string]any{"name": "it8-rw", "project": e.project, "stage": it8Stage}))
	ro := e.connect(newToken(map[string]any{"name": "it8-ro", "all_projects": true, "read_only": true}))
	app := map[string]any{"project": e.project, "stage": it8Stage, "app": it8App}
	with := func(extra map[string]any) map[string]any {
		m := maps.Clone(app)
		for k, v := range extra {
			m[k] = v
		}
		return m
	}

	// 3.
	if names := toolNames(t, rw); !slices.Contains(names, "deploy") || len(names) != 15 {
		t.Fatalf("tools/list con it8-rw: %v", names)
	}
	var queued struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	e.ok(rw, "deploy", app, &queued)
	if queued.ID == 0 || queued.Status != "queued" {
		t.Fatalf("deploy: %+v", queued)
	}
	type deployOut struct {
		Status    string            `json:"status"`
		Images    map[string]string `json:"images"`
		ActorName string            `json:"actor_name"`
		Trigger   string            `json:"trigger"`
		Log       *struct {
			Lines []string `json:"lines"`
		} `json:"log"`
		TimedOut bool `json:"timed_out"`
	}
	var d deployOut
	deadline := time.Now().Add(4 * time.Minute)
	for {
		d = deployOut{}
		e.ok(rw, "get_deploy", with(map[string]any{"id": queued.ID, "wait_s": 45}), &d)
		if d.Status != "queued" && d.Status != "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("el deploy %d sigue en %s después de 4 min", queued.ID, d.Status)
		}
	}
	if d.Status != "succeeded" || d.Log == nil || len(d.Log.Lines) == 0 || d.ActorName != "it8-rw" || d.Trigger != "api" {
		t.Fatalf("get_deploy: %+v", d)
	}
	if last := d.Log.Lines[len(d.Log.Lines)-1]; !strings.Contains(last, "succeeded en ") {
		t.Fatalf("la última línea del log: %q\n%s", last, strings.Join(d.Log.Lines, "\n"))
	}
	if !strings.HasPrefix(d.Images[it8App], e2eNginx+"@sha256:") {
		t.Fatalf("las imágenes del deploy: %v", d.Images)
	}

	// 4.
	type statusOut struct {
		Services []struct {
			Name            string `json:"name"`
			Image           string `json:"image"`
			ReplicasRunning int    `json:"replicas_running"`
			Tasks           []struct {
				State string `json:"state"`
			} `json:"tasks"`
		} `json:"services"`
	}
	var st statusOut
	ok := e2eRetry(context.Background(), 60*time.Second, func() bool {
		st = statusOut{}
		e.ok(rw, "get_app_status", app, &st)
		return len(st.Services) == 1 && st.Services[0].ReplicasRunning == 1 && len(st.Services[0].Tasks) > 0 &&
			st.Services[0].Tasks[0].State == "running"
	})
	if !ok || st.Services[0].Name != it8App || st.Services[0].Image != d.Images[it8App] {
		t.Fatalf("get_app_status: %+v", st)
	}

	// 5.
	type logsOut struct {
		Lines    []string `json:"lines"`
		Returned int      `json:"returned"`
	}
	var logs logsOut
	if !e2eRetry(context.Background(), 30*time.Second, func() bool {
		logs = logsOut{}
		e.ok(rw, "get_app_logs", with(map[string]any{"service": it8App, "tail": 50}), &logs)
		return slices.ContainsFunc(logs.Lines, func(l string) bool { return strings.Contains(l, "arranca con ***") })
	}) {
		t.Fatalf("get_app_logs sin la línea de la app enmascarada: %v", logs.Lines)
	}
	logs = logsOut{}
	e.ok(rw, "get_app_logs", with(map[string]any{"service": it8App, "tail": 50, "contains": "it8-secreto"}), &logs)
	if logs.Returned != 0 || len(logs.Lines) != 0 {
		t.Fatalf("contains con el secreto encontró líneas: %v", logs.Lines)
	}

	// 6.
	var set struct {
		Set []string `json:"set"`
	}
	e.ok(rw, "set_env", with(map[string]any{"vars": map[string]string{"LOG_LEVEL": "debug"}}), &set)
	if !slices.Equal(set.Set, []string{"LOG_LEVEL"}) {
		t.Fatalf("set_env: %v", set)
	}
	var appOut struct {
		PendingChanges bool `json:"pending_changes"`
	}
	e.ok(rw, "get_app", app, &appOut)
	if !appOut.PendingChanges {
		t.Fatal("get_app después de set_env: sin cambios pendientes")
	}
	if got, text := e.tool(rw, "set_env", with(map[string]any{"vars": map[string]string{"IT8_SECRET": "otro"}})); got != "invalid" {
		t.Fatalf("set_env sobre la secreta: %s %s", got, text)
	}

	// 7.
	names := toolNames(t, ro)
	for _, w := range []string{"deploy", "rollback", "set_env", "unset_env"} {
		if slices.Contains(names, w) {
			t.Errorf("tools/list con it8-ro trae %s: %v", w, names)
		}
	}
	if len(names) != 11 {
		t.Errorf("tools/list con it8-ro: %v", names)
	}
	var projects struct {
		Projects []struct {
			Slug string `json:"slug"`
		} `json:"projects"`
	}
	e.ok(ro, "list_projects", nil, &projects)
	if !slices.ContainsFunc(projects.Projects, func(p struct {
		Slug string `json:"slug"`
	}) bool {
		return p.Slug == e.project
	}) {
		t.Fatalf("list_projects con it8-ro: %+v", projects)
	}
	st = statusOut{}
	e.ok(ro, "get_app_status", app, &st)
	if len(st.Services) != 1 || st.Services[0].ReplicasRunning != 1 {
		t.Fatalf("get_app_status con it8-ro: %+v", st)
	}
	var domains struct {
		Domains []any `json:"domains"`
		Omitted int   `json:"omitted"`
	}
	e.ok(ro, "list_domains", app, &domains)
	if len(domains.Domains) != 0 || domains.Omitted != 0 {
		t.Fatalf("list_domains con it8-ro: %+v", domains)
	}
	if got, text := e.tool(ro, "deploy", app); got != "unknown" {
		t.Fatalf("deploy con it8-ro: %s %s", got, text)
	}

	// 8.
	var events []struct {
		Action    string         `json:"action"`
		Target    string         `json:"target"`
		ActorType string         `json:"actor_type"`
		ActorName string         `json:"actor_name"`
		Details   map[string]any `json:"details"`
	}
	if err := json.Unmarshal(e.call("GET", "/api/v1/audit?limit=200", nil, http.StatusOK), &events); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, ev := range events {
		if ev.Target != "app:"+e.project+"/"+it8Stage+"/"+it8App || (ev.Action != "deploy.create" && ev.Action != "env.patch") {
			continue
		}
		if ev.ActorType != "token" || ev.ActorName != "it8-rw" || ev.Details["via"] != "mcp" {
			t.Errorf("%s: %+v", ev.Action, ev)
		}
		seen[ev.Action] = true
	}
	if !seen["deploy.create"] || !seen["env.patch"] {
		t.Fatalf("la auditoría no tiene deploy.create y env.patch del token: %v", seen)
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	for _, text := range e.texts {
		if strings.Contains(text, secret) {
			t.Fatalf("una respuesta del MCP trae el secreto: %s", text)
		}
	}
	t.Logf("it8: %d respuestas del MCP sin el secreto", len(e.texts))
}

// toolNames son los nombres de tools/list, ordenados.
func toolNames(t *testing.T, cs *sdk.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range res.Tools {
		names = append(names, tl.Name)
	}
	slices.Sort(names)
	return names
}
