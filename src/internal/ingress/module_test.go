package ingress

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/caddy/caddytest"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare/cftest"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
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

// SyncRoutes (deploy, §7 paso 7): en desarrollo no hace nada; con el proxy
// habilitado manda la configuración, y si Caddy no la acepta devuelve el
// error que deploy registra como advertencia.
func TestModuleSyncRoutes(t *testing.T) {
	tm := newTestModule(t)
	ctx := context.Background()
	if err := tm.SyncRoutes(ctx); err != nil {
		t.Fatalf("sin ajustes (desarrollo): %v", err)
	}
	if n := len(tm.caddy.Loads()); n != 0 {
		t.Fatalf("en desarrollo no se manda nada: %d", n)
	}

	p := entity.DefaultProxySettings()
	p.PanelDomain = "panel.auditfy.cl"
	ingresstest.SetProxySettings(t, tm.db, tm.cipher, p)
	if err := tm.SyncRoutes(ctx); err != nil {
		t.Fatalf("habilitado: %v", err)
	}
	if n := len(tm.caddy.Loads()); n != 1 {
		t.Fatalf("loads: %d", n)
	}
	if got := tm.caddy.Forced(); got[0] {
		t.Fatal("SyncRoutes no fuerza la recarga")
	}

	tm.caddy.Reject("loading config: no")
	if err := tm.SyncRoutes(ctx); err == nil || !strings.Contains(err.Error(), "loading config: no") {
		t.Fatalf("rechazado: %v", err)
	}
}

// Lo que todavía es de la Fase 3: lo que llama admin no rompe la Fase 1.
func TestModuleStubs(t *testing.T) {
	m := newTestModule(t)
	ctx := context.Background()
	zones, err := m.CheckCloudflareToken(ctx, "cf-token")
	if err != nil || zones != nil {
		t.Fatalf("CheckCloudflareToken: %v %v (sin verificar: zonas desconocidas)", zones, err)
	}
	m.ProxySettingsChanged(ctx, entity.DefaultProxySettings(), entity.DefaultProxySettings())
}

// El job "proxy" cada 5 min (cron.Run lo corre también al arrancar) y
// "certificados" cada hora.
func TestModuleJobs(t *testing.T) {
	m := newTestModule(t)
	jobs := m.Jobs()
	if len(jobs) != 2 || jobs[0].Name != "proxy" || jobs[0].Every != 5*time.Minute ||
		jobs[1].Name != "certificados" || jobs[1].Every != time.Hour {
		t.Fatalf("Jobs: %+v", jobs)
	}
	// Sin el proxy synced no se sabe nada: el job no sondea ni falla.
	if err := jobs[1].Run(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// enableProxy deja el proxy habilitado: panel_domain, con la imagen de
// Caddy de la versión (v0.1.0) y los puertos 8080 y 8443.
func (tm *testModule) enableProxy(t *testing.T) {
	t.Helper()
	p := entity.DefaultProxySettings()
	p.PanelDomain = "panel.auditfy.cl"
	p.HTTPPort, p.HTTPSPort = 8080, 8443
	ingresstest.SetProxySettings(t, tm.db, tm.cipher, p)
}

// runModule corre m.Run hasta el final del test, y al final cancela y espera
// a que vuelva.
func runModule(t *testing.T, m *Module) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); m.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("Run no volvió al cancelar el ctx")
		}
	})
}

func eventually(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatalf("%s: no pasó en 2 s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// Run lleva los reintentos de /load (el onFail del Syncer despierta al
// Retrier) y la recreación agendada, y vuelve al cancelar.
func TestModuleRun(t *testing.T) {
	tm := newTestModule(t, func(o *Options) {
		o.Timing = Timing{RetryMin: time.Millisecond, RetryMax: 4 * time.Millisecond, RecreateDelay: time.Millisecond}
	})
	tm.enableProxy(t)
	runModule(t, tm.Module)
	ctx := context.Background()

	tm.caddy.Reject("todavía no")
	if err := tm.SyncRoutes(ctx); err == nil {
		t.Fatal("debió fallar")
	}
	tm.caddy.Accept()
	eventually(t, "el Retrier reintenta hasta synced", tm.state.Synced)

	tm.scheduleRecreate()
	eventually(t, "el Recreator crea muelle-caddy", func() bool {
		c, err := tm.docker.InspectContainer(ctx, naming.CaddyContainer)
		return err == nil && c.Running && !tm.state.Recreating()
	})
}

// Después del job, GET /api/v1/proxy dice synced con la imagen que corre.
func TestModuleProxyStatusAfterJob(t *testing.T) {
	tm := newTestModule(t)
	tm.enableProxy(t)
	if err := tm.Jobs()[0].Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	c := httptestutil.New(t, httptestutil.FakeAuth{Sessions: map[string]entity.Principal{"s": {UserID: 1, Username: "felipe"}}},
		tm.RegisterRoutes)
	w := c.Do("GET", "/api/v1/proxy", nil, httptestutil.WithSession("s"))
	httptestutil.Expect(t, w, http.StatusOK)
	got := httptestutil.Decode[map[string]any](t, w)
	caddy, _ := got["caddy"].(map[string]any)
	if got["status"] != "synced" || caddy["image"] != naming.CaddyImageFor("v0.1.0") || caddy["running"] != true || caddy["http_port"] != 8080.0 {
		t.Fatalf("GET /proxy: %v", got)
	}
}

// Con un nombre propio de muelle-caddy (los tests de integración), el job
// crea ese contenedor y nunca nombra muelle-caddy.
func TestModuleRecreatesWithCustomName(t *testing.T) {
	tm := newTestModule(t, func(o *Options) { o.CaddyContainer = "it3-x" })
	tm.enableProxy(t)
	ingresstest.Stage(t, tm.db, "auditfy", "staging")
	ctx := context.Background()
	if err := tm.Jobs()[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if c, err := tm.docker.InspectContainer(ctx, "it3-x"); err != nil || !c.Running || !slices.Contains(c.Networks, "auditfy_staging") {
		t.Fatalf("it3-x: %+v %v", c, err)
	}
	for _, c := range tm.docker.Calls() {
		if slices.Contains(strings.Fields(c), naming.CaddyContainer) {
			t.Errorf("la llamada %q nombra %s", c, naming.CaddyContainer)
		}
	}
}

// GET /api/v1/proxy y los dominios existen (los dominios ya no son 404
// no_route). El error de una red ajena se responde 409.
func TestModuleRoutes(t *testing.T) {
	tm := newTestModule(t)
	m, f := tm.Module, tm.docker
	var kit *utils.Kit
	c := httptestutil.New(t, httptestutil.FakeAuth{Sessions: map[string]entity.Principal{"s": {UserID: 1, Username: "felipe"}}},
		func(k *utils.Kit) {
			kit = k
			m.RegisterRoutes(k)
		})
	w := c.Do("GET", "/api/v1/projects/auditfy/stages/staging/apps/api/domains", nil, httptestutil.WithSession("s"))
	if w.Code != http.StatusNotFound || !bytes.Contains(w.Body.Bytes(), []byte(`"not_found"`)) {
		t.Errorf("GET …/domains de una app que no existe: %d %s", w.Code, w.Body)
	}
	ingresstest.App(t, tm.db, "auditfy", "staging", "api", "service")
	w = c.Do("GET", "/api/v1/projects/auditfy/stages/staging/apps/api/domains", nil, httptestutil.WithSession("s"))
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Errorf("GET …/domains: %d %s", w.Code, w.Body)
	}
	w = c.Do("GET", "/api/v1/proxy", nil, httptestutil.WithSession("s"))
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"disabled"`)) {
		t.Errorf("GET /api/v1/proxy: %d %s", w.Code, w.Body)
	}

	ctx := context.Background()
	if _, err := f.CreateNetwork(ctx, docker.NetworkSpec{Name: "auditfy_staging", Driver: "bridge"}); err != nil {
		t.Fatal(err)
	}
	err := m.EnsureStageNetwork(ctx, "auditfy", "staging")
	kit.Handle("POST /api/v1/x", utils.AccessSession, func(w http.ResponseWriter, r *http.Request) { kit.Fail(w, r, err) })
	w = c.Do("POST", "/api/v1/x", nil, httptestutil.WithSession("s"))
	if w.Code != http.StatusConflict || !bytes.Contains(w.Body.Bytes(), []byte(`"network_conflict"`)) {
		t.Fatalf("red ajena: %d %s", w.Code, w.Body)
	}
	if !slices.Contains(f.Calls(), "InspectNetwork auditfy_staging") {
		t.Fatal("debió inspeccionar la red")
	}
}

// New exige Audit: los dominios auditan cada cambio (contrato §3.2).
func TestModuleRequiresAudit(t *testing.T) {
	db := sqlitetest.New(t)
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), "Audit") {
			t.Fatalf("debía hacer panic por Audit: %v", r)
		}
	}()
	New(Deps{DB: db, Cipher: c, Docker: dockertest.New()})
}

// Los dominios por HTTP con el módulo armado: Cloudflare (cftest) y Caddy
// (caddytest) falsos, y Docker caído, que los dominios no usan.
func TestModuleDomainsFlow(t *testing.T) {
	tm := newTestModule(t)
	tm.cf.AddToken("cf-token-1234567890abcdef")
	tm.cf.AddZone("z1", "tudominio.cl")
	p := entity.DefaultProxySettings()
	p.PanelDomain, p.CloudflareToken, p.PublicIP = "panel.tudominio.cl", "cf-token-1234567890abcdef", "203.0.113.24"
	ingresstest.SetProxySettings(t, tm.db, tm.cipher, p)
	ingresstest.App(t, tm.db, "auditfy", "staging", "api", "service")
	tm.docker.SetUnavailable(true)
	c := httptestutil.New(t, httptestutil.FakeAuth{Sessions: map[string]entity.Principal{"s": {UserID: 1, Username: "felipe"}}},
		tm.RegisterRoutes)
	const app = "/api/v1/projects/auditfy/stages/staging/apps/api/domains"

	w := c.Do("POST", app, map[string]any{"hostname": "api.tudominio.cl", "target_service": "api", "target_port": 3000}, httptestutil.WithSession("s"))
	httptestutil.Expect(t, w, http.StatusCreated)
	if got := httptestutil.Decode[map[string]any](t, w); got["tls_mode"] != "cloudflare" || got["cf_record_id"] == nil {
		t.Fatalf("POST: %v", got)
	}
	recs := tm.cf.Records("z1")
	if len(recs) != 1 || recs[0].Name != "api.tudominio.cl" || recs[0].Proxied {
		t.Fatalf("registros: %+v", recs)
	}
	if !bytes.Contains(tm.caddy.Loads()[len(tm.caddy.Loads())-1], []byte(`"api.tudominio.cl"`)) {
		t.Fatal("el Load no trae el dominio")
	}

	w = c.Do("GET", app, nil, httptestutil.WithSession("s"))
	httptestutil.Expect(t, w, http.StatusOK)
	if got := httptestutil.Decode[[]map[string]any](t, w); len(got) != 1 {
		t.Fatalf("GET: %v", got)
	}

	w = c.Do("PATCH", app+"/api.tudominio.cl", map[string]any{"proxied": true}, httptestutil.WithSession("s"))
	httptestutil.Expect(t, w, http.StatusOK)
	if recs := tm.cf.Records("z1"); !recs[0].Proxied {
		t.Fatalf("PATCH: %+v", recs)
	}

	w = c.Do("DELETE", app+"/api.tudominio.cl", nil, httptestutil.WithSession("s"))
	httptestutil.Expect(t, w, http.StatusNoContent)
	if recs := tm.cf.Records("z1"); len(recs) != 0 {
		t.Fatalf("DELETE: %+v", recs)
	}

	ctx := context.Background()
	var n int
	if err := tm.db.Conn(ctx).QueryRowContext(ctx, `SELECT count(*) FROM audit_events WHERE action LIKE 'domain.%'`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("auditoría: %d %v", n, err)
	}
}
