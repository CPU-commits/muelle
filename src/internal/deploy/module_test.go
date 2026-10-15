package deploy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"testing"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

// fakeEnv y fakeIngress cumplen los puertos que conecta el contenedor
// (secrets.Module e ingress.Module).
type fakeEnv struct{}

func (fakeEnv) ResolveEnv(ctx context.Context, appID int64) (map[string]string, []string, error) {
	return map[string]string{}, nil, nil
}

type fakeIngress struct{}

func (fakeIngress) EnsureStageNetwork(ctx context.Context, project, stage string) error { return nil }
func (fakeIngress) SyncRoutes(ctx context.Context) error                                { return nil }

func newTestModule(t *testing.T) (*Module, *dockertest.Fake) {
	t.Helper()
	db := sqlitetest.New(t)
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := dockertest.New()
	m := New(Deps{DB: db, Cipher: c, Clock: clock.System{}, Audit: sqlite.NewAuditRecorder(db, clock.System{}, log), Log: log,
		Docker: f, Env: fakeEnv{}, Ingress: fakeIngress{}})
	return m, f
}

// Borrar una app (catalog) ejecuta docker stack rm de su stack: sus
// servicios se van y sus volúmenes quedan (§5.2; los datos no se borran).
func TestRemoveApp(t *testing.T) {
	m, f := newTestModule(t)
	ctx := context.Background()
	stack := naming.Stack("auditfy", "staging", "api")
	f.PutStackService(stack, docker.Service{Name: naming.Service("auditfy", "staging", "api", "api"), Labels: naming.AppLabels("auditfy", "staging", "api")})
	f.PutStackService(naming.Stack("auditfy", "staging", "web"), docker.Service{Name: naming.Service("auditfy", "staging", "web", "web")})
	f.PutVolume(docker.Volume{Name: naming.Volume(stack, "data"), Labels: map[string]string{naming.LabelStackNamespace: stack}})

	if err := m.RemoveApp(ctx, "auditfy", "staging", "api"); err != nil {
		t.Fatal(err)
	}
	if got := f.RemovedStacks(); !slices.Equal(got, []string{stack}) {
		t.Fatalf("stack rm de %v", got)
	}
	list, _ := f.ListServices(ctx, nil)
	if len(list) != 1 || list[0].Name != "auditfy_staging_web_web" {
		t.Fatalf("quedaron %+v", list)
	}
	if v, _ := f.ListVolumes(ctx, nil); len(v) != 1 {
		t.Fatalf("los volúmenes quedan: %+v", v)
	}
	// Una app que nunca se desplegó (sin stack) se borra igual.
	if err := m.RemoveApp(ctx, "auditfy", "staging", "nunca"); err != nil {
		t.Fatal(err)
	}
}

// Con Docker caído (o sin Swarm) no se borra: 503, y la app sigue en catalog.
func TestRemoveAppDockerDown(t *testing.T) {
	m, f := newTestModule(t)
	f.SetUnavailable(true)
	if err := m.RemoveApp(context.Background(), "auditfy", "staging", "api"); !errors.Is(err, entity.ErrDockerUnavailable) {
		t.Fatalf("Docker caído: %v", err)
	}
	if len(f.RemovedStacks()) != 0 {
		t.Fatal("no debió ejecutar docker stack rm")
	}
}

// Las rutas que todavía no están no existen (404 no_route): el panel muestra
// "Llega con la Fase 2".
func TestModuleRoutesAndStubs(t *testing.T) {
	m, _ := newTestModule(t)
	c := httptestutil.New(t, httptestutil.FakeAuth{Sessions: map[string]entity.Principal{"s": {UserID: 1, Username: "felipe"}}}, m.RegisterRoutes)
	app := "/api/v1/projects/auditfy/stages/staging/apps/api"
	for _, path := range []string{app + "/deploys/1/log/stream", app + "/status", app + "/logs"} {
		w := c.Do("GET", path, nil, httptestutil.WithSession("s"))
		if w.Code != http.StatusNotFound || !bytes.Contains(w.Body.Bytes(), []byte(`"no_route"`)) {
			t.Errorf("GET %s: %d %s", path, w.Code, w.Body)
		}
	}
	if jobs := m.Jobs(); len(jobs) != 0 {
		t.Fatalf("Jobs: %v", jobs)
	}
	m.Run(context.Background()) // el esqueleto vuelve enseguida
}
