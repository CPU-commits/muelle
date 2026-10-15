package ingress

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/caddy/caddytest"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare/cftest"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

// testModule es el módulo armado para un test, con sus fakes: Docker en
// memoria, Cloudflare y la admin API de Caddy falsos. Ningún test del módulo
// llega a Docker, a Caddy ni a la API de Cloudflare reales.
type testModule struct {
	*Module
	docker *dockertest.Fake
	cf     *cftest.Server
	caddy  *caddytest.Admin
	db     *sqlite.DB
	cipher *crypto.Cipher
}

// newTestModule arma el módulo con Version v0.1.0 (la imagen de Caddy por
// defecto existe: con panel_domain, el proxy está habilitado). Siempre pone
// CloudflareURL en un cftest nuevo y CaddyAdminDial en un caddytest nuevo,
// después de opts, así ningún test los puede dejar en los reales.
func newTestModule(t *testing.T, opts ...func(*Options)) *testModule {
	t.Helper()
	db := sqlitetest.New(t)
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	tm := &testModule{docker: dockertest.New(), cf: cftest.New(t), caddy: caddytest.New(t), db: db, cipher: c}
	var o Options
	for _, opt := range opts {
		opt(&o)
	}
	o.CloudflareURL = tm.cf.URL()
	o.CaddyAdminDial = tm.caddy.Dial
	tm.Module = New(Deps{
		DB: db, Cipher: c, Audit: sqlite.NewAuditRecorder(db, clock.System{}, log), Log: log, Docker: tm.docker,
		Version: "v0.1.0", Options: o,
	})
	return tm
}

// Lo que catalog y deploy llaman ya funciona: la red de cada stage.
func TestModuleStageNetwork(t *testing.T) {
	tm := newTestModule(t)
	m, f := tm.Module, tm.docker
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

// T2: con un nombre propio de muelle-caddy (los tests de integración), las
// redes de stage conectan ese contenedor y nunca nombran muelle-caddy.
func TestModuleStageNetworkCustomName(t *testing.T) {
	tm := newTestModule(t, func(o *Options) { o.CaddyContainer = "it3-x" })
	ctx := context.Background()
	f := tm.docker
	if _, err := f.CreateNetwork(ctx, docker.NetworkSpec{Name: "it3x_panel", Driver: "overlay", Attachable: true}); err != nil {
		t.Fatal(err)
	}
	f.SetImagePresent("caddy:test")
	if _, err := f.CreateContainer(ctx, docker.ContainerSpec{Name: "it3-x", Image: "caddy:test", Network: "it3x_panel"}); err != nil {
		t.Fatal(err)
	}
	if err := tm.EnsureStageNetwork(ctx, "auditfy", "staging"); err != nil {
		t.Fatal(err)
	}
	if c, err := f.InspectContainer(ctx, "it3-x"); err != nil || !slices.Contains(c.Networks, "auditfy_staging") {
		t.Fatalf("it3-x: %+v %v", c, err)
	}
	if err := tm.RemoveStageNetwork(ctx, "auditfy", "staging"); err != nil {
		t.Fatal(err)
	}
	calls := f.Calls()
	if !slices.Contains(calls, "ConnectNetwork auditfy_staging it3-x") || !slices.Contains(calls, "DisconnectNetwork auditfy_staging it3-x") {
		t.Fatalf("Calls: %v", calls)
	}
	for _, c := range calls {
		if slices.Contains(strings.Fields(c), naming.CaddyContainer) {
			t.Errorf("la llamada %q nombra %s", c, naming.CaddyContainer)
		}
	}
}

// Lo que todavía es de la Fase 3: SyncRoutes avisa con ErrNotImplemented (el
// deploy lo trata como una advertencia); lo que llama admin no rompe la Fase 1.
func TestModuleStubs(t *testing.T) {
	m := newTestModule(t)
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
	tm := newTestModule(t)
	m, f := tm.Module, tm.docker
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
