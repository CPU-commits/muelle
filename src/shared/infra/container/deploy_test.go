package container

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/settings"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
)

// El registry inicial de install (MUELLE_INIT_REGISTRY_FILE, contrato §2.10)
// se guarda solo en el primer arranque de la base: uno que el admin borró no
// vuelve con el próximo arranque, aunque el secreto siga montado.
func TestInitRegistrySeededOnce(t *testing.T) {
	const password = "ghp_s3cr3t-de-install"
	s := testSettings(t)
	s.Init.Registry = &settings.InitRegistry{Host: "ghcr.io", Username: "bot", Password: password}
	ctx := context.Background()

	c, err := New(ctx, s, testLogger(t), WithDocker(dockertest.New()))
	if err != nil {
		t.Fatal(err)
	}
	var host, username string
	var stored []byte
	if err := c.DB.Conn(ctx).QueryRowContext(ctx, `SELECT host, username, password FROM registries`).Scan(&host, &username, &stored); err != nil {
		c.Close()
		t.Fatalf("el primer arranque no guardó el registry: %v", err)
	}
	if host != "ghcr.io" || username != "bot" || bytes.Contains(stored, []byte(password)) {
		c.Close()
		t.Fatalf("registry: %s %s %q", host, username, stored)
	}
	// Lo borra el admin (como el DELETE del panel).
	if _, err := c.DB.Conn(ctx).ExecContext(ctx, `DELETE FROM registries`); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	c, err = New(ctx, s, testLogger(t), WithDocker(dockertest.New()))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var n int
	if err := c.DB.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM registries`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("el registry borrado volvió: %d", n)
	}
}

// --- Los contratos del deploy con el contenedor completo (Fase 2) ------------------------

// miniSwarm es un Swarm mínimo en el OnDeployStack del Docker en memoria del
// contenedor: por cada docker stack deploy crea o reemplaza los servicios del
// compose con sus etiquetas y su imagen. Normalmente quedan convergidos (las
// réplicas corriendo y un UpdateStatus completed de ahora, así el paso 6
// termina en el primer poll); con hold, las réplicas quedan en 0 y sin
// UpdateStatus, y el deploy espera en el paso 6 hasta su deploy_timeout_s.
type miniSwarm struct {
	f    *dockertest.Fake
	mu   sync.Mutex
	hold bool
}

func (s *miniSwarm) setHold(on bool) { s.mu.Lock(); s.hold = on; s.mu.Unlock() }

func (s *miniSwarm) deploy(d docker.StackDeploy) error {
	s.mu.Lock()
	hold := s.hold
	s.mu.Unlock()
	var compose struct {
		Services map[string]struct {
			Image  string `json:"image"`
			Deploy *struct {
				Replicas *uint64           `json:"replicas"`
				Labels   map[string]string `json:"labels"`
			} `json:"deploy"`
		} `json:"services"`
	}
	if err := jsonv2.Unmarshal(d.Compose, &compose, jsontext.AllowDuplicateNames(false)); err != nil {
		return fmt.Errorf("miniSwarm: el compose no es JSON: %w", err)
	}
	now := time.Now()
	for name, svc := range compose.Services {
		replicas, labels := uint64(1), map[string]string{}
		if svc.Deploy != nil {
			if svc.Deploy.Replicas != nil {
				replicas = *svc.Deploy.Replicas
			}
			maps.Copy(labels, svc.Deploy.Labels)
		}
		running, state := replicas, docker.TaskRunning
		var update *docker.UpdateStatus
		if hold {
			running, state = 0, docker.TaskStarting
		} else {
			update = &docker.UpdateStatus{State: docker.UpdateCompleted, StartedAt: &now}
		}
		s.f.PutStackService(d.Name, docker.Service{
			Name: d.Name + "_" + name, Labels: labels, Image: svc.Image, Replicas: &replicas,
			RunningTasks: running, DesiredTasks: replicas, Update: update, CreatedAt: now,
		}, docker.Task{Slot: 1, State: state, DesiredState: docker.TaskRunning, NodeID: "node-1", CreatedAt: now, UpdatedAt: now})
	}
	return nil
}

// deployEnv es el contenedor completo con miniSwarm, corriendo (c.Run en una
// goroutine que el test cancela y espera antes de cerrar la base), y un
// usuario con su sesión.
type deployEnv struct {
	api
	f     *dockertest.Fake
	swarm *miniSwarm
	s     httptestutil.Opt // la sesión del admin
}

func newDeployEnv(t *testing.T) *deployEnv {
	t.Helper()
	f := dockertest.New()
	c, err := New(context.Background(), testSettings(t), testLogger(t), WithDocker(f))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	swarm := &miniSwarm{f: f}
	f.OnDeployStack = swarm.deploy
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	e := &deployEnv{api: api{t: t, c: c}, f: f, swarm: swarm}
	e.s = e.setup()
	return e
}

const (
	webPath    = "/api/v1/projects/p/stages/staging/apps/web"
	digestWeb1 = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	digestWeb2 = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

// seedWeb crea por HTTP el proyecto p con los stages staging y production, y
// en staging la app de formulario web (nginx:alpine, puerto 80).
func (e *deployEnv) seedWeb() {
	e.t.Helper()
	e.create(e.s, "/api/v1/projects", map[string]string{"slug": "p", "name": "P"})
	e.create(e.s, "/api/v1/projects/p/stages", map[string]string{"slug": "staging"})
	e.create(e.s, "/api/v1/projects/p/stages", map[string]string{"slug": "production"})
	e.create(e.s, "/api/v1/projects/p/stages/staging/apps",
		map[string]any{"slug": "web", "kind": "service", "spec": map[string]any{"image": "nginx:alpine", "port": 80}})
	e.f.SetDigest("nginx:alpine", digestWeb1)
}

// startDeploy hace POST {app}/deploys con el Bearer y devuelve el id.
func (e *deployEnv) startDeploy(token, appPath string, body any) int64 {
	e.t.Helper()
	w := e.bearer(token, "POST", appPath+"/deploys", body)
	httptestutil.Expect(e.t, w, http.StatusAccepted)
	return int64(httptestutil.Decode[map[string]any](e.t, w)["id"].(float64))
}

// waitDeploy lee el log por polling (contrato §2.5) hasta que el deploy
// termina y devuelve su estado final y el log entero.
func (e *deployEnv) waitDeploy(token, appPath string, id int64) (string, string) {
	e.t.Helper()
	var log strings.Builder
	offset := 0
	deadline := time.Now().Add(30 * time.Second)
	for {
		w := e.bearer(token, "GET", fmt.Sprintf("%s/deploys/%d/log?offset=%d", appPath, id, offset), nil)
		httptestutil.Expect(e.t, w, http.StatusOK)
		page := httptestutil.Decode[struct {
			Chunk      string `json:"chunk"`
			NextOffset int    `json:"next_offset"`
			Status     string `json:"status"`
		}](e.t, w)
		log.WriteString(page.Chunk)
		offset = page.NextOffset
		switch page.Status {
		case "succeeded", "failed", "superseded":
			if page.Chunk == "" {
				return page.Status, log.String()
			}
			continue
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("el deploy %d no terminó (%s):\n%s", id, page.Status, log.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// El flujo de un deploy por HTTP con los módulos reales: el token de CI
// despliega, el log llega a succeeded (con la advertencia de Caddy mientras la
// Fase 3 no está, o la línea de las rutas cuando esté), catalog ve el deploy
// (last_deployed_at y pending_changes), el estado sale de Swarm, y el rollback
// vuelve a la versión anterior (con su digest verificado en el paso 2).
func TestContractDeployFlow(t *testing.T) {
	e := newDeployEnv(t)
	e.seedWeb()
	_, tok := e.newToken(e.s, "p", "")

	id := e.startDeploy(tok, webPath, nil)
	status, log := e.waitDeploy(tok, webPath, id)
	if status != "succeeded" {
		t.Fatalf("el primer deploy: %s\n%s", status, log)
	}
	if !strings.Contains(log, "rutas de Caddy actualizadas") && !strings.Contains(log, "advertencia: no se pudo actualizar Caddy:") {
		t.Fatalf("el log no tiene la línea del paso 7:\n%s", log)
	}

	w := e.bearer(tok, "GET", webPath+"/deploys", nil)
	httptestutil.Expect(t, w, http.StatusOK)
	list := httptestutil.Decode[[]map[string]any](t, w)
	if len(list) != 1 || list[0]["images"].(map[string]any)["web"] != "nginx:alpine@"+digestWeb1 || list[0]["actor_type"] != "token" {
		t.Fatalf("GET …/deploys: %s", w.Body)
	}
	w = e.bearer(tok, "GET", webPath, nil)
	httptestutil.Expect(t, w, http.StatusOK)
	if app := httptestutil.Decode[map[string]any](t, w); app["last_deployed_at"] == nil || app["pending_changes"] != false {
		t.Fatalf("catalog no vio el deploy: %s", w.Body)
	}
	w = e.bearer(tok, "GET", webPath+"/status", nil)
	httptestutil.Expect(t, w, http.StatusOK)
	st := httptestutil.Decode[map[string][]map[string]any](t, w)
	if svcs := st["services"]; len(svcs) != 1 || svcs[0]["name"] != "web" || svcs[0]["image"] != "nginx:alpine@"+digestWeb1 {
		t.Fatalf("GET …/status: %s", w.Body)
	}
	expect(t, "rollback sin anterior", e.bearer(tok, "POST", webPath+"/rollback", nil), http.StatusConflict, "no_previous_deploy")

	// Un segundo deploy con otra imagen, y el rollback a la primera.
	e.f.SetDigest("nginx:1.27", digestWeb2)
	id2 := e.startDeploy(tok, webPath, map[string]string{"image": "nginx:1.27"})
	if status, log := e.waitDeploy(tok, webPath, id2); status != "succeeded" {
		t.Fatalf("el segundo deploy: %s\n%s", status, log)
	}
	e.f.SetDigest("nginx:alpine@"+digestWeb1, digestWeb1) // el paso 2 verifica la ref con digest
	w = e.bearer(tok, "POST", webPath+"/rollback", nil)
	httptestutil.Expect(t, w, http.StatusAccepted)
	rb := int64(httptestutil.Decode[map[string]any](t, w)["id"].(float64))
	if status, log := e.waitDeploy(tok, webPath, rb); status != "succeeded" {
		t.Fatalf("el rollback: %s\n%s", status, log)
	}
	w = e.bearer(tok, "GET", fmt.Sprintf("%s/deploys/%d", webPath, rb), nil)
	httptestutil.Expect(t, w, http.StatusOK)
	if d := httptestutil.Decode[map[string]any](t, w); d["trigger"] != "rollback" || d["images"].(map[string]any)["web"] != "nginx:alpine@"+digestWeb1 {
		t.Fatalf("el rollback: %s", w.Body)
	}
	svc, err := e.f.InspectService(context.Background(), "p_staging_web_web")
	if err != nil || svc.Image != "nginx:alpine@"+digestWeb1 {
		t.Fatalf("Swarm después del rollback: %+v, %v", svc, err)
	}
}

// Borrar una app (catalog) con un deploy esperando la convergencia (paso 6)
// detiene su worker, hace docker stack rm, y la fila de la app se lleva sus
// deploys (ON DELETE CASCADE).
func TestContractDeleteAppStopsDeploys(t *testing.T) {
	e := newDeployEnv(t)
	e.seedWeb()
	_, tok := e.newToken(e.s, "p", "")
	e.swarm.setHold(true)
	id := e.startDeploy(tok, webPath, nil)
	deadline := time.Now().Add(15 * time.Second)
	for {
		w := e.bearer(tok, "GET", fmt.Sprintf("%s/deploys/%d/log", webPath, id), nil)
		if strings.Contains(w.Body.String(), "esperando convergencia") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("el deploy no llegó al paso 6: %s", w.Body)
		}
		time.Sleep(50 * time.Millisecond)
	}
	appID := e.queryInt(`SELECT id FROM apps WHERE slug = 'web'`)
	httptestutil.Expect(t, e.do("DELETE", webPath, nil, e.s), http.StatusNoContent)
	if n := e.queryInt(`SELECT COUNT(*) FROM deploys WHERE app_id = ?`, appID); n != 0 {
		t.Fatalf("quedaron %d deploys de la app", n)
	}
	if got := e.f.RemovedStacks(); !slices.Contains(got, "p_staging_web") {
		t.Fatalf("docker stack rm: %v", got)
	}
	if list, _ := e.f.ListServices(context.Background(), map[string]string{"com.docker.stack.namespace": "p_staging_web"}); len(list) != 0 {
		t.Fatalf("quedaron servicios del stack: %+v", list)
	}
}

// Los alcances de los tokens en las rutas de deploy (§10.1): uno de
// p/staging lee y despliega en su stage, no en otro stage ni en otro
// proyecto; /registries es solo con sesión.
func TestContractDeployScopes(t *testing.T) {
	e := newDeployEnv(t)
	e.seedWeb()
	e.create(e.s, "/api/v1/projects/p/stages/production/apps",
		map[string]any{"slug": "web", "kind": "service", "spec": map[string]any{"image": "nginx:alpine", "port": 80}})
	e.create(e.s, "/api/v1/projects", map[string]string{"slug": "otro", "name": "Otro"})
	e.create(e.s, "/api/v1/projects/otro/stages", map[string]string{"slug": "staging"})
	e.create(e.s, "/api/v1/projects/otro/stages/staging/apps",
		map[string]any{"slug": "web", "kind": "service", "spec": map[string]any{"image": "nginx:alpine", "port": 80}})
	_, tok := e.newToken(e.s, "p", "staging")

	expect(t, "su stage: GET deploys", e.bearer(tok, "GET", webPath+"/deploys", nil), http.StatusOK, "")
	expect(t, "su stage: GET status", e.bearer(tok, "GET", webPath+"/status", nil), http.StatusOK, "")
	id := e.startDeploy(tok, webPath, nil)
	if status, log := e.waitDeploy(tok, webPath, id); status != "succeeded" {
		t.Fatalf("deploy en su stage: %s\n%s", status, log)
	}
	for _, path := range []string{"/api/v1/projects/p/stages/production/apps/web", "/api/v1/projects/otro/stages/staging/apps/web"} {
		for _, r := range []struct{ method, path string }{
			{"GET", "/deploys"}, {"POST", "/deploys"}, {"GET", "/deploys/1"}, {"GET", "/deploys/1/log"},
			{"GET", "/deploys/1/log/stream"}, {"POST", "/rollback"}, {"GET", "/status"}, {"GET", "/logs?service=web"},
		} {
			expect(t, "token de p/staging en "+path+r.path, e.bearer(tok, r.method, path+r.path, nil), http.StatusForbidden, "out_of_scope")
		}
	}
	for _, r := range []struct{ method, path string }{
		{"GET", "/api/v1/registries"}, {"POST", "/api/v1/registries"},
		{"PATCH", "/api/v1/registries/ghcr.io"}, {"DELETE", "/api/v1/registries/ghcr.io"},
	} {
		expect(t, "token en "+r.path, e.bearer(tok, r.method, r.path, map[string]string{}), http.StatusForbidden, "token_not_allowed")
	}
}

// El SSE del log de un deploy por el servidor del contenedor (contrato §2.6),
// hasta el status final.
func TestContractDeployLogStream(t *testing.T) {
	e := newDeployEnv(t)
	e.seedWeb()
	_, tok := e.newToken(e.s, "p", "")
	srv := httptest.NewServer(e.c.Server)
	t.Cleanup(srv.Close)
	id := e.startDeploy(tok, webPath, nil)

	req, err := http.NewRequest("GET", fmt.Sprintf("%s%s/deploys/%d/log/stream", srv.URL, webPath, id), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	// Lee hasta que el servidor cierra (después del status final).
	done := make(chan []byte, 1)
	go func() { b, _ := io.ReadAll(resp.Body); done <- b }()
	var body string
	select {
	case b := <-done:
		body = string(b)
	case <-time.After(30 * time.Second):
		t.Fatal("el stream no cerró")
	}
	events := strings.Split(strings.TrimSpace(body), "\n\n")
	last := events[len(events)-1]
	if last != "event: status\ndata: succeeded" {
		t.Fatalf("el último evento: %q\n%s", last, body)
	}
	_, polled := e.waitDeploy(tok, webPath, id)
	var lines []string
	for _, ev := range events {
		if strings.HasPrefix(ev, "event: log\n") {
			lines = append(lines, ev[strings.Index(ev, "data: ")+len("data: "):])
		}
	}
	if got := strings.Join(lines, "\n") + "\n"; got != polled {
		t.Fatalf("el stream no trae los mismos bytes que el polling:\n%s\n---\n%s", got, polled)
	}
}
