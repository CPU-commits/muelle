package usecases

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	reposqlite "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/repository/sqlite"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare/cftest"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

const (
	dnsToken = "cf-token-1234567890abcdef"
	dnsHost  = "api.tudominio.cl"
	dnsIPv4  = "203.0.113.24"
	dnsIPv6  = "2001:db8::24"
)

// dnsFixture es dnsOps con el cliente real contra cftest (la zona z1
// tudominio.cl y el token dnsToken) y los ajustes con las dos IPs.
type dnsFixture struct {
	cf    *cftest.Server
	clock *clock.Manual
	ops   dnsOps
	p     shared.ProxySettings
}

func newDNSFixture(t *testing.T) *dnsFixture {
	t.Helper()
	cf := cftest.New(t)
	cf.AddToken(dnsToken)
	cf.AddZone("z1", "tudominio.cl")
	clk := clock.NewManual(time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC))
	p := shared.DefaultProxySettings()
	p.CloudflareToken, p.PublicIP, p.PublicIPv6 = dnsToken, dnsIPv4, dnsIPv6
	return &dnsFixture{cf: cf, clock: clk, ops: dnsOps{dns: cloudflare.New(cf.URL()), clock: clk}, p: p}
}

// domain es un dominio recién creado (NewDomain: pending, tls http).
func (f *dnsFixture) domain(t *testing.T, host string, proxied bool) entity.Domain {
	t.Helper()
	d, err := entity.NewDomain(1, host, "api", 3000, proxied, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// managed es un dominio con su A propio en z1 (y su AAAA, si aaaa), en error
// + cloudflare.
func (f *dnsFixture) managed(t *testing.T, proxied, aaaa bool) entity.Domain {
	t.Helper()
	d := f.domain(t, dnsHost, proxied)
	d.CFZoneID, d.TLSMode = "z1", entity.TLSCloudflare
	d.CFRecordID = f.cf.PutRecord("z1", service.Record{Type: "A", Name: dnsHost, Content: "198.51.100.1", Proxied: proxied})
	if aaaa {
		d.CFRecordIDAAAA = f.cf.PutRecord("z1", service.Record{Type: "AAAA", Name: dnsHost, Content: "2001:db8::1", Proxied: proxied})
	}
	f.clock.Advance(time.Minute)
	d.Fail(entity.CodeCloudflare, "Cloudflare no responde; reintenta en unos minutos", f.clock.Now())
	f.clock.Advance(time.Minute)
	return d
}

// record es el registro de la zona con ese tipo (falla si hay más de uno o ninguno).
func (f *dnsFixture) record(t *testing.T, typ string) service.Record {
	t.Helper()
	var out []service.Record
	for _, r := range f.cf.Records("z1") {
		if r.Type == typ && r.Name == dnsHost {
			out = append(out, r)
		}
	}
	if len(out) != 1 {
		t.Fatalf("registros %s: %+v", typ, out)
	}
	return out[0]
}

// writes son las llamadas a cftest que cambian algo.
func (f *dnsFixture) writes() []string {
	return slices.DeleteFunc(f.cf.Calls(), func(c string) bool { return strings.HasPrefix(c, "GET ") })
}

func TestEnsureNoToken(t *testing.T) {
	f := newDNSFixture(t)
	f.p.CloudflareToken = ""
	d := f.domain(t, dnsHost, true)
	f.ops.ensure(context.Background(), &d, f.p)
	if d.TLSMode != entity.TLSHTTP || d.Status != entity.StatusPending || d.Proxied || d.CFZoneID != "" || d.CFRecordID != "" {
		t.Fatalf("%+v", d)
	}
	if calls := f.cf.Calls(); len(calls) != 0 {
		t.Fatalf("no debía llamar a Cloudflare: %v", calls)
	}
}

func TestEnsureNoZone(t *testing.T) {
	f := newDNSFixture(t)
	d := f.domain(t, "api.otro.com", true)
	f.ops.ensure(context.Background(), &d, f.p)
	if d.TLSMode != entity.TLSHTTP || d.Status != entity.StatusPending || d.Proxied || d.CFZoneID != "" || d.CFRecordID != "" {
		t.Fatalf("%+v", d)
	}
}

func TestEnsureCreates(t *testing.T) {
	f := newDNSFixture(t)
	d := f.domain(t, dnsHost, true)
	f.ops.ensure(context.Background(), &d, f.p)
	if d.Status != entity.StatusPending || d.TLSMode != entity.TLSCloudflare || d.CFZoneID != "z1" || d.CFRecordID == "" || d.CFRecordIDAAAA == "" {
		t.Fatalf("%+v", d)
	}
	a, aaaa := f.record(t, "A"), f.record(t, "AAAA")
	if a.ID != d.CFRecordID || a.Content != dnsIPv4 || !a.Proxied || f.cf.Comment("z1", a.ID) != cloudflare.RecordComment {
		t.Fatalf("A: %+v (%q)", a, f.cf.Comment("z1", a.ID))
	}
	if aaaa.ID != d.CFRecordIDAAAA || aaaa.Content != dnsIPv6 || !aaaa.Proxied || f.cf.Comment("z1", aaaa.ID) != cloudflare.RecordComment {
		t.Fatalf("AAAA: %+v", aaaa)
	}
}

func TestEnsureNoPublicIP(t *testing.T) {
	f := newDNSFixture(t)
	f.p.PublicIP = ""
	d := f.domain(t, dnsHost, false)
	f.ops.ensure(context.Background(), &d, f.p)
	if d.Status != entity.StatusError || d.ErrorCode != entity.CodeCloudflare || d.LastError != "configura la IP pública en Ajustes" ||
		d.TLSMode != entity.TLSCloudflare || d.CFZoneID != "z1" || d.CFRecordID != "" {
		t.Fatalf("%+v", d)
	}
	if w := f.writes(); len(w) != 0 {
		t.Fatalf("no debía crear nada: %v", w)
	}
}

// Un registro ajeno no se toca, y la zona queda (D5, D7).
func TestEnsureForeign(t *testing.T) {
	for _, r := range []service.Record{
		{Type: "A", Name: dnsHost, Content: "198.51.100.7"},
		{Type: "AAAA", Name: dnsHost, Content: "2001:db8::7"},
		{Type: "CNAME", Name: dnsHost, Content: "otro.example.com"},
	} {
		t.Run(r.Type, func(t *testing.T) {
			f := newDNSFixture(t)
			f.cf.PutRecord("z1", r)
			d := f.domain(t, dnsHost, true)
			f.ops.ensure(context.Background(), &d, f.p)
			if d.Status != entity.StatusError || d.ErrorCode != entity.CodeRecordExists || !strings.Contains(d.LastError, r.Type) ||
				d.TLSMode != entity.TLSCloudflare || d.CFZoneID != "z1" || d.CFRecordID != "" || d.CFRecordIDAAAA != "" || !d.Proxied {
				t.Fatalf("%+v", d)
			}
			if w := f.writes(); len(w) != 0 {
				t.Fatalf("no debía tocar nada: %v", w)
			}
		})
	}
}

// Sin las zonas: error + cloudflare con tls http provisorio, y el proxied
// pedido se guarda para el reintento (D26).
func TestEnsureZonesFail(t *testing.T) {
	f := newDNSFixture(t)
	f.cf.SetDown(true)
	d := f.domain(t, dnsHost, true)
	f.ops.ensure(context.Background(), &d, f.p)
	if d.Status != entity.StatusError || d.ErrorCode != entity.CodeCloudflare || d.LastError != "Cloudflare no responde; reintenta en unos minutos" ||
		d.TLSMode != entity.TLSHTTP || d.CFZoneID != "" || !d.Proxied {
		t.Fatalf("%+v", d)
	}
}

// failAAAA es Cloudflare que rechaza crear un AAAA.
type failAAAA struct{ service.DNS }

func (f failAAAA) CreateRecord(ctx context.Context, token, zoneID string, r service.Record) (service.Record, error) {
	if r.Type == "AAAA" {
		return service.Record{}, &service.DNSError{Status: 400, Code: 9005, Message: "Content for AAAA record is invalid."}
	}
	return f.DNS.CreateRecord(ctx, token, zoneID, r)
}

// Si el AAAA falla después del A, el A queda manejado y el dominio en error.
func TestEnsureAAAAFails(t *testing.T) {
	f := newDNSFixture(t)
	f.ops.dns = failAAAA{f.ops.dns}
	d := f.domain(t, dnsHost, false)
	f.ops.ensure(context.Background(), &d, f.p)
	if d.Status != entity.StatusError || d.ErrorCode != entity.CodeCloudflare || !strings.Contains(d.LastError, "Content for AAAA record is invalid.") ||
		d.CFRecordID == "" || d.CFRecordIDAAAA != "" || d.TLSMode != entity.TLSCloudflare {
		t.Fatalf("%+v", d)
	}
	if a := f.record(t, "A"); a.ID != d.CFRecordID {
		t.Fatalf("A: %+v", a)
	}
}

// Con el A propio, ensure lo actualiza (contenido y proxied) y crea el AAAA
// que falta; sin public_ipv6, borra el AAAA propio.
func TestEnsureManaged(t *testing.T) {
	f := newDNSFixture(t)
	d := f.managed(t, true, false)
	before := d.CFRecordID
	f.ops.ensure(context.Background(), &d, f.p)
	if d.Status != entity.StatusPending || d.CFRecordID != before || d.CFRecordIDAAAA == "" || !d.UpdatedAt.Equal(f.clock.Now()) {
		t.Fatalf("%+v", d)
	}
	if a := f.record(t, "A"); a.Content != dnsIPv4 || !a.Proxied {
		t.Fatalf("A: %+v", a)
	}
	if aaaa := f.record(t, "AAAA"); aaaa.ID != d.CFRecordIDAAAA || aaaa.Content != dnsIPv6 || !aaaa.Proxied {
		t.Fatalf("AAAA: %+v", aaaa)
	}

	f.p.PublicIPv6 = ""
	f.ops.ensure(context.Background(), &d, f.p)
	if d.Status != entity.StatusPending || d.CFRecordIDAAAA != "" || d.CFRecordID != before {
		t.Fatalf("sin IPv6: %+v", d)
	}
	for _, r := range f.cf.Records("z1") {
		if r.Type == "AAAA" {
			t.Fatalf("el AAAA debía borrarse: %+v", r)
		}
	}
}

// Con el A propio y un AAAA ajeno, el AAAA no se crea: error + cloudflare.
func TestEnsureManagedForeignAAAA(t *testing.T) {
	f := newDNSFixture(t)
	d := f.managed(t, false, false)
	f.cf.PutRecord("z1", service.Record{Type: "AAAA", Name: dnsHost, Content: "2001:db8::7"})
	f.ops.ensure(context.Background(), &d, f.p)
	if d.Status != entity.StatusError || d.ErrorCode != entity.CodeCloudflare || !strings.Contains(d.LastError, "AAAA") || d.CFRecordIDAAAA != "" {
		t.Fatalf("%+v", d)
	}
	if slices.ContainsFunc(f.writes(), func(c string) bool { return strings.HasPrefix(c, "POST ") }) {
		t.Fatalf("no debía crear: %v", f.writes())
	}
}

// D29: si el A propio ya no está, se borra el AAAA propio y se olvidan los
// dos ids; después sigue como sin registros (crea los suyos).
func TestEnsureManagedAGone(t *testing.T) {
	ctx := context.Background()
	f := newDNSFixture(t)
	repo, d := f.saved(t, f.managed(t, false, true))
	oldAAAA := d.CFRecordIDAAAA
	f.removeA(t, d)
	f.ops.ensure(ctx, &d, f.p)
	if d.Status != entity.StatusPending || d.CFRecordID == "" || d.CFRecordIDAAAA == "" || d.CFRecordIDAAAA == oldAAAA {
		t.Fatalf("%+v", d)
	}
	if !slices.Contains(f.cf.Calls(), "DELETE /zones/z1/dns_records/"+oldAAAA) {
		t.Fatalf("el AAAA viejo debía borrarse: %v", f.cf.Calls())
	}
	if a, aaaa := f.record(t, "A"), f.record(t, "AAAA"); a.ID != d.CFRecordID || aaaa.ID != d.CFRecordIDAAAA {
		t.Fatalf("registros: %+v %+v", a, aaaa)
	}
	if err := repo.Update(ctx, d); err != nil {
		t.Fatalf("Update: %v", err)
	}
}

// D29 con el borrado del AAAA rechazado: igual se olvidan los dos ids, ese
// AAAA queda ajeno, y el dominio da record_exists (→ Adoptar).
func TestEnsureManagedAGoneDeleteFails(t *testing.T) {
	ctx := context.Background()
	f := newDNSFixture(t)
	repo, d := f.saved(t, f.managed(t, false, true))
	aaaa := d.CFRecordIDAAAA
	f.removeA(t, d)
	f.cf.FailNext("DELETE", aaaa, 403, 10000, "Authentication error")
	f.ops.ensure(ctx, &d, f.p)
	if d.Status != entity.StatusError || d.ErrorCode != entity.CodeRecordExists || d.CFRecordID != "" || d.CFRecordIDAAAA != "" ||
		d.CFZoneID != "z1" || d.TLSMode != entity.TLSCloudflare {
		t.Fatalf("%+v", d)
	}
	if !slices.ContainsFunc(f.cf.Records("z1"), func(r service.Record) bool { return r.ID == aaaa }) {
		t.Fatal("el AAAA debía quedar en Cloudflare")
	}
	if err := repo.Update(ctx, d); err != nil {
		t.Fatalf("Update: %v", err)
	}
}

// saved guarda d en una base nueva, con su app, y devuelve el repositorio.
func (f *dnsFixture) saved(t *testing.T, d entity.Domain) (*reposqlite.Domains, entity.Domain) {
	t.Helper()
	db := sqlitetest.New(t)
	d.AppID = ingresstest.App(t, db, "auditfy", "staging", "api", "service")
	repo := reposqlite.NewDomains(db)
	if err := repo.Create(context.Background(), &d); err != nil {
		t.Fatal(err)
	}
	return repo, d
}

// removeA borra en cftest el A propio de d, como si lo borraran en Cloudflare.
func (f *dnsFixture) removeA(t *testing.T, d entity.Domain) {
	t.Helper()
	if err := cloudflare.New(f.cf.URL()).DeleteRecord(context.Background(), dnsToken, "z1", d.CFRecordID); err != nil {
		t.Fatal(err)
	}
}

// Sin token, los registros propios no se pueden actualizar (D18).
func TestEnsureManagedNoToken(t *testing.T) {
	f := newDNSFixture(t)
	d := f.managed(t, false, false)
	f.p.CloudflareToken = ""
	f.ops.ensure(context.Background(), &d, f.p)
	if d.Status != entity.StatusError || d.ErrorCode != entity.CodeCloudflare || d.LastError != "agrega el token de Cloudflare en Ajustes" ||
		d.CFRecordID == "" || d.TLSMode != entity.TLSCloudflare {
		t.Fatalf("%+v", d)
	}
	if calls := f.cf.Calls(); len(calls) != 0 {
		t.Fatalf("no debía llamar a Cloudflare: %v", calls)
	}
}

// findZone (retry "sin zona", D23): sin zona no toca d; con zona crea los
// registros como al crear.
func TestFindZone(t *testing.T) {
	f := newDNSFixture(t)
	d := f.domain(t, "api.otro.com", false)
	before := d
	if f.ops.findZone(context.Background(), &d, f.p) || d != before {
		t.Fatalf("sin zona: %+v", d)
	}
	f.cf.AddZone("z2", "otro.com")
	if !f.ops.findZone(context.Background(), &d, f.p) || d.CFZoneID != "z2" || d.TLSMode != entity.TLSCloudflare || d.CFRecordID == "" || d.Status != entity.StatusPending {
		t.Fatalf("con zona: %+v", d)
	}
	f.p.CloudflareToken = ""
	e := f.domain(t, "web.otro.com", false)
	if f.ops.findZone(context.Background(), &e, f.p) {
		t.Fatalf("sin token: %+v", e)
	}
	f.p.CloudflareToken = dnsToken
	f.cf.SetDown(true)
	if f.ops.findZone(context.Background(), &e, f.p) || e.Status != entity.StatusPending {
		t.Fatalf("Cloudflare caído: %+v", e)
	}
}
