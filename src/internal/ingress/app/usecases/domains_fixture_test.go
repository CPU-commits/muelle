package usecases

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/app/dto"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	reposqlite "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/repository/sqlite"
	outservice "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/caddy"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/caddy/caddytest"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare/cftest"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

// fakeProbe es un CertProbe que devuelve lo que pone el test: obs[host], o
// def; err si no es nil. Con block, cada Probe espera a que se cierre.
type fakeProbe struct {
	mu    sync.Mutex
	obs   map[string]service.CertObservation
	def   service.CertObservation
	err   error
	block chan struct{}
	calls []string
	// probing recibe el host cuando un Probe empieza (si no es nil).
	probing chan string
}

func (p *fakeProbe) Probe(ctx context.Context, host string) (service.CertObservation, error) {
	p.mu.Lock()
	p.calls = append(p.calls, host)
	block, probing := p.block, p.probing
	p.mu.Unlock()
	if probing != nil {
		probing <- host
	}
	if block != nil {
		<-block
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return service.CertObservation{}, p.err
	}
	if o, ok := p.obs[host]; ok {
		return o, nil
	}
	return p.def, nil
}

func (p *fakeProbe) set(host string, o service.CertObservation) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.obs == nil {
		p.obs = map[string]service.CertObservation{}
	}
	p.obs[host] = o
}

func (p *fakeProbe) setErr(err error) { p.mu.Lock(); p.err = err; p.mu.Unlock() }

func (p *fakeProbe) probed() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.calls)
}

// ucFixture arma los casos de uso de dominios como el módulo, con sqlite,
// cftest y el cliente de Cloudflare real, caddytest y el Syncer, un reloj
// manual, la auditoría en la base y una sonda falsa dentro del CertChecker
// real (sin Run: el test lee su cola first). El proxy queda synced.
type ucFixture struct {
	db     *sqlite.DB
	cipher *crypto.Cipher
	cf     *cftest.Server
	caddy  *caddytest.Admin
	clock  *clock.Manual
	repo   *reposqlite.Domains
	state  *ProxyState
	syncer *Syncer
	probe  *fakeProbe
	certs  *CertChecker
	ops    *Ops
	deps   DomainDeps
	// apiID es la app de formulario auditfy/staging/api; infraID la compose
	// auditfy/staging/infra.
	apiID, infraID int64
}

// goodCert es un certificado válido que vence en 60 días.
func (f *ucFixture) goodCert() service.CertObservation {
	return service.CertObservation{Valid: true, Issuer: "Let's Encrypt", NotAfter: f.clock.Now().Add(60 * 24 * time.Hour)}
}

var invalidCert = service.CertObservation{Problem: "el certificado no lo firmó una CA de confianza"}

func newUCFixture(t *testing.T) *ucFixture {
	t.Helper()
	db := sqlitetest.New(t)
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := &ucFixture{
		db: db, cipher: c, cf: cftest.New(t), caddy: caddytest.New(t),
		clock: clock.NewManual(time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)),
		repo:  reposqlite.NewDomains(db), state: NewProxyState(), probe: &fakeProbe{def: invalidCert}, ops: &Ops{},
	}
	f.cf.AddToken(dnsToken)
	f.cf.AddZone("z1", "tudominio.cl")
	settings := outservice.NewProxySettingsReader(db, c)
	catalog := outservice.NewCatalogReader(db)
	f.syncer = NewSyncer(settings, f.repo, catalog, caddy.NewAdmin(f.caddy.Dial), f.state, f.clock, "", false, nil, log)
	f.certs = NewCertChecker(f.probe, f.repo, f.state, f.ops, f.clock, time.Second, time.Second, time.Second, log)
	f.deps = DomainDeps{
		Repo: f.repo, Catalog: catalog, Settings: settings, DNS: cloudflare.New(f.cf.URL()), Syncer: f.syncer,
		Certs: f.certs, Ops: f.ops, Clock: f.clock, Audit: sqlite.NewAuditRecorder(db, f.clock, log), Log: log,
		DomainOp: 30 * time.Second, DomainRetry: 20 * time.Second,
	}
	f.apiID = ingresstest.App(t, db, "auditfy", "staging", "api", "service")
	f.infraID = ingresstest.App(t, db, "auditfy", "staging", "infra", "compose")
	f.settings(t, nil)
	f.state.Loaded(f.clock.Now())
	return f
}

// settings guarda los ajustes: el proxy habilitado (panel.tudominio.cl y
// caddy_image), el token y las dos IPs, con los cambios de mod.
func (f *ucFixture) settings(t *testing.T, mod func(*shared.ProxySettings)) {
	t.Helper()
	p := shared.DefaultProxySettings()
	p.PanelDomain, p.CaddyImage = "panel.tudominio.cl", "caddy:2-alpine"
	p.CloudflareToken, p.PublicIP, p.PublicIPv6 = dnsToken, dnsIPv4, dnsIPv6
	if mod != nil {
		mod(&p)
	}
	ingresstest.SetProxySettings(t, f.db, f.cipher, p)
}

// put guarda un dominio de la app api tal cual, sin pasar por los casos de uso.
func (f *ucFixture) put(t *testing.T, mod func(*entity.Domain)) entity.Domain {
	t.Helper()
	d, err := entity.NewDomain(f.apiID, dnsHost, "api", 3000, false, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	if mod != nil {
		mod(&d)
	}
	if err := f.repo.Create(context.Background(), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// get lee el dominio de la base.
func (f *ucFixture) get(t *testing.T, host string) entity.Domain {
	t.Helper()
	d, err := f.repo.Get(context.Background(), host)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// scheduled son los hostnames que agendó ScheduleFirst (vacía la cola).
func (f *ucFixture) scheduled() []string {
	var out []string
	for {
		select {
		case h := <-f.certs.first:
			out = append(out, h)
		default:
			return out
		}
	}
}

// audits son las acciones de auditoría, con su target y details, en orden.
func (f *ucFixture) audits(t *testing.T) []string {
	t.Helper()
	return auditRows(t, f.db)
}

// auditRows son las filas de audit_events como "acción target details", en
// orden.
func auditRows(t *testing.T, db *sqlite.DB) []string {
	t.Helper()
	ctx := context.Background()
	rows, err := db.Conn(ctx).QueryContext(ctx, `SELECT action, target, details FROM audit_events ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a, target, details string
		if err := rows.Scan(&a, &target, &details); err != nil {
			t.Fatal(err)
		}
		out = append(out, a+" "+target+" "+details)
	}
	return out
}

var (
	apiRef   = dto.AppRef{Project: "auditfy", Stage: "staging", App: "api"}
	infraRef = dto.AppRef{Project: "auditfy", Stage: "staging", App: "infra"}
)

func ptr[T any](v T) *T { return &v }

// create crea host en la app api por el caso de uso (target api:3000).
func (f *ucFixture) create(t *testing.T, host string, proxied bool) dto.DomainOutput {
	t.Helper()
	out, err := NewCreateDomain(f.deps).Execute(context.Background(), dto.CreateDomainInput{
		AppRef: apiRef, Hostname: host, TargetService: "api", TargetPort: 3000, Proxied: &proxied,
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// writes son las llamadas a cftest que cambian algo.
func (f *ucFixture) writes() []string {
	return slices.DeleteFunc(f.cf.Calls(), func(c string) bool { return strings.HasPrefix(c, "GET ") })
}

// recordsOf son los registros de host en z1, por tipo.
func (f *ucFixture) recordsOf(host, typ string) []service.Record {
	return slices.DeleteFunc(f.cf.Records("z1"), func(r service.Record) bool { return r.Name != host || r.Type != typ })
}

// lastLoad es el último POST /load, como texto.
func (f *ucFixture) lastLoad(t *testing.T) string {
	t.Helper()
	loads := f.caddy.Loads()
	if len(loads) == 0 {
		t.Fatal("no llegó ningún POST /load")
	}
	return string(loads[len(loads)-1])
}
