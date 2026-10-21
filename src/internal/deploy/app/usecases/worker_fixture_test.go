package usecases

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	outservice "github.com/CPU-commits/muelle/src/internal/deploy/infra/out/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
)

// Los digests de los tests: 64 hex.
var (
	digestA = "sha256:" + strings.Repeat("a", 64)
	digestB = "sha256:" + strings.Repeat("b", 64)
	digestC = "sha256:" + strings.Repeat("c", 64)
)

// testTiming son los plazos cortos de los tests del worker.
var testTiming = Timing{
	Poll: 10 * time.Millisecond, Settle: 150 * time.Millisecond,
	ResolveTimeout: 2 * time.Second, NetworkTimeout: 2 * time.Second, StackTimeout: 5 * time.Second,
	RoutesTimeout: time.Second, RetryEvery: 50 * time.Millisecond,
}

// fakeEnv son las variables de cada app: vars y los valores secretos.
type fakeEnv struct {
	mu      sync.Mutex
	vars    map[int64]map[string]string
	secrets map[int64][]string
	err     error
}

func newFakeEnv() *fakeEnv {
	return &fakeEnv{vars: map[int64]map[string]string{}, secrets: map[int64][]string{}}
}

func (e *fakeEnv) set(appID int64, vars map[string]string, secrets ...string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.vars[appID] = vars
	e.secrets[appID] = secrets
}

func (e *fakeEnv) ResolveEnv(ctx context.Context, appID int64) (map[string]string, []string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.err != nil {
		return nil, nil, e.err
	}
	return maps.Clone(e.vars[appID]), append([]string(nil), e.secrets[appID]...), nil
}

// fakeIngress cuenta las llamadas; SyncRoutes devuelve syncErr o, si es nil,
// ErrNotImplemented, así los tests del worker pasan por la advertencia del
// paso 7 (un error de SyncRoutes no hace fallar el deploy). Con syncOK
// devuelve nil.
type fakeIngress struct {
	mu        sync.Mutex
	ensureErr error
	syncErr   error
	syncOK    bool
	onEnsure  func()
	ensured   []string
	synced    int
}

func (i *fakeIngress) EnsureStageNetwork(ctx context.Context, project, stage string) error {
	i.mu.Lock()
	i.ensured = append(i.ensured, project+"/"+stage)
	err, hook := i.ensureErr, i.onEnsure
	i.mu.Unlock()
	if hook != nil {
		hook()
	}
	return err
}

func (i *fakeIngress) SyncRoutes(ctx context.Context) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.synced++
	switch {
	case i.syncErr != nil:
		return i.syncErr
	case i.syncOK:
		return nil
	}
	return shared.ErrNotImplemented
}

// syncBuffer es un bytes.Buffer para el slog de los tests, que escriben
// varias goroutines.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}
func (b *syncBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.b.String() }

// world es el worker completo sobre la base del fixture: dockertest con el
// Swarm simulado, los adaptadores reales de Docker y de settings, un env y
// un ingress falsos, el reloj del sistema y los plazos cortos. Deps usa la
// cola falsa del fixture (queue) hasta que newDispatcher la cambia.
type world struct {
	*fixture
	docker  *dockertest.Fake
	sim     *swarmSim
	env     *fakeEnv
	ingress *fakeIngress
	run     *RunDeploy
	deps    Deps
	logs    *syncBuffer
	slog    *slog.Logger
}

func newWorld(t *testing.T) *world {
	t.Helper()
	f := newFixture(t)
	w := &world{fixture: f, docker: dockertest.New(), env: newFakeEnv(), ingress: &fakeIngress{}, logs: &syncBuffer{}}
	w.slog = slog.New(slog.NewJSONHandler(w.logs, nil))
	w.sim = simulate(w.docker, simOpts{})
	w.deps = Deps{Tx: f.db, Apps: f.apps, Deploys: f.deploys, Queue: f.queue, Hub: f.hub, Clock: clock.System{}, Audit: f.audit}
	dk := outservice.NewDocker(w.docker)
	w.run = &RunDeploy{
		Apps: f.apps, Deploys: f.deploys, Registries: f.regs, Env: w.env,
		Domains: outservice.NewDomainTargets(f.db), Ingress: w.ingress,
		Images: dk, Stacks: outservice.NewDockerStacks(w.docker), Cluster: dk,
		Manager: outservice.NewDeploySettings(f.db), Gate: f.queue,
		Log: NewLogWriter(f.deploys, f.hub, clock.System{}), Hub: f.hub, Tx: f.db,
		Clock: clock.System{}, Timing: testTiming,
	}
	// Las imágenes de las apps del fixture existen en el registry.
	w.docker.SetDigest("ghcr.io/x/api:1", digestA)
	w.docker.SetDigest("ghcr.io/x/web:1", digestA)
	w.docker.SetDigest("ghcr.io/x/worker:1", digestB)
	w.docker.SetDigest("nginx", digestC)
	w.env.set(f.infraID, map[string]string{"TAG": "1"})
	// El deploy_timeout_s de las apps: corto, salvo que el test lo cambie.
	exec(t, f.db, `UPDATE apps SET deploy_timeout_s = 5`)
	return w
}

// deploy encola un deploy de la app con ese cuerpo, como un token.
func (w *world) deploy(t *testing.T, ref dto.AppRef, image *string, images map[string]string) int64 {
	t.Helper()
	q, err := NewCreateDeploy(w.deps).Execute(tokenCtx(), dto.CreateDeploy{AppRef: ref, Image: image, Images: images})
	if err != nil {
		t.Fatal(err)
	}
	return q.ID
}

// claimAndRun reclama el queued más viejo de la app y lo corre con Execute,
// sin el despachador. Devuelve el deploy como quedó y el error de Execute.
func (w *world) claimAndRun(t *testing.T, appID int64) (entity.Deploy, error) {
	t.Helper()
	ctx := context.Background()
	q, err := w.deploys.NextQueued(ctx, appID)
	if err != nil || q == nil {
		t.Fatalf("NextQueued: %v %v", q, err)
	}
	at := clock.System{}.Now()
	if ok, err := w.deploys.Claim(ctx, q.ID, at); err != nil || !ok {
		t.Fatalf("Claim: %v %v", ok, err)
	}
	q.Status, q.StartedAt = entity.StatusRunning, &at
	runErr := w.run.Execute(ctx, *q)
	return w.get(t, q.ID), runErr
}

// runOK es claimAndRun sin error de Execute.
func (w *world) runOK(t *testing.T, appID int64) entity.Deploy {
	t.Helper()
	d, err := w.claimAndRun(t, appID)
	if err != nil {
		t.Fatalf("Execute: %v (log:\n%s)", err, w.log(t, d.ID))
	}
	return d
}

func (w *world) get(t *testing.T, id int64) entity.Deploy {
	t.Helper()
	d, err := w.deploys.GetByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// log es el log entero del deploy.
func (w *world) log(t *testing.T, id int64) string {
	t.Helper()
	var s string
	ctx := context.Background()
	if err := w.db.Conn(ctx).QueryRowContext(ctx, `SELECT log FROM deploys WHERE id = ?`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

var lineTime = regexp.MustCompile(`(?m)^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z `)

// texts son las líneas del log sin la hora.
func (w *world) texts(t *testing.T, id int64) []string {
	t.Helper()
	l := w.log(t, id)
	if l == "" {
		return nil
	}
	lines := strings.Split(strings.TrimSuffix(l, "\n"), "\n")
	for i, line := range lines {
		if !lineTime.MatchString(line) {
			t.Fatalf("línea sin hora: %q", line)
		}
		lines[i] = lineTime.ReplaceAllString(line, "")
	}
	return lines
}

// expectFailed verifica que el deploy cerró failed con un error que contiene want.
func expectFailed(t *testing.T, w *world, d entity.Deploy, want string) {
	t.Helper()
	if d.Status != entity.StatusFailed || d.Error == nil || !strings.Contains(*d.Error, want) || d.FinishedAt == nil {
		got := "<nil>"
		if d.Error != nil {
			got = *d.Error
		}
		t.Fatalf("esperaba failed con %q: %s, error %q\nlog:\n%s", want, d.Status, got, w.log(t, d.ID))
	}
	if !strings.Contains(w.log(t, d.ID), "failed: "+want) && !strings.Contains(w.log(t, d.ID), "failed: ") {
		t.Fatalf("el log no cierra con failed:\n%s", w.log(t, d.ID))
	}
}

// settingValue es settings.<key>, o "" si no está.
func settingValue(t *testing.T, w *world, key string) string {
	t.Helper()
	var v []byte
	ctx := context.Background()
	err := w.db.Conn(ctx).QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(v)
}

// containsInOrder dice si lines trae cada want, en orden (como prefijo de una línea).
func containsInOrder(lines []string, want ...string) error {
	i := 0
	for _, l := range lines {
		if i < len(want) && strings.HasPrefix(l, want[i]) {
			i++
		}
	}
	if i < len(want) {
		return fmt.Errorf("falta %q (en orden) en:\n%s", want[i], strings.Join(lines, "\n"))
	}
	return nil
}

// waitFor espera hasta 5 s a que cond sea cierta.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("no pasó: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

var _ service.EnvResolver = (*fakeEnv)(nil)
var _ service.Ingress = (*fakeIngress)(nil)
