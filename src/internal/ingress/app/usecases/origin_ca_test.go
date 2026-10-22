package usecases

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

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

const (
	oToken = "cf-token-origen-1234567890abcdef"
	oPanel = "panel.auditfy.cl"
	day    = 24 * time.Hour
)

// oFixture arma EnsureOriginCA como el módulo, con SQLite real, el reloj
// manual (también el del Origin CA falso), cftest con la zona z1
// (auditfy.cl, con el permiso SSL para oToken) y el Syncer real contra
// caddytest. Los ajustes: el proxy habilitado, el token y la opción encendida.
type oFixture struct {
	db      *sqlite.DB
	cipher  *crypto.Cipher
	cf      *cftest.Server
	caddy   *caddytest.Admin
	clock   *clock.Manual
	domains *reposqlite.Domains
	origin  OriginRepos
	panel   *outservice.ProxySettingsReader
	syncer  *Syncer
	uc      *EnsureOriginCA
	apiID   int64
}

func newOFixture(t *testing.T) *oFixture {
	t.Helper()
	db := sqlitetest.New(t)
	c := testCipherFor(t)
	f := &oFixture{
		db: db, cipher: c, cf: cftest.New(t), caddy: caddytest.New(t),
		clock:   clock.NewManual(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)),
		domains: reposqlite.NewDomains(db),
		origin:  OriginRepos{Certs: reposqlite.NewOriginCerts(db, c), Hosts: reposqlite.NewOriginHosts(db)},
	}
	f.cf.SetClock(f.clock.Now)
	f.cf.AddToken(oToken)
	f.cf.AddZone("z1", "auditfy.cl")
	f.cf.GrantSSL(oToken, "z1")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	settings := outservice.NewProxySettingsReader(db, c)
	f.panel = settings
	f.syncer = NewSyncer(settings, f.domains, outservice.NewCatalogReader(db), f.origin, caddy.NewAdmin(f.caddy.Dial),
		NewProxyState(), f.clock, "", false, nil, log)
	client := cloudflare.New(f.cf.URL())
	f.uc = NewEnsureOriginCA(OriginDeps{
		Settings: settings, Domains: f.domains, Repos: f.origin, DNS: client, CA: client, Keys: outservice.OriginKeys{},
		Syncer: f.syncer, Clock: f.clock, Pass: 5 * time.Minute, Grace: time.Minute, Log: log,
	})
	f.apiID = ingresstest.App(t, db, "auditfy", "staging", "api", "service")
	f.settings(t, nil)
	return f
}

func (f *oFixture) settings(t *testing.T, mod func(*shared.ProxySettings)) {
	t.Helper()
	p := shared.DefaultProxySettings()
	p.PanelDomain, p.CaddyImage = oPanel, "caddy:2-alpine"
	p.CloudflareToken, p.CloudflareOriginCA = oToken, true
	if mod != nil {
		mod(&p)
	}
	ingresstest.SetProxySettings(t, f.db, f.cipher, p)
}

// managed crea un dominio manejado por muelle en z1: su A en cftest (con
// cfProxied) y el dominio con proxied en muelle.
func (f *oFixture) managed(t *testing.T, host string, proxied, cfProxied bool) entity.Domain {
	t.Helper()
	id := f.cf.PutRecord("z1", service.Record{Type: "A", Name: host, Content: "203.0.113.24", Proxied: cfProxied})
	return f.domain(t, host, func(d *entity.Domain) {
		d.TLSMode, d.CFZoneID, d.CFRecordID, d.Proxied, d.Status = entity.TLSCloudflare, "z1", id, proxied, entity.StatusActive
	})
}

func (f *oFixture) domain(t *testing.T, host string, mod func(*entity.Domain)) entity.Domain {
	t.Helper()
	d, err := entity.NewDomain(f.apiID, host, "api", 3000, false, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	mod(&d)
	if err := f.domains.Create(context.Background(), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// pass es una pasada que tiene que salir bien.
func (f *oFixture) pass(t *testing.T) {
	t.Helper()
	if err := f.uc.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// job es lo que hace el job "proxy" cada 5 min: avanza el reloj, manda la
// configuración y corre una pasada.
func (f *oFixture) job(t *testing.T, d time.Duration) {
	t.Helper()
	f.clock.Advance(d)
	if err := f.syncer.Sync(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	f.pass(t)
}

func (f *oFixture) active(t *testing.T, host string) *entity.OriginCert {
	t.Helper()
	c, err := f.origin.Certs.ActiveFor(context.Background(), host)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *oFixture) retired(t *testing.T) []entity.OriginCert {
	t.Helper()
	r, err := f.origin.Certs.Retired(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (f *oFixture) host(t *testing.T, host string) entity.OriginHost {
	t.Helper()
	h, err := f.origin.Hosts.Get(context.Background(), host)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// served son los hostnames con load_pem en el último POST /load.
func (f *oFixture) served(t *testing.T) []string {
	t.Helper()
	loads := f.caddy.Loads()
	if len(loads) == 0 {
		return nil
	}
	var out []string
	for _, part := range strings.Split(string(loads[len(loads)-1]), `"muelle_origin_ca:`)[1:] {
		out = append(out, part[:strings.Index(part, `"`)])
	}
	return out
}

// calls son las llamadas a Cloudflare que contienen sub.
func (f *oFixture) calls(sub string) []string {
	var out []string
	for _, c := range f.cf.Calls() {
		if strings.Contains(c, sub) {
			out = append(out, c)
		}
	}
	return out
}

// Con la opción apagada no se llama a Cloudflare ni a Caddy.
func TestOriginPassOff(t *testing.T) {
	f := newOFixture(t)
	f.managed(t, "api.auditfy.cl", true, true)
	f.settings(t, func(p *shared.ProxySettings) { p.CloudflareOriginCA = false })
	f.pass(t)
	if c := f.cf.Calls(); len(c) != 0 {
		t.Fatalf("llamó a Cloudflare: %q", c)
	}
	// Con el proxy deshabilitado, tampoco.
	f.settings(t, func(p *shared.ProxySettings) { p.PanelDomain = "" })
	f.pass(t)
	if c := f.cf.Calls(); len(c) != 0 || len(f.caddy.Loads()) != 0 {
		t.Fatalf("deshabilitado: %q, %d loads", c, len(f.caddy.Loads()))
	}
}

// Encendida: emite para el manejado con el proxy en muelle y en Cloudflare,
// y no para el DNS only (en muelle o en Cloudflare), el manual ni el ajeno.
// Lo emitido va a Caddy enseguida.
func TestOriginPassIssues(t *testing.T) {
	f := newOFixture(t)
	f.managed(t, "api.auditfy.cl", true, true)
	f.managed(t, "web.auditfy.cl", false, false)
	f.managed(t, "gris.auditfy.cl", true, false)
	f.domain(t, "manual.ejemplo.com", func(d *entity.Domain) { d.TLSMode = entity.TLSHTTP })
	f.domain(t, "ajeno.auditfy.cl", func(d *entity.Domain) {
		d.TLSMode, d.CFZoneID, d.Proxied = entity.TLSCloudflare, "z1", true
		d.Fail(entity.CodeRecordExists, "ya existe", f.clock.Now())
	})
	f.pass(t)

	issued := f.cf.Issued()
	if len(issued) != 1 || !slices.Equal(issued[0].Hostnames, []string{"api.auditfy.cl"}) || issued[0].Validity != 90 || issued[0].RequestType != "origin-ecc" {
		t.Fatalf("emitidos: %+v", issued)
	}
	a := f.active(t, "api.auditfy.cl")
	if a == nil || a.CFCertID != issued[0].ID || a.ZoneID != "z1" || !a.ExpiresAt.Equal(f.clock.Now().Add(90*day)) {
		t.Fatalf("active: %+v", a)
	}
	if h := f.host(t, "api.auditfy.cl"); h.Observed != entity.ObservedProxied || h.Failed() {
		t.Fatalf("api: %+v", h)
	}
	if h := f.host(t, "gris.auditfy.cl"); h.Observed != entity.ObservedDNSOnly {
		t.Fatalf("gris: %+v", h)
	}
	// Solo se leen los registros de los candidatos.
	if got := f.calls("dns_records"); len(got) != 2 || !strings.Contains(got[0], "api.auditfy.cl") || !strings.Contains(got[1], "gris.auditfy.cl") {
		t.Fatalf("lecturas: %q", got)
	}
	if got := f.served(t); !slices.Equal(got, []string{"api.auditfy.cl"}) {
		t.Fatalf("en Caddy: %v", got)
	}
	// Otra pasada no emite otra vez.
	f.job(t, 5*time.Minute)
	if len(f.cf.Issued()) != 1 {
		t.Fatalf("emitió otra vez: %+v", f.cf.Issued())
	}
}

// DNS only visto en Cloudflare: lo retira, manda la configuración sin él y
// lo revoca recién después de que Caddy cargó una configuración sin él y
// pasó el margen (§6 del diseño de Origin CA).
func TestOriginPassDNSOnlyRevokesAfterLoad(t *testing.T) {
	f := newOFixture(t)
	d := f.managed(t, "api.auditfy.cl", true, true)
	f.pass(t)
	id := f.active(t, "api.auditfy.cl").CFCertID

	f.cf.PutRecord("z1", service.Record{ID: d.CFRecordID, Type: "A", Name: "api.auditfy.cl", Content: "203.0.113.24"})
	f.job(t, 5*time.Minute)
	if a := f.active(t, "api.auditfy.cl"); a != nil {
		t.Fatalf("sigue active: %+v", a)
	}
	if r := f.retired(t); len(r) != 1 || r[0].CFCertID != id {
		t.Fatalf("retirados: %+v", r)
	}
	if got := f.served(t); len(got) != 0 {
		t.Fatalf("Caddy lo sigue sirviendo: %v", got)
	}
	if f.cf.Revoked(id) {
		t.Fatal("revocado antes de la carga y del margen")
	}
	// La carga que lo sacó empezó a leer cuando ya estaba retirado, pero el
	// margen de 1 min todavía no pasó.
	f.job(t, 30*time.Second)
	if f.cf.Revoked(id) {
		t.Fatal("revocado dentro del margen")
	}
	f.job(t, time.Minute)
	if !f.cf.Revoked(id) || len(f.retired(t)) != 0 {
		t.Fatalf("no se revocó: %v %+v", f.cf.Revoked(id), f.retired(t))
	}
}

// Cloudflare caído al leer los registros: vale lo último que vio, y no se
// retira nada.
func TestOriginPassCloudflareDown(t *testing.T) {
	f := newOFixture(t)
	f.managed(t, "api.auditfy.cl", true, true)
	f.pass(t)
	f.cf.SetDown(true)
	f.job(t, 5*time.Minute)
	if a := f.active(t, "api.auditfy.cl"); a == nil {
		t.Fatal("retiró con Cloudflare caído")
	}
	if h := f.host(t, "api.auditfy.cl"); h.Observed != entity.ObservedProxied {
		t.Fatalf("cambió lo observado: %+v", h)
	}
	if got := f.served(t); !slices.Equal(got, []string{"api.auditfy.cl"}) {
		t.Fatalf("en Caddy: %v", got)
	}
}

// Se renueva cuando faltan 30 días (con otra clave) y el viejo se revoca;
// si la renovación falla, a los 14 días deja de servirse.
func TestOriginPassRenews(t *testing.T) {
	f := newOFixture(t)
	f.managed(t, "api.auditfy.cl", true, true)
	f.pass(t)
	old := f.active(t, "api.auditfy.cl")
	f.job(t, 59*day) // faltan 31 días: todavía no
	if a := f.active(t, "api.auditfy.cl"); a.ID != old.ID {
		t.Fatalf("renovó antes: %+v", a)
	}
	f.job(t, 2*day) // faltan 29
	renewed := f.active(t, "api.auditfy.cl")
	if renewed.ID == old.ID || !renewed.ExpiresAt.Equal(f.clock.Now().Add(90*day)) {
		t.Fatalf("no renovó: %+v", renewed)
	}
	keys, err := f.origin.Certs.ActiveWithKeys(context.Background())
	if err != nil || keys["api.auditfy.cl"].KeyPEM == "" {
		t.Fatal(err)
	}
	f.job(t, 2*time.Minute)
	if !f.cf.Revoked(old.CFCertID) || f.cf.Revoked(renewed.CFCertID) {
		t.Fatalf("revocados: viejo %v, nuevo %v", f.cf.Revoked(old.CFCertID), f.cf.Revoked(renewed.CFCertID))
	}

	// La renovación falla (Cloudflare no responde): sigue sirviendo el actual
	// hasta que faltan 14 días, y ahí vuelve a Let's Encrypt.
	f.cf.SetDown(true)
	f.job(t, 61*day) // faltan 29
	if a := f.active(t, "api.auditfy.cl"); a.ID != renewed.ID {
		t.Fatalf("renovó: %+v", a)
	}
	if h := f.host(t, "api.auditfy.cl"); h.ErrorCode != entity.OriginCloudflare || h.Observed != entity.ObservedProxied {
		t.Fatalf("la falla: %+v", h)
	}
	if got := f.served(t); !slices.Equal(got, []string{"api.auditfy.cl"}) {
		t.Fatalf("a 29 días se sigue sirviendo: %v", got)
	}
	f.job(t, 15*day+time.Hour) // faltan menos de 14
	if got := f.served(t); len(got) != 0 {
		t.Fatalf("lo sigue sirviendo a menos de 14 días: %v", got)
	}
}

// Una emisión que falla registra el código, la causa y el reintento: 1, 2,
// 4… min si Cloudflare no responde, y 6 h si lo rechaza. Mientras espera, no
// se reintenta.
func TestOriginPassFailureBackoff(t *testing.T) {
	f := newOFixture(t)
	f.managed(t, "api.auditfy.cl", true, true)
	for i, wait := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute} {
		f.cf.FailNext("POST", "/certificates", 503, 0, "boom")
		f.pass(t)
		h := f.host(t, "api.auditfy.cl")
		if h.Failures != i+1 || h.ErrorCode != entity.OriginCloudflare || h.NextAttemptAt == nil || !h.NextAttemptAt.Equal(f.clock.Now().Add(wait)) ||
			h.LastError != "Cloudflare no respondió (HTTP 503); se reintenta solo" {
			t.Fatalf("falla %d: %+v", i+1, h)
		}
		posts := len(f.calls("POST /certificates"))
		f.clock.Advance(wait - time.Second)
		f.pass(t)
		if len(f.calls("POST /certificates")) != posts {
			t.Fatalf("reintentó antes del backoff (%s)", wait)
		}
		f.clock.Advance(time.Second)
	}
	// Sin el permiso: permission, 6 h.
	f.cf.RevokeSSL(oToken, "z1")
	f.pass(t)
	h := f.host(t, "api.auditfy.cl")
	if h.ErrorCode != entity.OriginPermission || !h.NextAttemptAt.Equal(f.clock.Now().Add(6*time.Hour)) ||
		!strings.Contains(h.LastError, "en la zona de api.auditfy.cl (Cloudflare: Authentication error); agrégalo al token y pulsa Reintentar") {
		t.Fatalf("permission: %+v", h)
	}
	if f.active(t, "api.auditfy.cl") != nil {
		t.Fatal("guardó algo")
	}
	// Una emisión que sale bien borra la falla.
	f.cf.GrantSSL(oToken, "z1")
	f.clock.Advance(6 * time.Hour)
	f.pass(t)
	if h := f.host(t, "api.auditfy.cl"); h.Failed() || f.active(t, "api.auditfy.cl") == nil {
		t.Fatalf("después: %+v", h)
	}
}

// Un 429 bloquea el token 5 min: la pasada termina ahí, sin más llamadas, y
// cuenta como cloudflare para el host que lo recibió.
func TestOriginPass429(t *testing.T) {
	f := newOFixture(t)
	f.managed(t, "a.auditfy.cl", true, true)
	f.managed(t, "b.auditfy.cl", true, true)
	f.cf.FailNext("POST", "/certificates", 429, 971, "Please wait and consider throttling your request speed")
	f.pass(t)
	if got := f.calls("b.auditfy.cl"); len(got) != 0 {
		t.Fatalf("siguió después del 429: %q", got)
	}
	if h := f.host(t, "a.auditfy.cl"); h.ErrorCode != entity.OriginCloudflare || h.Failures != 1 {
		t.Fatalf("a: %+v", h)
	}
	// Un 429 al leer los registros también corta, sin una falla.
	f.cf.FailNext("GET", "a.auditfy.cl", 429, 971, "Please wait")
	before := len(f.cf.Calls())
	f.clock.Advance(time.Minute)
	f.pass(t)
	if got := f.cf.Calls()[before:]; len(got) != 1 {
		t.Fatalf("llamadas después del 429: %q", got)
	}
}

// No revoca si el Sync falló, ni si el retiro es de después del último Sync
// bueno (la carrera: un Sync que leyó la base antes del retiro y terminó
// después), ni antes del margen.
func TestOriginPassNoRevokeWithoutGoodLoad(t *testing.T) {
	f := newOFixture(t)
	d := f.managed(t, "api.auditfy.cl", true, true)
	f.pass(t)
	id := f.active(t, "api.auditfy.cl").CFCertID
	f.cf.PutRecord("z1", service.Record{ID: d.CFRecordID, Type: "A", Name: "api.auditfy.cl", Content: "203.0.113.24"})
	f.caddy.Reject("no")
	f.clock.Advance(time.Minute)
	if err := f.uc.Execute(context.Background()); err == nil {
		t.Fatal("el Sync falló: la pasada devuelve el error")
	}
	if len(f.retired(t)) != 1 {
		t.Fatal("no retiró")
	}
	// Sin un Sync bueno después del retiro, nunca se revoca.
	f.clock.Advance(10 * time.Minute)
	_ = f.uc.Execute(context.Background())
	if f.cf.Revoked(id) {
		t.Fatal("revocó sin una carga buena")
	}
	f.caddy.Accept()
	f.job(t, time.Minute)
	if !f.cf.Revoked(id) {
		t.Fatal("no revocó después de la carga buena")
	}
}

// Revocar uno desconocido (404) o ya revocado (1014) borra la fila.
func TestOriginPassRevokeGone(t *testing.T) {
	f := newOFixture(t)
	ctx := context.Background()
	for _, id := range []string{"999", "998"} {
		c := entity.OriginCert{Hostname: "x.auditfy.cl", ZoneID: "z1", CFCertID: id, CertPEM: "PEM", ExpiresAt: f.clock.Now().Add(90 * day), CreatedAt: f.clock.Now()}
		if err := f.origin.Certs.InsertRetired(ctx, &c, f.clock.Now()); err != nil {
			t.Fatal(err)
		}
	}
	f.cf.FailNext("DELETE", "/certificates/998", 400, 1014, "Certificate already revoked")
	f.job(t, 2*time.Minute)
	if r := f.retired(t); len(r) != 0 {
		t.Fatalf("quedaron: %+v", r)
	}
}

// Sin token no se puede revocar: los retirados quedan, y se borran al vencer.
func TestOriginPassNoTokenKeepsRetired(t *testing.T) {
	f := newOFixture(t)
	f.managed(t, "api.auditfy.cl", true, true)
	f.pass(t)
	f.settings(t, func(p *shared.ProxySettings) { p.CloudflareToken = "" })
	f.job(t, time.Minute) // sin token no hay candidatos: lo retira
	f.job(t, 5*time.Minute)
	if r := f.retired(t); len(r) != 1 || f.cf.Revoked(r[0].CFCertID) {
		t.Fatalf("retirados: %+v", r)
	}
	f.job(t, 91*day)
	if r := f.retired(t); len(r) != 0 {
		t.Fatalf("vencido y no se borró: %+v", r)
	}
	if got := f.calls("DELETE"); len(got) != 0 {
		t.Fatalf("revocó sin token: %q", got)
	}
}

// Un dominio borrado, o que cambió de zona, deja de ser candidato de ese
// certificado: se retira y se revoca; con otra zona se emite uno nuevo.
func TestOriginPassDeletedAndZoneChange(t *testing.T) {
	f := newOFixture(t)
	ctx := context.Background()
	api := f.managed(t, "api.auditfy.cl", true, true)
	web := f.managed(t, "web.auditfy.cl", true, true)
	f.pass(t)
	apiCert, webCert := f.active(t, "api.auditfy.cl"), f.active(t, "web.auditfy.cl")

	if err := f.domains.Delete(ctx, api.ID); err != nil {
		t.Fatal(err)
	}
	// web pasa a otra zona (z2, donde el token también tiene el permiso).
	f.cf.AddZone("z2", "web.auditfy.cl")
	f.cf.GrantSSL(oToken, "z2")
	rec := f.cf.PutRecord("z2", service.Record{Type: "A", Name: "web.auditfy.cl", Content: "203.0.113.24", Proxied: true})
	web.CFZoneID, web.CFRecordID = "z2", rec
	if err := f.domains.Update(ctx, web); err != nil {
		t.Fatal(err)
	}
	f.job(t, time.Minute)
	if f.active(t, "api.auditfy.cl") != nil {
		t.Fatal("el del dominio borrado sigue active")
	}
	if a := f.active(t, "web.auditfy.cl"); a == nil || a.ZoneID != "z2" || a.ID == webCert.ID {
		t.Fatalf("otra zona: %+v", a)
	}
	f.job(t, 2*time.Minute)
	if !f.cf.Revoked(apiCert.CFCertID) || !f.cf.Revoked(webCert.CFCertID) {
		t.Fatal("no revocó los viejos")
	}
	// La fila de lo observado del dominio borrado se va.
	if all, _ := f.origin.Hosts.All(ctx); len(all) != 1 {
		t.Fatalf("filas: %v", all)
	}
}

// El panel califica con un registro ajeno con el proxy (muelle solo lo
// lee); en DNS only no, y con otro panel_domain el viejo se revoca.
func TestOriginPassPanel(t *testing.T) {
	f := newOFixture(t)
	ctx := context.Background()
	if err := f.panel.SavePanelDNS(ctx, service.PanelDNS{ZoneID: "z1"}); err != nil {
		t.Fatal(err)
	}
	rec := f.cf.PutRecord("z1", service.Record{Type: "CNAME", Name: oPanel, Content: "otro.auditfy.cl", Proxied: true})
	f.pass(t)
	first := f.active(t, oPanel)
	if first == nil || !slices.Equal(f.served(t), []string{oPanel}) {
		t.Fatalf("panel: %+v %v", first, f.served(t))
	}
	if h := f.host(t, oPanel); h.Observed != entity.ObservedProxied {
		t.Fatalf("observado: %+v", h)
	}
	f.cf.PutRecord("z1", service.Record{ID: rec, Type: "CNAME", Name: oPanel, Content: "otro.auditfy.cl"})
	f.job(t, time.Minute)
	if f.active(t, oPanel) != nil || len(f.served(t)) != 0 {
		t.Fatal("DNS only y sigue")
	}
	f.cf.PutRecord("z1", service.Record{ID: rec, Type: "CNAME", Name: oPanel, Content: "otro.auditfy.cl", Proxied: true})
	f.job(t, 2*time.Minute)
	second := f.active(t, oPanel)
	if second == nil || !f.cf.Revoked(first.CFCertID) {
		t.Fatalf("de vuelta: %+v, revocado el primero %v", second, f.cf.Revoked(first.CFCertID))
	}
	// Otro panel_domain: el host viejo deja de ser candidato.
	f.settings(t, func(p *shared.ProxySettings) { p.PanelDomain = "nuevo.auditfy.cl" })
	f.job(t, time.Minute)
	f.job(t, 2*time.Minute)
	if f.active(t, oPanel) != nil || !f.cf.Revoked(second.CFCertID) {
		t.Fatal("el del panel viejo no se revocó")
	}
}

// ResetBackoff pone el reintento en la hora del pedido y deja la falla, que
// se ve hasta que la pasada emite; Forget borra lo observado y las fallas.
func TestOriginResetBackoffAndForget(t *testing.T) {
	f := newOFixture(t)
	ctx := context.Background()
	f.managed(t, "api.auditfy.cl", true, true)
	f.cf.RevokeSSL(oToken, "z1")
	f.pass(t)
	f.clock.Advance(time.Minute)
	if err := f.uc.ResetBackoff(ctx, "api.auditfy.cl"); err != nil {
		t.Fatal(err)
	}
	if h := f.host(t, "api.auditfy.cl"); h.Failures != 1 || !h.NextAttemptAt.Equal(f.clock.Now()) {
		t.Fatalf("reinicio: %+v", h)
	}
	f.cf.GrantSSL(oToken, "z1")
	f.pass(t)
	if h := f.host(t, "api.auditfy.cl"); h.Failed() || f.active(t, "api.auditfy.cl") == nil {
		t.Fatalf("después del reinicio: %+v", h)
	}
	if err := f.uc.ForgetObserved(ctx, "api.auditfy.cl"); err != nil {
		t.Fatal(err)
	}
	if h := f.host(t, "api.auditfy.cl"); h.Observed != "" {
		t.Fatalf("ForgetObserved: %+v", h)
	}
	if err := f.uc.Forget(ctx); err != nil {
		t.Fatal(err)
	}
	if all, _ := f.origin.Hosts.All(ctx); len(all) != 0 || f.active(t, "api.auditfy.cl") == nil {
		t.Fatalf("Forget: %v", all)
	}
}

// Kick no bloquea, y Run corre una pasada por cada Kick (con dos seguidos,
// a lo más dos).
func TestOriginKickRun(t *testing.T) {
	f := newOFixture(t)
	f.managed(t, "api.auditfy.cl", true, true)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { f.uc.Run(ctx); close(done) }()
	f.uc.Kick()
	f.uc.Kick()
	f.uc.Kick()
	deadline := time.Now().Add(5 * time.Second)
	for len(f.cf.Issued()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if len(f.cf.Issued()) != 1 {
		t.Fatalf("emitidos: %+v", f.cf.Issued())
	}
	if errors.Is(ctx.Err(), context.Canceled) == false {
		t.Fatal("ctx")
	}
}
