package usecases

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	reposqlite "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/repository/sqlite"
	outservice "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/caddy"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/caddy/caddytest"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

const syncToken = "cf-token-1234567890abcdef"

// lockedBuffer es un bytes.Buffer que se puede leer mientras slog escribe.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type syncFixture struct {
	db     *sqlite.DB
	cipher *crypto.Cipher
	caddy  *caddytest.Admin
	state  *ProxyState
	clock  *clock.Manual
	repo   *reposqlite.Domains
	syncer *Syncer
	fails  atomic.Int32
	log    *lockedBuffer
	apiID  int64
}

func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()
	db := sqlitetest.New(t)
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	f := &syncFixture{
		db: db, cipher: c, caddy: caddytest.New(t), state: NewProxyState(),
		clock: clock.NewManual(time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)),
		repo:  reposqlite.NewDomains(db), log: &lockedBuffer{},
	}
	log := slog.New(slog.NewTextHandler(f.log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	f.syncer = NewSyncer(outservice.NewProxySettingsReader(db, c), f.repo, outservice.NewCatalogReader(db),
		OriginRepos{Certs: reposqlite.NewOriginCerts(db, c), Hosts: reposqlite.NewOriginHosts(db)}, caddy.NewAdmin(f.caddy.Dial), f.state, f.clock, "", false, func() { f.fails.Add(1) }, log)
	f.apiID = ingresstest.App(t, db, "auditfy", "staging", "api", "service")
	return f
}

// enable deja los ajustes con panel_domain y caddy_image (Enabled), y los
// cambios que pida mod.
func (f *syncFixture) enable(t *testing.T, mod func(*shared.ProxySettings)) {
	t.Helper()
	p := shared.DefaultProxySettings()
	p.PanelDomain = "panel.auditfy.cl"
	p.CaddyImage = "caddy:2-alpine"
	if mod != nil {
		mod(&p)
	}
	ingresstest.SetProxySettings(t, f.db, f.cipher, p)
}

func (f *syncFixture) addDomain(t *testing.T, host string) {
	t.Helper()
	d, err := entity.NewDomain(f.apiID, host, "api", 3000, false, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.Create(context.Background(), &d); err != nil {
		t.Fatal(err)
	}
}

func (f *syncFixture) lastLoad(t *testing.T) string {
	t.Helper()
	loads := f.caddy.Loads()
	if len(loads) == 0 {
		t.Fatal("no llegó ningún POST /load")
	}
	return string(loads[len(loads)-1])
}

func TestSyncLoadsRoutes(t *testing.T) {
	f := newSyncFixture(t)
	f.enable(t, nil)
	f.addDomain(t, "api.auditfy.cl")
	f.addDomain(t, "www.auditfy.cl")
	if err := f.syncer.Sync(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	body := f.lastLoad(t)
	for _, want := range []string{`"api.auditfy.cl"`, `"www.auditfy.cl"`, `"panel.auditfy.cl"`, `"auditfy_staging_api_api:3000"`, `"muelle_muelle:8080"`} {
		if !strings.Contains(body, want) {
			t.Errorf("falta %s en %s", want, body)
		}
	}
	status, msg, at := f.state.Snapshot(true)
	if status != ProxySynced || msg != "" || at == nil || !at.Equal(f.clock.Now()) {
		t.Fatalf("estado: %s %q %v", status, msg, at)
	}
	if f.fails.Load() != 0 {
		t.Fatal("onFail sin fallar")
	}
}

// Sin panel_domain (desarrollo) no se manda nada (contrato §3.4).
func TestSyncDisabled(t *testing.T) {
	f := newSyncFixture(t)
	f.addDomain(t, "api.auditfy.cl")
	if err := f.syncer.Sync(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if len(f.caddy.Loads()) != 0 {
		t.Fatal("disabled no manda nada")
	}
	if status, msg, _ := f.state.Snapshot(true); status != ProxyOutOfSync || msg != NotSyncedYet {
		t.Fatalf("el estado no cambia: %s %q", status, msg)
	}
}

func TestSyncRejected(t *testing.T) {
	f := newSyncFixture(t)
	f.enable(t, func(p *shared.ProxySettings) { p.CloudflareToken = syncToken })
	f.caddy.Reject(`loading config: api_token "` + syncToken + `" no sirve`)
	err := f.syncer.Sync(context.Background(), false)
	var rej *service.CaddyRejected
	if !errors.As(err, &rej) || strings.Contains(err.Error(), syncToken) {
		t.Fatalf("Sync: %v", err)
	}
	status, msg, _ := f.state.Snapshot(true)
	if status != ProxyOutOfSync || msg != `loading config: api_token "***" no sirve` {
		t.Fatalf("estado: %s %q", status, msg)
	}
	if n := f.fails.Load(); n != 1 {
		t.Fatalf("onFail: %d veces", n)
	}
	if log := f.log.String(); strings.Contains(log, syncToken) || strings.Contains(log, "routes") || !strings.Contains(log, "***") {
		t.Fatalf("log: %s", log)
	}
}

func TestSyncRecovers(t *testing.T) {
	f := newSyncFixture(t)
	f.enable(t, nil)
	f.caddy.Reject("no")
	if err := f.syncer.Sync(context.Background(), false); err == nil {
		t.Fatal("debió fallar")
	}
	f.caddy.Accept()
	if err := f.syncer.Sync(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if status, msg, _ := f.state.Snapshot(true); status != ProxySynced || msg != "" {
		t.Fatalf("estado: %s %q", status, msg)
	}
}

func TestSyncUnreachable(t *testing.T) {
	f := newSyncFixture(t)
	f.enable(t, nil)
	f.caddy.SetDown(true)
	if err := f.syncer.Sync(context.Background(), false); !errors.Is(err, service.ErrCaddyUnreachable) {
		t.Fatalf("Sync: %v", err)
	}
	if _, msg, _ := f.state.Snapshot(true); msg != "muelle no pudo conectarse con la admin API de Caddy; se reintenta solo" {
		t.Fatalf("mensaje: %q", msg)
	}
	if f.fails.Load() != 1 {
		t.Fatal("onFail")
	}
}

// D16: con { o } en el email no se manda nada, y el estado dice por qué
// (sin el email ni el token).
func TestSyncPlaceholder(t *testing.T) {
	f := newSyncFixture(t)
	f.enable(t, func(p *shared.ProxySettings) { p.ACMEEmail = "a{x}@b.cl"; p.CloudflareToken = syncToken })
	err := f.syncer.Sync(context.Background(), false)
	if !errors.Is(err, service.ErrPlaceholderInSettings) {
		t.Fatalf("Sync: %v", err)
	}
	if len(f.caddy.Loads()) != 0 {
		t.Fatal("no debió mandar nada")
	}
	status, msg, _ := f.state.Snapshot(true)
	if status != ProxyOutOfSync || msg != "el email ACME o el token de Cloudflare tienen { o }, que Caddy tomaría como placeholders; corrígelo en Ajustes" {
		t.Fatalf("estado: %s %q", status, msg)
	}
	if log := f.log.String(); strings.Contains(log, syncToken) || strings.Contains(log, "a{x}@b.cl") {
		t.Fatalf("log: %s", log)
	}
}

// Un error de la base deja un mensaje genérico en el estado y el detalle en el log.
func TestSyncInternalError(t *testing.T) {
	f := newSyncFixture(t)
	f.enable(t, nil)
	ctx := context.Background()
	if _, err := f.db.Conn(ctx).ExecContext(ctx, `UPDATE settings SET value = 'no-es-un-puerto' WHERE key = ?`, shared.SettingProxyHTTPPort); err != nil {
		t.Fatal(err)
	}
	if err := f.syncer.Sync(ctx, false); err == nil {
		t.Fatal("debió fallar")
	}
	if _, msg, _ := f.state.Snapshot(true); msg != "error interno armando la configuración" {
		t.Fatalf("mensaje: %q", msg)
	}
	if !strings.Contains(f.log.String(), "no-es-un-puerto") {
		t.Fatalf("el log tiene el detalle: %s", f.log.String())
	}
	if len(f.caddy.Loads()) != 0 || f.fails.Load() != 1 {
		t.Fatalf("loads %d, onFail %d", len(f.caddy.Loads()), f.fails.Load())
	}
}

func TestSyncForce(t *testing.T) {
	f := newSyncFixture(t)
	f.enable(t, nil)
	if err := f.syncer.Sync(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if got := f.caddy.Forced(); len(got) != 1 || !got[0] {
		t.Fatalf("Forced: %v", got)
	}
}

// D14: un POST /load a la vez, y el último lleva lo que hay en la base.
func TestSyncSerialized(t *testing.T) {
	f := newSyncFixture(t)
	f.enable(t, nil)
	f.caddy.Delay(5 * time.Millisecond)
	burst := func(n int) {
		var wg sync.WaitGroup
		for range n {
			wg.Go(func() {
				if err := f.syncer.Sync(context.Background(), false); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
	}
	burst(20)
	f.addDomain(t, "nuevo.auditfy.cl")
	burst(5)
	if got := f.caddy.MaxInFlight(); got != 1 {
		t.Fatalf("MaxInFlight = %d", got)
	}
	if n := len(f.caddy.Loads()); n != 25 {
		t.Fatalf("%d loads", n)
	}
	if !strings.Contains(f.lastLoad(t), `"nuevo.auditfy.cl"`) {
		t.Fatal("el último Load no tiene el dominio nuevo")
	}
}

// adminFunc es una admin API de Caddy hecha con una función.
type adminFunc func(ctx context.Context, cfg service.ProxyConfig, force bool) error

func (f adminFunc) Load(ctx context.Context, cfg service.ProxyConfig, force bool) error {
	return f(ctx, cfg, force)
}

const syncOriginKey = "-----BEGIN PRIVATE KEY-----\nMIGHAgEAMBMGByqGSM49AgEGCCqGSM49ClaveDelSync\n-----END PRIVATE KEY-----\n"

// seedOrigin deja host como un dominio que califica (manejado, proxied, con
// zona z1), con un active que vence en 90 días y visto con el proxy en
// Cloudflare.
func (f *syncFixture) seedOrigin(t *testing.T, host string) {
	t.Helper()
	ctx := context.Background()
	d, err := entity.NewDomain(f.apiID, host, "api", 3000, true, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	d.TLSMode, d.CFZoneID, d.CFRecordID = entity.TLSCloudflare, "z1", "rec-"+host
	if err := f.repo.Create(ctx, &d); err != nil {
		t.Fatal(err)
	}
	c := entity.OriginCert{Hostname: host, ZoneID: "z1", CFCertID: "cf-" + host, CertPEM: "-----BEGIN CERTIFICATE-----\nMIIBsync\n-----END CERTIFICATE-----\n",
		KeyPEM: syncOriginKey, ExpiresAt: f.clock.Now().Add(90 * 24 * time.Hour), CreatedAt: f.clock.Now()}
	if err := reposqlite.NewOriginCerts(f.db, f.cipher).Insert(ctx, &c); err != nil {
		t.Fatal(err)
	}
	if err := reposqlite.NewOriginHosts(f.db).SetObserved(ctx, host, entity.ObservedProxied, f.clock.Now()); err != nil {
		t.Fatal(err)
	}
}

// El Syncer lee los certificados de Origin CA (con la clave descifrada) y lo
// observado, y los manda a Caddy solo con la opción encendida y con token.
func TestSyncOriginCA(t *testing.T) {
	f := newSyncFixture(t)
	f.seedOrigin(t, "api.auditfy.cl")
	on := func(p *shared.ProxySettings) { p.CloudflareToken, p.CloudflareOriginCA = syncToken, true }
	f.enable(t, on)
	if err := f.syncer.Sync(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	body := f.lastLoad(t)
	if !strings.Contains(body, `"load_pem"`) || !strings.Contains(body, `ClaveDelSync`) || !strings.Contains(body, `"skip_certificates":["api.auditfy.cl"]`) {
		t.Fatalf("sin el certificado de Origin CA: %s", body)
	}
	if strings.Contains(f.log.String(), "ClaveDelSync") {
		t.Fatalf("la clave en el log: %s", f.log.String())
	}
	f.enable(t, func(p *shared.ProxySettings) { p.CloudflareToken = syncToken })
	if err := f.syncer.Sync(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if body := f.lastLoad(t); strings.Contains(body, `"load_pem"`) || strings.Contains(body, "ClaveDelSync") {
		t.Fatalf("con la opción apagada no va: %s", body)
	}
}

// LastGoodBuild es el momento en que empezó a leer la base el último Sync
// cuyo POST /load salió bien (§6 del diseño de Origin CA): no el del fin, y
// no avanza si el Load falla.
func TestSyncLastGoodBuild(t *testing.T) {
	f := newSyncFixture(t)
	f.enable(t, nil)
	var fail atomic.Bool
	admin := adminFunc(func(ctx context.Context, cfg service.ProxyConfig, force bool) error {
		f.clock.Advance(time.Minute) // el Load tarda: termina un minuto después
		if fail.Load() {
			return &service.CaddyRejected{Message: "no"}
		}
		return nil
	})
	s := NewSyncer(outservice.NewProxySettingsReader(f.db, f.cipher), f.repo, outservice.NewCatalogReader(f.db), OriginRepos{},
		admin, f.state, f.clock, "", false, nil, slog.New(slog.DiscardHandler))
	if !s.LastGoodBuild().IsZero() {
		t.Fatal("antes del primer Sync es cero")
	}
	start := f.clock.Now()
	if err := s.Sync(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if got := s.LastGoodBuild(); !got.Equal(start) {
		t.Fatalf("LastGoodBuild = %s, esperaba %s (el inicio, no el fin)", got, start)
	}
	fail.Store(true)
	if err := s.Sync(context.Background(), false); err == nil {
		t.Fatal("debió fallar")
	}
	if got := s.LastGoodBuild(); !got.Equal(start) {
		t.Fatalf("un Load que falla no lo mueve: %s", got)
	}
	// Con el proxy deshabilitado no se manda nada, y tampoco avanza.
	ingresstest.SetProxySettings(t, f.db, f.cipher, shared.DefaultProxySettings())
	fail.Store(false)
	if err := s.Sync(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if got := s.LastGoodBuild(); !got.Equal(start) {
		t.Fatalf("deshabilitado: %s", got)
	}
}
