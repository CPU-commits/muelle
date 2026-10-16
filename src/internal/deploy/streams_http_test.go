package deploy

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	httpserver "github.com/CPU-commits/muelle/src/cmd/http"
	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/internal/deploy/app/usecases"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	dephttp "github.com/CPU-commits/muelle/src/internal/deploy/infra/in/http"
	depsqlite "github.com/CPU-commits/muelle/src/internal/deploy/infra/out/repository/sqlite"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

// Los streams (contrato §0.3, §2.6 y §2.9) necesitan un servidor de verdad:
// httptestutil.Client usa un ResponseRecorder, que solo se lee cuando el
// handler vuelve.

// lockedAuth es un Authenticator con candado: un stream abierto lo consulta
// desde su goroutine (Kit.Revalidate) mientras el test revoca credenciales.
// httptestutil.FakeAuth tiene mapas sin candado: modificarlo con un servidor
// corriendo es una carrera.
type lockedAuth struct {
	mu       sync.Mutex
	sessions map[string]shared.Principal
	tokens   map[string]shared.Principal
}

// newLockedAuth tiene las sesiones "s" y "s2" (de dos usuarios) y los tokens
// "tp" (del proyecto auditfy), "ts" (de auditfy/staging) y "tx" (de otro).
func newLockedAuth() *lockedAuth {
	return &lockedAuth{
		sessions: map[string]shared.Principal{
			"s":  {UserID: 1, Username: "felipe", SessionHash: "h-s"},
			"s2": {UserID: 2, Username: "ana", SessionHash: "h-s2"},
		},
		tokens: map[string]shared.Principal{
			"tp": {Token: &shared.TokenScope{ID: 4, Name: "github-actions", ProjectSlug: "auditfy"}},
			"ts": {Token: &shared.TokenScope{ID: 6, Name: "staging", ProjectSlug: "auditfy", StageSlug: "staging"}},
			"tx": {Token: &shared.TokenScope{ID: 5, Name: "otro", ProjectSlug: "otro"}},
		},
	}
}

func (a *lockedAuth) AuthenticateSession(ctx context.Context, token string) (shared.Principal, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if p, ok := a.sessions[token]; ok {
		return p, nil
	}
	return shared.Principal{}, shared.ErrUnauthenticated
}

func (a *lockedAuth) AuthenticateToken(ctx context.Context, bearer string) (shared.Principal, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if p, ok := a.tokens[bearer]; ok {
		return p, nil
	}
	return shared.Principal{}, shared.ErrUnauthenticated
}

// Revoke borra una sesión o un token (cerrar sesión, revocar el token).
func (a *lockedAuth) Revoke(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, token)
	delete(a.tokens, token)
}

// Rescope cambia el alcance de un token (ya no alcanza a lo que alcanzaba).
func (a *lockedAuth) Rescope(token, project string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	p := a.tokens[token]
	scope := *p.Token
	scope.ProjectSlug = project
	p.Token = &scope
	a.tokens[token] = p
}

// testSSE son límites de SSE para los tests: el heartbeat en 1 min, así solo
// un aviso del Hub despierta un stream.
func testSSE() dephttp.SSEConfig {
	return dephttp.SSEConfig{Heartbeat: time.Minute, RevalidateEvery: 4, MaxDuration: time.Hour, WriteTimeout: 5 * time.Second, MaxPerPrincipal: 8}
}

type streamTest struct {
	*httpTest
	auth *lockedAuth
	srv  *httptest.Server
}

// newStreamTest es el módulo con los límites cfg detrás de un httptest.Server
// sobre el servidor de cmd/http (configure cambia su http.Server antes de
// arrancarlo), con lockedAuth. h.c sigue sirviendo para lo que no es un stream.
func newStreamTest(t *testing.T, cfg dephttp.SSEConfig, configure ...func(*http.Server)) *streamTest {
	t.Helper()
	h := newModuleTest(t, fakeEnv{})
	return newStreamTestOn(t, h, cfg, configure...)
}

func newStreamTestOn(t *testing.T, h *httpTest, cfg dephttp.SSEConfig, configure ...func(*http.Server)) *streamTest {
	t.Helper()
	h.m.sse = cfg
	auth := newLockedAuth()
	s := httpserver.NewServer(httpserver.Config{
		PanelOrigin: httptestutil.PanelOrigin, SecureCookies: true, Auth: auth,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	h.m.RegisterRoutes(s.Kit())
	h.c = httptestutil.New(t, auth, h.m.RegisterRoutes)
	srv := httptest.NewUnstartedServer(s)
	for _, f := range configure {
		f(srv.Config)
	}
	srv.Start()
	t.Cleanup(srv.Close)
	// Antes de srv.Close (t.Cleanup va al revés): Close espera a los
	// handlers, y un stream abierto no vuelve solo.
	t.Cleanup(h.m.stopStreams)
	return &streamTest{httpTest: h, auth: auth, srv: srv}
}

// get hace un GET al servidor de verdad; el cuerpo se cierra al terminar el test.
func (h *streamTest) get(t *testing.T, path string, opts ...httptestutil.Opt) *http.Response {
	t.Helper()
	req, err := http.NewRequest("GET", h.srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", httptestutil.PanelOrigin)
	for _, o := range opts {
		o(req)
	}
	resp, err := h.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// stream abre un SSE que tiene que responder 200.
func (h *streamTest) stream(t *testing.T, path string, opts ...httptestutil.Opt) *sseReader {
	t.Helper()
	resp := h.get(t, path, opts...)
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s: %d %s", path, resp.StatusCode, b)
	}
	return readSSE(t, resp)
}

type sseEvent struct {
	Name, ID, Data string
	Comment        string // ": ping" es "ping"
}

// sseReader lee los eventos de un stream en una goroutine.
type sseReader struct {
	resp   *http.Response
	events chan sseEvent // se cierra cuando el stream termina
}

func readSSE(t *testing.T, resp *http.Response) *sseReader {
	r := &sseReader{resp: resp, events: make(chan sseEvent, 4096)}
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop); resp.Body.Close() })
	go func() {
		defer close(r.events)
		br := bufio.NewReader(resp.Body)
		var ev sseEvent
		has := false
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSuffix(line, "\n")
			switch {
			case line == "":
				if has {
					select {
					case r.events <- ev:
					case <-stop:
						return
					}
				}
				ev, has = sseEvent{}, false
				continue
			case strings.HasPrefix(line, ":"):
				ev.Comment = strings.TrimPrefix(line[1:], " ")
			default:
				k, v, _ := strings.Cut(line, ":")
				v = strings.TrimPrefix(v, " ")
				switch k {
				case "event":
					ev.Name = v
				case "id":
					ev.ID = v
				case "data":
					ev.Data = v
				}
			}
			has = true
		}
	}()
	return r
}

// next es el próximo evento que no es un comentario; ok es false si el
// stream se cerró. Falla si no llega nada en d.
func (r *sseReader) next(t *testing.T, d time.Duration) (sseEvent, bool) {
	t.Helper()
	timeout := time.After(d)
	for {
		select {
		case ev, ok := <-r.events:
			if !ok {
				return sseEvent{}, false
			}
			if ev.Comment != "" && ev.Name == "" {
				continue
			}
			return ev, true
		case <-timeout:
			t.Fatalf("no llegó ningún evento en %s", d)
			return sseEvent{}, false
		}
	}
}

// expect es el próximo evento, que tiene que llamarse name (y, si data no es
// "", tener ese data).
func (r *sseReader) expect(t *testing.T, name, data string) sseEvent {
	t.Helper()
	ev, ok := r.next(t, 5*time.Second)
	if !ok {
		t.Fatalf("el stream se cerró; esperaba %s %q", name, data)
	}
	if ev.Name != name || (data != "" && ev.Data != data) {
		t.Fatalf("evento %+v; esperaba %s %q", ev, name, data)
	}
	return ev
}

// expectClosed espera que el stream se cierre en d sin otro evento que
// comentarios.
func (r *sseReader) expectClosed(t *testing.T, d time.Duration) {
	t.Helper()
	if ev, ok := r.next(t, d); ok {
		t.Fatalf("esperaba el cierre del stream, llegó %+v", ev)
	}
}

// comment espera un comentario (": ping") en d.
func (r *sseReader) comment(t *testing.T, d time.Duration) string {
	t.Helper()
	timeout := time.After(d)
	for {
		select {
		case ev, ok := <-r.events:
			if !ok {
				t.Fatal("el stream se cerró antes del comentario")
			}
			if ev.Comment != "" {
				return ev.Comment
			}
		case <-timeout:
			t.Fatalf("no llegó ningún comentario en %s", d)
		}
	}
}

func sessionOf(token string) httptestutil.Opt { return httptestutil.WithSession(token) }
func bearer(token string) httptestutil.Opt    { return httptestutil.WithBearer(token) }

// queuedDeploy encola un deploy de api con la sesión y devuelve su id.
func (h *streamTest) queuedDeploy(t *testing.T) int64 {
	t.Helper()
	return expectQueued(t, h.c.Do("POST", apiPath+"/deploys", nil, sessionOf("s")), apiPath).ID
}

func logStreamPath(id int64) string {
	return apiPath + "/deploys/" + strconv.FormatInt(id, 10) + "/log/stream"
}

// --- El SSE común (contrato §0.3) ------------------------------------------------------

func TestSSEHeaders(t *testing.T) {
	h := newStreamTest(t, testSSE())
	h.seed(t)
	id := h.queuedDeploy(t)
	resp := h.get(t, logStreamPath(id), sessionOf("s"))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	for k, want := range map[string]string{
		"Content-Type": "text/event-stream", "Cache-Control": "no-cache", "X-Accel-Buffering": "no",
	} {
		if got := resp.Header.Get(k); got != want {
			t.Errorf("%s: %q, esperaba %q", k, got, want)
		}
	}
	readSSE(t, resp).expect(t, "status", "queued")
}

func TestSSEHeartbeat(t *testing.T) {
	cfg := testSSE()
	cfg.Heartbeat = 30 * time.Millisecond
	h := newStreamTest(t, cfg)
	h.seed(t)
	r := h.stream(t, logStreamPath(h.queuedDeploy(t)), sessionOf("s"))
	r.expect(t, "status", "queued")
	for range 3 {
		if c := r.comment(t, 2*time.Second); c != "ping" {
			t.Fatalf("comentario %q", c)
		}
	}
}

// El ReadTimeout del servidor (30 s en main.go) cancelaría el ctx de la
// petición al vencer (decisión 11 del plan): el stream le quita el plazo de
// lectura a la conexión.
func TestSSESurvivesReadTimeout(t *testing.T) {
	cfg := testSSE()
	cfg.Heartbeat = 50 * time.Millisecond
	h := newStreamTest(t, cfg, func(s *http.Server) { s.ReadTimeout = 200 * time.Millisecond })
	h.seed(t)
	r := h.stream(t, logStreamPath(h.queuedDeploy(t)), sessionOf("s"))
	r.expect(t, "status", "queued")
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		r.comment(t, time.Second)
	}
	// Sigue vivo: el próximo heartbeat llega.
	r.comment(t, time.Second)
}

// La credencial se revisa mientras dura el stream: una sesión que se cerró,
// un token revocado o uno que ya no alcanza al proyecto cierran el stream,
// sin mandar nada más.
func TestSSERevalidate(t *testing.T) {
	cfg := testSSE()
	cfg.Heartbeat, cfg.RevalidateEvery = 20*time.Millisecond, 2
	h := newStreamTest(t, cfg)
	h.seed(t)
	id := h.queuedDeploy(t)
	for _, c := range []struct {
		name   string
		opt    httptestutil.Opt
		revoke func()
	}{
		{"sesión cerrada", sessionOf("s"), func() { h.auth.Revoke("s") }},
		{"token revocado", bearer("tp"), func() { h.auth.Revoke("tp") }},
		{"token de otro alcance", bearer("ts"), func() { h.auth.Rescope("ts", "otro") }},
	} {
		r := h.stream(t, logStreamPath(id), c.opt)
		r.expect(t, "status", "queued")
		// Con la credencial vigente pasa varias revisiones.
		for range 5 {
			r.comment(t, time.Second)
		}
		c.revoke()
		r.expectClosed(t, 2*time.Second)
	}
}

// A lo más 8 streams abiertos por principal (los dos tipos suman); el noveno
// es 429 rate_limited con Retry-After, antes de abrir. Un stream que no abrió
// (404) no ocupa lugar, y uno que se cierra lo devuelve.
func TestSSELimitPerPrincipal(t *testing.T) {
	h := newStreamTest(t, testSSE())
	h.seed(t)
	h.putAPIService()
	id := h.queuedDeploy(t)
	for range 10 {
		for _, path := range []string{logStreamPath(id + 100), apiPath + "/logs?service=web"} {
			resp := h.get(t, path, sessionOf("s"))
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("%s: %d", path, resp.StatusCode)
			}
			resp.Body.Close()
		}
	}
	var open []*sseReader
	for i := range 8 {
		if i%2 == 0 {
			r := h.stream(t, logStreamPath(id), sessionOf("s"))
			r.expect(t, "status", "queued")
			open = append(open, r)
			continue
		}
		open = append(open, h.stream(t, logsPath("service=api&follow=1"), sessionOf("s")))
	}
	// Los streams de logs abiertos sin líneas no mandan nada: se espera a
	// que el servidor los cuente (el 200 ya llegó, así que ya están contados).
	if resp := h.get(t, logsPath("service=api&follow=1"), sessionOf("s")); resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("el noveno (logs): %d", resp.StatusCode)
	}
	resp := h.get(t, logStreamPath(id), sessionOf("s"))
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("el noveno: %d (Retry-After %q)", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	if b, _ := io.ReadAll(resp.Body); !strings.Contains(string(b), `"rate_limited"`) {
		t.Fatalf("el noveno: %s", b)
	}
	// Otro principal (otra sesión, un token) sí abre.
	h.stream(t, logStreamPath(id), sessionOf("s2")).expect(t, "status", "queued")
	h.stream(t, logStreamPath(id), bearer("tp")).expect(t, "status", "queued")

	// Cerrar uno devuelve su lugar (cuando el servidor nota el cierre).
	open[0].resp.Body.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp := h.get(t, logStreamPath(id), sessionOf("s"))
		if resp.StatusCode == http.StatusOK {
			readSSE(t, resp).expect(t, "status", "queued")
			break
		}
		resp.Body.Close()
		if time.Now().After(deadline) {
			t.Fatalf("el lugar del stream cerrado no volvió: %d", resp.StatusCode)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Un stream dura a lo más MaxDuration: el de un deploy cierra sin el status
// final (contrato §0.3).
func TestSSEMaxDuration(t *testing.T) {
	cfg := testSSE()
	cfg.MaxDuration = 300 * time.Millisecond
	h := newStreamTest(t, cfg)
	h.seed(t)
	r := h.stream(t, logStreamPath(h.queuedDeploy(t)), sessionOf("s"))
	r.expect(t, "status", "queued")
	r.expectClosed(t, 3*time.Second)
}

// Los streams se cierran con el ctx de Module.Run (la señal de apagado): si
// no, srv.Shutdown esperaría sus 8 s.
func TestSSEClosesOnModuleStop(t *testing.T) {
	h := newStreamTest(t, testSSE())
	api, _, _ := h.seed(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { h.m.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	// Un queued que el despachador no conoce (sin Wake): sigue queued.
	d := &entity.Deploy{AppID: api, Trigger: entity.TriggerPanel, ActorType: entity.ActorUser, ActorName: "felipe", CreatedAt: t0}
	if err := depsqlite.NewDeployRepository(h.db).Create(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	r := h.stream(t, logStreamPath(d.ID), sessionOf("s"))
	r.expect(t, "status", "queued")
	cancel()
	r.expectClosed(t, 2*time.Second)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run no volvió")
	}
}

// --- El log de un deploy en vivo (contrato §2.6) -------------------------------------------

// pollLog es el log entero por polling, con su next_offset.
func (h *streamTest) pollLog(t *testing.T, id int64) (string, int) {
	t.Helper()
	w := h.c.Do("GET", apiPath+"/deploys/"+strconv.FormatInt(id, 10)+"/log", nil, sessionOf("s"))
	httptestutil.Expect(t, w, http.StatusOK)
	res := httptestutil.Decode[struct {
		Chunk      string `json:"chunk"`
		NextOffset int    `json:"next_offset"`
	}](t, w)
	return res.Chunk, res.NextOffset
}

func (h *streamTest) logWriter(id int64) *usecases.DeployLog {
	return usecases.NewLogWriter(depsqlite.NewDeployRepository(h.db), h.m.hub, h.clock).For(id, service.Masker{})
}

// expectLog espera un evento log con ese texto al final y devuelve su id.
func (r *sseReader) expectLog(t *testing.T, text string) int {
	t.Helper()
	ev := r.expect(t, "log", "")
	if !strings.HasSuffix(ev.Data, " "+text) {
		t.Fatalf("log %q; esperaba que termine en %q", ev.Data, text)
	}
	n, err := strconv.Atoi(ev.ID)
	if err != nil {
		t.Fatalf("id %q", ev.ID)
	}
	return n
}

// Las líneas llegan a medida que se escriben, con el id = el offset después
// de cada una; cada cambio de estado llega como status, y el status final va
// después de la última línea.
func TestDeployLogStreamLive(t *testing.T) {
	h := newStreamTest(t, testSSE())
	h.seed(t)
	id := h.queuedDeploy(t)
	r := h.stream(t, logStreamPath(id), sessionOf("s"))
	r.expect(t, "status", "queued")

	ctx := context.Background()
	repo := depsqlite.NewDeployRepository(h.db)
	if ok, err := repo.Claim(ctx, id, h.clock.Now()); err != nil || !ok {
		t.Fatal(ok, err)
	}
	h.m.hub.Notify(id)
	r.expect(t, "status", "running")

	log := h.logWriter(id)
	var ids []int
	for _, text := range []string{"resolviendo imágenes", "render: 1 servicio"} {
		if err := log.Line("%s", text); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, r.expectLog(t, text))
	}
	if err := log.Final("succeeded en 1 s"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Finish(ctx, id, entity.StatusSucceeded, nil, h.clock.Now()); err != nil {
		t.Fatal(err)
	}
	h.m.hub.Notify(id)
	ids = append(ids, r.expectLog(t, "succeeded en 1 s"))
	r.expect(t, "status", "succeeded")
	r.expectClosed(t, 2*time.Second)

	// Los mismos bytes que el polling, y el último id es su next_offset.
	chunk, next := h.pollLog(t, id)
	lines := strings.SplitAfter(chunk, "\n")
	off := 0
	for i, want := range ids {
		off += len(lines[i])
		if want != off {
			t.Fatalf("id de la línea %d: %d, esperaba %d", i, want, off)
		}
	}
	if ids[len(ids)-1] != next {
		t.Fatalf("último id %d, next_offset %d", ids[len(ids)-1], next)
	}
	if n := h.m.hub.Pending(); n != 0 {
		t.Fatalf("el stream dejó %d canales en el Hub", n)
	}
}

// Un deploy que ya terminó: las líneas que faltan, el status final y se
// cierra, sin status al conectar.
func TestDeployLogStreamFinished(t *testing.T) {
	h := newStreamTest(t, testSSE())
	h.seed(t)
	id := h.queuedDeploy(t)
	log := h.logWriter(id)
	for _, text := range []string{"uno", "dos"} {
		if err := log.Line("%s", text); err != nil {
			t.Fatal(err)
		}
	}
	h.finish(t, id, entity.StatusFailed, map[string]string{})

	r := h.stream(t, logStreamPath(id), sessionOf("s"))
	r.expectLog(t, "uno")
	r.expectLog(t, "dos")
	r.expect(t, "status", "failed")
	r.expectClosed(t, 2*time.Second)

	// Desde el final: solo el status final.
	_, next := h.pollLog(t, id)
	r = h.stream(t, logStreamPath(id)+"?offset="+strconv.Itoa(next), sessionOf("s"))
	r.expect(t, "status", "failed")
	r.expectClosed(t, 2*time.Second)
	if n := h.m.hub.Pending(); n != 0 {
		t.Fatalf("el stream de un deploy terminado dejó %d canales en el Hub", n)
	}
}

// Desde dónde: Last-Event-ID gana sobre ?offset; si no, ?offset; si no, 0.
func TestDeployLogStreamResume(t *testing.T) {
	h := newStreamTest(t, testSSE())
	h.seed(t)
	id := h.queuedDeploy(t)
	log := h.logWriter(id)
	for _, text := range []string{"uno", "dos", "tres"} {
		if err := log.Line("%s", text); err != nil {
			t.Fatal(err)
		}
	}
	chunk, _ := h.pollLog(t, id)
	first := len(strings.SplitAfter(chunk, "\n")[0])

	r := h.stream(t, logStreamPath(id)+"?offset=0", sessionOf("s"), func(r *http.Request) {
		r.Header.Set("Last-Event-ID", strconv.Itoa(first))
	})
	r.expect(t, "status", "queued")
	r.expectLog(t, "dos")
	r.expectLog(t, "tres")

	r = h.stream(t, logStreamPath(id)+"?offset="+strconv.Itoa(first), sessionOf("s"))
	r.expect(t, "status", "queued")
	r.expectLog(t, "dos")

	r = h.stream(t, logStreamPath(id), sessionOf("s"))
	r.expect(t, "status", "queued")
	r.expectLog(t, "uno")
}

// Un queued que otro deploy de la app reemplaza (o que cierra RemoveApp) manda
// su status final y cierra enseguida, por el aviso del Hub: el heartbeat está
// en 1 min.
func TestDeployLogStreamSuperseded(t *testing.T) {
	h := newStreamTest(t, testSSE())
	h.seed(t)
	first := h.queuedDeploy(t)
	r := h.stream(t, logStreamPath(first), sessionOf("s"))
	r.expect(t, "status", "queued")
	second := h.queuedDeploy(t)
	r.expect(t, "status", "superseded")
	r.expectClosed(t, 2*time.Second)

	r = h.stream(t, logStreamPath(second), bearer("tp"))
	r.expect(t, "status", "queued")
	if err := h.m.RemoveApp(context.Background(), "auditfy", "staging", "api"); err != nil {
		t.Fatal(err)
	}
	r.expect(t, "status", "failed")
	r.expectClosed(t, 2*time.Second)
}

// Los 404 y 422 van en JSON, antes de abrir el stream, en el orden del
// polling: la app, el deploy de esa app y el offset.
func TestDeployLogStreamErrors(t *testing.T) {
	h := newStreamTest(t, testSSE())
	h.seed(t)
	id := h.queuedDeploy(t)
	other := expectQueued(t, h.c.Do("POST", infraPath+"/deploys", nil, sessionOf("s")), infraPath).ID
	if err := h.logWriter(id).Line("uno"); err != nil {
		t.Fatal(err)
	}
	lastEventID := func(v string) httptestutil.Opt {
		return func(r *http.Request) { r.Header.Set("Last-Event-ID", v) }
	}
	for _, c := range []struct {
		path    string
		opt     httptestutil.Opt
		status  int
		code    string
		message string
	}{
		{nadaPath + "/deploys/1/log/stream", nil, 404, "not_found", ""},
		{logStreamPath(other), nil, 404, "not_found", ""},
		{logStreamPath(other) + "?offset=x", nil, 404, "not_found", ""},
		{apiPath + "/deploys/abc/log/stream", nil, 404, "not_found", ""},
		{logStreamPath(id) + "?offset=x", nil, 422, "invalid", "offset: tiene que ser un entero ≥ 0"},
		{logStreamPath(id) + "?offset=", nil, 422, "invalid", "offset: tiene que ser un entero ≥ 0"},
		{logStreamPath(id) + "?offset=100000", nil, 422, "invalid", "offset: pasa el largo del log"},
		{logStreamPath(id), lastEventID("x"), 422, "invalid", "offset: tiene que ser un entero ≥ 0"},
		{logStreamPath(id) + "?offset=0", lastEventID("100000"), 422, "invalid", "offset: pasa el largo del log"},
		{logStreamPath(id), bearer("tx"), 403, "out_of_scope", ""},
	} {
		opts := []httptestutil.Opt{sessionOf("s")}
		if c.opt != nil {
			opts = append(opts, c.opt)
		}
		w := h.c.Do("GET", c.path, nil, opts...)
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s: Content-Type %q", c.path, ct)
		}
		expectError(t, w, c.status, c.code, c.message)
	}
}

// --- Los logs de una app (contrato §2.9) --------------------------------------------------

// secretEnv son variables con valores secretos para todas las apps.
type secretEnv []string

func (e secretEnv) ResolveEnv(ctx context.Context, appID int64) (map[string]string, []string, error) {
	return map[string]string{}, e, nil
}

const appService = "auditfy_staging_api_api"

// putAPIService deja en Swarm el servicio api de la app, con su tarea del slot 1.
func (h *streamTest) putAPIService() {
	one := uint64(1)
	h.f.PutStackService(naming.Stack("auditfy", "staging", "api"), docker.Service{
		Name: appService, Labels: naming.Labels("auditfy", "staging", "api", 1), Replicas: &one,
	}, docker.Task{ID: "t1aaaaaaaaaa", Slot: 1, State: "running", DesiredState: "running"})
}

func logsPath(query string) string { return apiPath + "/logs?" + query }

// Cada línea es un evento log con un JSON compacto (time truncada al
// milisegundo, task, stream y line enmascarada); sin follow, end eof y se
// cierra.
func TestAppLogsHTTP(t *testing.T) {
	const secret = "s3cr3t-de-la-app-1234"
	h := newStreamTestOn(t, newModuleTest(t, secretEnv{secret}), testSSE())
	h.seed(t)
	h.putAPIService()
	at := time.Date(2026, 10, 10, 14, 31, 40, 118_999_999, time.UTC)
	h.f.AppendServiceLogs(appService,
		docker.LogLine{Time: at, Stream: "stdout", TaskID: "t1aaaaaaaaaa", Text: "WARN consulta lenta · 812 ms"},
		docker.LogLine{Time: at.Add(time.Second), Stream: "stderr", TaskID: "t1aaaaaaaaaa", Text: "token=" + secret + " \"x\""},
	)
	r := h.stream(t, logsPath("service=api&tail=10"), bearer("tp"))
	r.expect(t, "log", `{"time":"2026-10-10T14:31:40.118Z","task":"api.1","stream":"stdout","line":"WARN consulta lenta · 812 ms"}`)
	r.expect(t, "log", `{"time":"2026-10-10T14:31:41.118Z","task":"api.1","stream":"stderr","line":"token=*** \"x\""}`)
	r.expect(t, "end", "eof")
	r.expectClosed(t, 2*time.Second)

	// Con follow sigue abierto y entrega lo nuevo; a la hora (MaxDuration),
	// end timeout.
	h.m.sse.MaxDuration = 500 * time.Millisecond
	h2 := newStreamTestOn(t, h.httpTest, h.m.sse)
	r = h2.stream(t, logsPath("service=api&tail=0&follow=1"), sessionOf("s"))
	h.f.AppendServiceLogs(appService, docker.LogLine{Time: at.Add(2 * time.Second), Stream: "stdout", TaskID: "t1aaaaaaaaaa", Text: "nueva"})
	ev := r.expect(t, "log", "")
	if !strings.Contains(ev.Data, `"line":"nueva"`) {
		t.Fatalf("línea nueva: %s", ev.Data)
	}
	r.expect(t, "end", "timeout")
	r.expectClosed(t, 2*time.Second)
}

// Los errores van en JSON antes de abrir, en el orden del contrato: la app
// (404), la query (422) y el servicio en Swarm (404, 503).
func TestAppLogsHTTPErrors(t *testing.T) {
	h := newStreamTest(t, testSSE())
	h.seed(t)
	for _, c := range []struct {
		path    string
		status  int
		code    string
		message string
	}{
		{nadaPath + "/logs?tail=abc", 404, "not_found", ""},
		{logsPath("tail=10"), 422, "invalid", "service: es obligatorio"},
		{logsPath("service=api&tail=abc"), 422, "invalid", "tail: "},
		{logsPath("service=api&since=ayer"), 422, "invalid", "since: "},
		{logsPath("service=api&follow=si"), 422, "invalid", "follow: "},
		{logsPath("service=api"), 404, "not_found", ""},
	} {
		expectError(t, h.c.Do("GET", c.path, nil, sessionOf("s")), c.status, c.code, c.message)
	}
	h.putAPIService()
	expectError(t, h.c.Do("GET", logsPath("service=api"), nil, bearer("tx")), 403, "out_of_scope", "")
	h.f.SetUnavailable(true)
	expectError(t, h.c.Do("GET", logsPath("service=api"), nil, sessionOf("s")), 503, "docker_unavailable", "")
}
