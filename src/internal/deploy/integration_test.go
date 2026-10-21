//go:build integration

// Tests del worker de deploy contra el Swarm real (este nodo, manager de un
// Swarm activo):
//
//	go test -tags integration -run Integration -count=1 -timeout 25m ./src/internal/deploy/
//
// Todo lo que crean se llama it2… (un proyecto it2 + 4 hex por test: sus
// stacks, servicios, redes y volúmenes empiezan con eso) y la red lleva la
// etiqueta muelle.test=it2. Se borra al terminar, también si el test falla; un
// -timeout que vence no corre los t.Cleanup, y lo barre el TestMain de la
// corrida siguiente (integration_main_test.go). Nunca publican puertos del
// host ni usan los nombres fijos de muelle. Usan imágenes que ya están en el
// nodo (nginx:alpine y alpine/socat:latest, con RepoDigests): si faltan, el
// test se salta, no las baja.
//
// Para limpiar a mano lo que haya quedado (solo toca lo que empieza con it2):
//
//	bash -c '
//	docker stack ls --format "{{.Name}}" | grep "^it2" | xargs -r -n1 docker stack rm
//	for i in $(seq 60); do docker ps -a --format "{{.Label \"com.docker.stack.namespace\"}}" | grep -q "^it2" || break; sleep 2; done
//	docker network ls --format "{{.Name}}" | grep "^it2" | xargs -r docker network rm
//	docker volume ls --format "{{.Name}}" | grep "^it2" | xargs -r docker volume rm'
package deploy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/distribution/reference"

	httpserver "github.com/CPU-commits/muelle/src/cmd/http"
	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	depsqlite "github.com/CPU-commits/muelle/src/internal/deploy/infra/out/repository/sqlite"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

const (
	itPrefix  = "it2"
	itLabel   = "muelle.test"
	nginxImg  = "nginx:alpine"
	socatImg  = "alpine/socat:latest"
	itTimeout = 3 * time.Minute // lo que espera un test a que un deploy cierre
)

// itClient es el cliente real, o salta el test si no hay un Swarm activo con
// este nodo como manager.
func itClient(t *testing.T) *docker.Client {
	t.Helper()
	c, err := docker.New(docker.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	info, err := c.Info(context.Background())
	if err != nil {
		t.Skipf("sin Docker: %v", err)
	}
	if info.SwarmState != docker.SwarmActive || !info.IsManager {
		t.Skipf("el nodo no es manager de un Swarm activo (%s)", info.SwarmState)
	}
	return c
}

// requireImage salta el test si la imagen no está en el nodo o no tiene
// RepoDigests (una construida o cargada con docker load no tiene digest de
// registry, y localDigests no tendría qué devolver).
func requireImage(t *testing.T, c *docker.Client, ref string) {
	t.Helper()
	img, err := c.InspectImage(context.Background(), ref)
	if err != nil {
		t.Skipf("falta la imagen %s en el nodo (%v); bájala con docker pull", ref, err)
	}
	if len(img.RepoDigests) == 0 {
		t.Skipf("la imagen %s no tiene RepoDigests (¿docker load?); bájala con docker pull", ref)
	}
}

// itProject es un proyecto it2 + 4 hex.
func itProject(t *testing.T) string {
	t.Helper()
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return itPrefix + hex.EncodeToString(b)
}

// localDigests es el cliente real, salvo que DistributionDigest no sale a
// internet: devuelve el digest de RepoDigests de la imagen del nodo. Como
// docker stack deploy va con --resolve-image=never y la ref resuelta trae ese
// digest, Swarm no baja nada. Una ref con digest (un redeploy, un rollback)
// también pasa por acá, como en producción (decisión 1 del plan): se busca
// sin el tag (<repo>@sha256:…, la forma de RepoDigests) y devuelve ese mismo
// digest; si no está en el nodo, docker.ErrNotFound.
type localDigests struct{ *docker.Client }

func (l localDigests) DistributionDigest(ctx context.Context, ref string, auth *docker.RegistryAuth) (string, error) {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return "", fmt.Errorf("localDigests: %s: %w", ref, docker.ErrNotFound)
	}
	lookup := ref
	if d, ok := named.(reference.Digested); ok {
		lookup = reference.FamiliarName(named) + "@" + d.Digest().String()
	}
	img, err := l.InspectImage(ctx, lookup)
	if err != nil {
		return "", err
	}
	for _, rd := range img.RepoDigests {
		rn, err := reference.ParseNormalizedNamed(rd)
		if err != nil || rn.Name() != named.Name() {
			continue
		}
		if d, ok := rn.(reference.Digested); ok {
			if pinned, ok := named.(reference.Digested); ok && pinned.Digest() != d.Digest() {
				continue
			}
			return d.Digest().String(), nil
		}
	}
	return "", fmt.Errorf("localDigests: %s no tiene RepoDigests de %s: %w", ref, named.Name(), docker.ErrNotFound)
}

// itIngress crea la red del stage (overlay attachable, con muelle.test=it2)
// y no tiene rutas que actualizar.
type itIngress struct{ c *docker.Client }

func (i itIngress) EnsureStageNetwork(ctx context.Context, project, stage string) error {
	name := naming.Network(project, stage)
	if !strings.HasPrefix(name, itPrefix) {
		return fmt.Errorf("itIngress: %s no es una red de los tests", name)
	}
	if _, err := i.c.InspectNetwork(ctx, name); err == nil {
		return nil
	} else if !errors.Is(err, docker.ErrNotFound) {
		return err
	}
	_, err := i.c.CreateNetwork(ctx, docker.NetworkSpec{Name: name, Driver: "overlay", Attachable: true,
		Labels: map[string]string{itLabel: "it2"}})
	if errors.Is(err, docker.ErrConflict) {
		return nil
	}
	return err
}

func (itIngress) SyncRoutes(ctx context.Context) error { return nil }

// itEnv es un proyecto de los tests con su base: el stage staging sembrado
// por SQL y el cliente real.
type itEnv struct {
	t       *testing.T
	c       *docker.Client
	p       string
	db      *sqlite.DB
	deploys *depsqlite.DeployRepository
	stageID int64
}

// newItEnv arma el proyecto y registra su limpieza (también si el test
// falla): stack rm de sus stacks, esperar a que no quede ningún servicio del
// proyecto, y borrar la red y los volúmenes <p>_…, reintentando mientras
// Swarm los suelta.
func newItEnv(t *testing.T) *itEnv {
	t.Helper()
	c := itClient(t)
	requireImage(t, c, nginxImg)
	e := &itEnv{t: t, c: c, p: itProject(t), db: sqlitetest.New(t)}
	e.deploys = depsqlite.NewDeployRepository(e.db)
	t.Cleanup(e.cleanup)
	e.exec(`INSERT INTO projects (slug, name, created_at) VALUES (?, 'it2', 0)`, e.p)
	e.stageID = e.exec(`INSERT INTO stages (project_id, slug, created_at) SELECT id, 'staging', 0 FROM projects`)
	return e
}

func (e *itEnv) exec(query string, args ...any) int64 {
	e.t.Helper()
	ctx := context.Background()
	res, err := e.db.Conn(ctx).ExecContext(ctx, query, args...)
	if err != nil {
		e.t.Fatalf("%s: %v", query, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func (e *itEnv) formApp(slug, spec string) {
	e.exec(`INSERT INTO apps (stage_id, slug, kind, spec, deploy_timeout_s, config_changed_at, created_at)
		VALUES (?, ?, 'service', ?, 120, 0, 0)`, e.stageID, slug, spec)
}

func (e *itEnv) composeApp(slug, yaml string) {
	e.exec(`INSERT INTO apps (stage_id, slug, kind, compose_yaml, deploy_timeout_s, config_changed_at, created_at)
		VALUES (?, ?, 'compose', ?, 120, 0, 0)`, e.stageID, slug, yaml)
}

func (e *itEnv) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	byProject := map[string]string{naming.LabelProject: e.p}
	if svcs, err := e.c.ListServices(ctx, byProject); err == nil {
		stacks := map[string]bool{}
		for _, s := range svcs {
			if st := s.Labels[naming.LabelStackNamespace]; strings.HasPrefix(st, e.p+"_") {
				stacks[st] = true
			}
		}
		for st := range stacks {
			if out, err := e.c.RemoveStack(ctx, st); err != nil {
				e.t.Logf("limpieza: docker stack rm %s: %v\n%s", st, err, out)
			}
		}
	}
	retry(ctx, 90*time.Second, func() bool {
		svcs, err := e.c.ListServices(ctx, byProject)
		return err == nil && len(svcs) == 0
	})
	// Una red overlay se borra del Swarm enseguida, pero el nodo la sigue
	// listando un momento: se espera a que ya no esté.
	network := naming.Network(e.p, "staging")
	retry(ctx, 60*time.Second, func() bool {
		err := e.c.RemoveNetwork(ctx, network)
		if err != nil && !errors.Is(err, docker.ErrNotFound) {
			return false
		}
		_, err = e.c.InspectNetwork(ctx, network)
		return errors.Is(err, docker.ErrNotFound)
	})
	retry(ctx, 60*time.Second, func() bool {
		vols, err := e.c.ListVolumes(ctx, nil)
		if err != nil {
			return false
		}
		left := false
		for _, v := range vols {
			if strings.HasPrefix(v.Name, e.p+"_") {
				if err := e.c.RemoveVolume(ctx, v.Name, false); err != nil && !errors.Is(err, docker.ErrNotFound) {
					left = true
				}
			}
		}
		return !left
	})
}

// retry repite ok cada 2 s hasta que da true o pasa d.
func retry(ctx context.Context, d time.Duration, ok func() bool) bool {
	deadline := time.Now().Add(d)
	for {
		if ok() {
			return true
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return false
		}
		time.Sleep(2 * time.Second)
	}
}

// itModule es el módulo sobre la base del proyecto, con el Docker real (sin
// salir a internet para los digests), corriendo (Run) hasta que se llama a
// su stop (o termina el test).
type itModule struct {
	m    *Module
	http *httptestutil.Client
	stop func()
}

func (e *itEnv) start() *itModule {
	e.t.Helper()
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		e.t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := New(Deps{DB: e.db, Cipher: c, Clock: clock.System{}, Audit: sqlite.NewAuditRecorder(e.db, clock.System{}, log), Log: log,
		Docker: localDigests{e.c}, Env: fakeEnv{}, Ingress: itIngress{e.c}})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	stop := func() { cancel(); <-done }
	e.t.Cleanup(stop)
	return &itModule{m: m, http: httptestutil.New(e.t, testAuth, m.RegisterRoutes), stop: stop}
}

func (e *itEnv) appPath(slug string) string {
	return fmt.Sprintf("/api/v1/projects/%s/stages/staging/apps/%s", e.p, slug)
}

// deploy pide un deploy de la app por HTTP y devuelve su id.
func (e *itEnv) deploy(im *itModule, slug string) int64 {
	e.t.Helper()
	w := im.http.Do("POST", e.appPath(slug)+"/deploys", nil, session())
	httptestutil.Expect(e.t, w, http.StatusAccepted)
	return httptestutil.Decode[queuedJSON](e.t, w).ID
}

// wait espera a que el deploy cierre y lo devuelve.
func (e *itEnv) wait(id int64) entity.Deploy {
	e.t.Helper()
	deadline := time.Now().Add(itTimeout)
	for {
		d, err := e.deploys.GetByID(context.Background(), id)
		if err != nil {
			e.t.Fatal(err)
		}
		if d.Status.Final() {
			return d
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("el deploy %d no cerró en %v (%s):\n%s", id, itTimeout, d.Status, e.log(id))
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (e *itEnv) waitOK(id int64) entity.Deploy {
	e.t.Helper()
	d := e.wait(id)
	if d.Status != entity.StatusSucceeded {
		e.t.Fatalf("deploy %d: %s (%v)\n%s", id, d.Status, d.Error, e.log(id))
	}
	return d
}

func (e *itEnv) log(id int64) string {
	var s string
	ctx := context.Background()
	_ = e.db.Conn(ctx).QueryRowContext(ctx, `SELECT log FROM deploys WHERE id = ?`, id).Scan(&s)
	return s
}

func (e *itEnv) service(app, svc string) docker.Service {
	e.t.Helper()
	s, err := e.c.InspectService(context.Background(), naming.Service(e.p, "staging", app, svc))
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

func (e *itEnv) runningTasks(app, svc string) []string {
	e.t.Helper()
	tasks, err := e.c.ListTasks(context.Background(), docker.TaskFilter{Service: naming.Service(e.p, "staging", app, svc), DesiredState: docker.TaskRunning})
	if err != nil {
		e.t.Fatal(err)
	}
	var ids []string
	for _, t := range tasks {
		ids = append(ids, t.ID)
	}
	slices.Sort(ids)
	return ids
}

// Una app de formulario con nginx y healthcheck http: converge 1/1, y un
// redeploy sin cambios no reinicia la tarea (decisión 6).
func TestIntegrationFormAppDeploy(t *testing.T) {
	e := newItEnv(t)
	e.formApp("web", `{"image":"nginx:alpine","port":80,"replicas":1,
		"healthcheck":{"type":"http","path":"/","interval_s":1,"timeout_s":2,"retries":3}}`)
	im := e.start()
	d := e.waitOK(e.deploy(im, "web"))
	if !strings.HasPrefix(d.Images["web"], "nginx:alpine@sha256:") {
		t.Fatalf("images: %v", d.Images)
	}
	s := e.service("web", "web")
	if s.Replicas == nil || *s.Replicas != 1 || s.RunningTasks != 1 || s.Labels[naming.LabelDeploy] != strconv.FormatInt(d.ID, 10) {
		t.Fatalf("servicio: %+v", s)
	}
	before := e.runningTasks("web", "web")

	d2 := e.waitOK(e.deploy(im, "web"))
	if after := e.runningTasks("web", "web"); !slices.Equal(before, after) || len(after) != 1 {
		t.Fatalf("el redeploy sin cambios reinició la tarea: %v → %v", before, after)
	}
	if got := e.service("web", "web").Labels[naming.LabelDeploy]; got != strconv.FormatInt(d2.ID, 10) {
		t.Fatalf("muelle.deploy = %s", got)
	}
}

// Un compose con dos servicios; el mismo sin uno de ellos lo borra (--prune).
func TestIntegrationComposePrune(t *testing.T) {
	e := newItEnv(t)
	requireImage(t, e.c, socatImg)
	e.composeApp("front", `services:
  web:
    image: nginx:alpine
  side:
    image: alpine/socat:latest
    command: ["TCP-LISTEN:9000,fork", "SYSTEM:echo"]
`)
	im := e.start()
	e.waitOK(e.deploy(im, "front"))
	if s := e.service("front", "side"); s.RunningTasks != 1 {
		t.Fatalf("side: %+v", s)
	}
	e.exec(`UPDATE apps SET compose_yaml = ? WHERE slug = 'front'`, "services:\n  web:\n    image: nginx:alpine\n")
	e.waitOK(e.deploy(im, "front"))
	if _, err := e.c.InspectService(context.Background(), naming.Service(e.p, "staging", "front", "side")); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("side sigue en Swarm: %v", err)
	}
	if s := e.service("front", "web"); s.RunningTasks != 1 {
		t.Fatalf("web: %+v", s)
	}
}

// Una versión que no pasa el healthcheck: Swarm hace rollback y el deploy
// falla con el Err de la tarea; el servicio vuelve a llevar el muelle.deploy
// de la versión anterior.
func TestIntegrationFailingHealthcheckRollsBack(t *testing.T) {
	e := newItEnv(t)
	compose := func(path string) string {
		return `services:
  web:
    image: nginx:alpine
    healthcheck:
      test: ["CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1` + path + `"]
      interval: 1s
      timeout: 2s
      retries: 2
      start_period: 1s
    deploy:
      update_config:
        monitor: 5s
`
	}
	e.composeApp("hc", compose("/"))
	im := e.start()
	v1 := e.waitOK(e.deploy(im, "hc"))

	e.exec(`UPDATE apps SET compose_yaml = ? WHERE slug = 'hc'`, compose("/no-existe"))
	v2 := e.wait(e.deploy(im, "hc"))
	if v2.Status != entity.StatusFailed || v2.Error == nil || !strings.Contains(*v2.Error, "web: Swarm hizo rollback") {
		t.Fatalf("v2: %s %v\n%s", v2.Status, v2.Error, e.log(v2.ID))
	}
	if !strings.Contains(e.log(v2.ID), "web: última tarea fallida: ") {
		t.Fatalf("falta el Err de la tarea:\n%s", e.log(v2.ID))
	}
	want := strconv.FormatInt(v1.ID, 10)
	if !retry(context.Background(), 90*time.Second, func() bool {
		s, err := e.c.InspectService(context.Background(), naming.Service(e.p, "staging", "hc", "web"))
		return err == nil && s.Labels[naming.LabelDeploy] == want
	}) {
		t.Fatalf("el servicio no volvió a v1: %v", e.service("hc", "web").Labels)
	}
}

// Un servicio con volumen: stop-first, fijo al nodo de los volúmenes (§6.2),
// y el volumen <stack>_data existe.
func TestIntegrationVolumeStopFirst(t *testing.T) {
	e := newItEnv(t)
	e.formApp("db", `{"image":"nginx:alpine","port":80,"volumes":[{"name":"data","mount":"/data"}]}`)
	im := e.start()
	e.waitOK(e.deploy(im, "db"))
	info, err := e.c.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	name := naming.Service(e.p, "staging", "db", "db")
	out, err := exec.Command("docker", "service", "inspect", "--format",
		"{{.Spec.UpdateConfig.Order}} {{.Spec.TaskTemplate.Placement.Constraints}}", name).Output()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(string(out)), "stop-first [node.id == "+info.NodeID+"]"; got != want {
		t.Fatalf("docker service inspect: %q, esperaba %q", got, want)
	}
	vols, err := e.c.ListVolumes(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	vol := naming.Volume(naming.Stack(e.p, "staging", "db"), "data")
	if !slices.ContainsFunc(vols, func(v docker.Volume) bool { return v.Name == vol }) {
		t.Fatalf("falta el volumen %s", vol)
	}
}

// El apagado durante la convergencia deja el deploy running; el próximo
// arranque (un módulo nuevo sobre la misma base) lo cierra succeeded con
// "muelle se reinició durante el deploy" (contrato §1.2).
func TestIntegrationResumeAfterRestart(t *testing.T) {
	e := newItEnv(t)
	e.formApp("slow", `{"image":"nginx:alpine","port":80,
		"healthcheck":{"type":"http","path":"/","interval_s":5,"timeout_s":2,"retries":3}}`)
	im := e.start()
	id := e.deploy(im, "slow")
	if !retry(context.Background(), 2*time.Minute, func() bool { return strings.Contains(e.log(id), "esperando convergencia") }) {
		t.Fatalf("no llegó a la convergencia:\n%s", e.log(id))
	}
	im.stop()
	d, err := e.deploys.GetByID(context.Background(), id)
	if err != nil || d.Status != entity.StatusRunning {
		t.Fatalf("después del apagado: %s %v\n%s", d.Status, err, e.log(id))
	}

	e.start()
	d = e.waitOK(id)
	if !strings.Contains(e.log(id), "muelle se reinició durante el deploy") {
		t.Fatalf("falta la línea del reinicio:\n%s", e.log(id))
	}
	if s := e.service("slow", "slow"); s.RunningTasks != 1 {
		t.Fatalf("servicio: %+v", s)
	}
}

// El estado y los logs de una app con nginx desplegado (contrato §2.8 y
// §2.9): GET …/status da 1/1, health healthy y las medidas de la tarea (corre
// en este nodo); GET …/logs sin follow trae las líneas de nginx y end eof; y
// con since = la hora de la última, esa línea vuelve a llegar (inclusivo).
func TestIntegrationStatusAndLogs(t *testing.T) {
	e := newItEnv(t)
	e.formApp("web", `{"image":"nginx:alpine","port":80,"replicas":1,
		"healthcheck":{"type":"http","path":"/","interval_s":1,"timeout_s":2,"retries":3}}`)
	im := e.start()
	e.waitOK(e.deploy(im, "web"))

	var svc map[string]any
	if !retry(context.Background(), 30*time.Second, func() bool {
		w := im.http.Do("GET", e.appPath("web")+"/status", nil, session())
		if w.Code != http.StatusOK {
			return false
		}
		svcs := httptestutil.Decode[map[string][]map[string]any](t, w)["services"]
		if len(svcs) != 1 {
			return false
		}
		svc = svcs[0]
		tasks, _ := svc["tasks"].([]any)
		if len(tasks) == 0 {
			return false
		}
		task, _ := tasks[0].(map[string]any)
		return task["health"] == "healthy" && task["cpu_percent"] != nil && task["memory_bytes"] != nil
	}) {
		t.Fatalf("GET …/status: %v", svc)
	}
	if svc["name"] != "web" || svc["replicas_desired"] != 1.0 || svc["replicas_running"] != 1.0 ||
		svc["memory_bytes"] == nil || svc["cpu_percent"] == nil {
		t.Fatalf("servicio: %v", svc)
	}
	task := svc["tasks"].([]any)[0].(map[string]any)
	if task["slot"] != 1.0 || task["state"] != "running" || task["node"] == nil || task["since"] == nil {
		t.Fatalf("tarea: %v", task)
	}

	s := httpserver.NewServer(httpserver.Config{PanelOrigin: httptestutil.PanelOrigin, SecureCookies: true, Auth: testAuth,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	im.m.RegisterRoutes(s.Kit())
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	logs := func(query string) []sseEvent {
		t.Helper()
		req, err := http.NewRequest("GET", srv.URL+e.appPath("web")+"/logs?"+query, nil)
		if err != nil {
			t.Fatal(err)
		}
		session()(req)
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("GET …/logs?%s: %d %s", query, resp.StatusCode, b)
		}
		r := readSSE(t, resp)
		var out []sseEvent
		for {
			ev, ok := r.next(t, 30*time.Second)
			if !ok {
				return out
			}
			out = append(out, ev)
		}
	}
	type line struct {
		Time   string `json:"time"`
		Task   string `json:"task"`
		Stream string `json:"stream"`
		Line   string `json:"line"`
	}
	parse := func(evs []sseEvent) []line {
		t.Helper()
		if len(evs) < 2 || evs[len(evs)-1].Name != "end" || evs[len(evs)-1].Data != "eof" {
			t.Fatalf("sin end eof al final: %+v", evs)
		}
		var out []line
		for _, ev := range evs[:len(evs)-1] {
			if ev.Name != "log" {
				t.Fatalf("evento %+v", ev)
			}
			var l line
			if err := jsonv2.Unmarshal([]byte(ev.Data), &l); err != nil {
				t.Fatalf("data %q: %v", ev.Data, err)
			}
			out = append(out, l)
		}
		return out
	}
	first := parse(logs("service=web&tail=200"))
	if !slices.ContainsFunc(first, func(l line) bool { return strings.Contains(l.Line, "nginx") }) {
		t.Fatalf("no están las líneas de arranque de nginx: %+v", first)
	}
	last := first[len(first)-1]
	lastAt, err := time.Parse(time.RFC3339Nano, last.Time)
	if err != nil || last.Task != "web.1" || (last.Stream != "stdout" && last.Stream != "stderr") ||
		!strings.HasSuffix(last.Time, "Z") || !lastAt.Equal(lastAt.Truncate(time.Millisecond)) {
		t.Fatalf("última línea: %+v (%v)", last, err)
	}
	again := parse(logs("service=web&since=" + last.Time))
	if !slices.Contains(again, last) {
		t.Fatalf("since=%s no trajo de nuevo la última línea %+v: %+v", last.Time, last, again)
	}
	for _, l := range again {
		if at, err := time.Parse(time.RFC3339Nano, l.Time); err != nil || at.Before(lastAt) {
			t.Fatalf("since=%s trajo una línea anterior: %+v", last.Time, l)
		}
	}
}
