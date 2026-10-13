package ingress

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
	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

func newTestModule(t *testing.T) (*Module, *dockertest.Fake) {
	t.Helper()
	db := sqlitetest.New(t)
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := dockertest.New()
	m := New(Deps{DB: db, Cipher: c, Audit: sqlite.NewAuditRecorder(db, clock.System{}, log), Log: log, Docker: f, Version: "v0.1.0"})
	return m, f
}

// Lo que catalog y deploy llaman ya funciona: la red de cada stage.
func TestModuleStageNetwork(t *testing.T) {
	m, f := newTestModule(t)
	ctx := context.Background()
	if err := m.EnsureStageNetwork(ctx, "auditfy", "staging"); err != nil {
		t.Fatal(err)
	}
	if n, err := f.InspectNetwork(ctx, "auditfy_staging"); err != nil || n.Driver != "overlay" || !n.Attachable {
		t.Fatalf("red: %+v %v", n, err)
	}
	if err := m.RemoveStageNetwork(ctx, "auditfy", "staging"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.InspectNetwork(ctx, "auditfy_staging"); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("red borrada: %v", err)
	}
}

// Lo que todavía es de la Fase 3: SyncRoutes avisa con ErrNotImplemented (el
// deploy lo trata como una advertencia); lo que llama admin no rompe la Fase 1.
func TestModuleStubs(t *testing.T) {
	m, _ := newTestModule(t)
	ctx := context.Background()
	if err := m.SyncRoutes(ctx); !errors.Is(err, entity.ErrNotImplemented) {
		t.Fatalf("SyncRoutes: %v", err)
	}
	zones, err := m.CheckCloudflareToken(ctx, "cf-token")
	if err != nil || zones != nil {
		t.Fatalf("CheckCloudflareToken: %v %v (sin verificar: zonas desconocidas)", zones, err)
	}
	m.ProxySettingsChanged(ctx, entity.DefaultProxySettings(), entity.DefaultProxySettings())
	if jobs := m.Jobs(); len(jobs) != 0 {
		t.Fatalf("Jobs: %v", jobs)
	}
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	<-done // el esqueleto no tiene nada que correr
}

// Mientras la Fase 3 no está, sus rutas no existen (404 no_route): así el
// panel muestra "Llega con la Fase 3". El error de una red ajena ya se responde 409.
func TestModuleRoutes(t *testing.T) {
	m, f := newTestModule(t)
	var kit *utils.Kit
	c := httptestutil.New(t, httptestutil.FakeAuth{Sessions: map[string]entity.Principal{"s": {UserID: 1, Username: "felipe"}}},
		func(k *utils.Kit) {
			kit = k
			m.RegisterRoutes(k)
		})
	for _, path := range []string{"/api/v1/projects/auditfy/stages/staging/apps/api/domains", "/api/v1/proxy"} {
		w := c.Do("GET", path, nil, httptestutil.WithSession("s"))
		if w.Code != http.StatusNotFound || !bytes.Contains(w.Body.Bytes(), []byte(`"no_route"`)) {
			t.Errorf("GET %s: %d %s", path, w.Code, w.Body)
		}
	}

	ctx := context.Background()
	if _, err := f.CreateNetwork(ctx, docker.NetworkSpec{Name: "auditfy_staging", Driver: "bridge"}); err != nil {
		t.Fatal(err)
	}
	err := m.EnsureStageNetwork(ctx, "auditfy", "staging")
	kit.Handle("POST /api/v1/x", utils.AccessSession, func(w http.ResponseWriter, r *http.Request) { kit.Fail(w, r, err) })
	w := c.Do("POST", "/api/v1/x", nil, httptestutil.WithSession("s"))
	if w.Code != http.StatusConflict || !bytes.Contains(w.Body.Bytes(), []byte(`"network_conflict"`)) {
		t.Fatalf("red ajena: %d %s", w.Code, w.Body)
	}
	if !slices.Contains(f.Calls(), "InspectNetwork auditfy_staging") {
		t.Fatal("debió inspeccionar la red")
	}
}
