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
		caddy.NewAdmin(f.caddy.Dial), f.state, f.clock, "", false, func() { f.fails.Add(1) }, log)
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
