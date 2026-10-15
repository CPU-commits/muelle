package usecases

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/ingress/app/dto"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// foreign siembra los registros ajenos y crea dnsHost, que queda en
// record_exists. Devuelve sus ids y cuántas llamadas recibió cftest hasta ahí.
func (f *ucFixture) foreign(t *testing.T, recs ...service.Record) (ids []string, calls int) {
	t.Helper()
	for _, r := range recs {
		r.Name = dnsHost
		ids = append(ids, f.cf.PutRecord("z1", r))
	}
	out := f.create(t, dnsHost, false)
	if out.Status != "error" || out.LastErrorCode == nil || *out.LastErrorCode != "record_exists" || out.CFZoneID == nil {
		t.Fatalf("debía quedar en record_exists: %+v", out)
	}
	f.scheduled()
	return ids, len(f.cf.Calls())
}

func (f *ucFixture) adopt(t *testing.T) dto.DomainOutput {
	t.Helper()
	out, err := NewAdoptDomain(f.deps).Execute(context.Background(), dto.DomainRef{AppRef: apiRef, Hostname: dnsHost})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// adopted revisa lo común a todo lo que adopta o crea: pending, tls
// cloudflare con la zona de record_exists, y sin volver a listar las zonas (D7).
func (f *ucFixture) adopted(t *testing.T, out dto.DomainOutput, from int) entity.Domain {
	t.Helper()
	d := f.get(t, dnsHost)
	if out.Status != "pending" || out.TLSMode != "cloudflare" || out.CFZoneID == nil || *out.CFZoneID != "z1" ||
		d.CFRecordID == "" || out.CFRecordID == nil || *out.CFRecordID != d.CFRecordID {
		t.Fatalf("%+v / %+v", out, d)
	}
	for _, c := range f.cf.Calls()[from:] {
		if strings.HasPrefix(c, "GET /zones?") {
			t.Fatalf("adoptar no vuelve a buscar la zona: %v", f.cf.Calls()[from:])
		}
	}
	return d
}

// Un A ajeno pasa a la IP pública y conserva su proxied (D17), y el AAAA
// que falta se crea con ese proxied.
func TestAdoptA(t *testing.T) {
	f := newUCFixture(t)
	ids, from := f.foreign(t, service.Record{Type: "A", Content: "198.51.100.7", Proxied: true})
	out := f.adopt(t)
	d := f.adopted(t, out, from)
	if d.CFRecordID != ids[0] || !out.Proxied || !d.Proxied {
		t.Fatalf("%+v", d)
	}
	if a := f.recordsOf(dnsHost, "A"); len(a) != 1 || a[0].Content != dnsIPv4 || !a[0].Proxied {
		t.Fatalf("A: %+v", a)
	}
	if aaaa := f.recordsOf(dnsHost, "AAAA"); len(aaaa) != 1 || aaaa[0].ID != d.CFRecordIDAAAA || !aaaa[0].Proxied {
		t.Fatalf("AAAA: %+v", aaaa)
	}
	if got := f.scheduled(); !slices.Equal(got, []string{dnsHost}) {
		t.Fatalf("agendadas: %v", got)
	}
	if got := f.audits(t); got[len(got)-1] != "domain.adopt domain:"+dnsHost+" "+apiAudit {
		t.Fatalf("auditoría: %v", got)
	}
}

// Un CNAME pasa a A: un PATCH con type y content.
func TestAdoptCNAME(t *testing.T) {
	f := newUCFixture(t)
	f.settings(t, func(p *shared.ProxySettings) { p.PublicIPv6 = "" })
	ids, from := f.foreign(t, service.Record{Type: "CNAME", Content: "viejo.example.com"})
	d := f.adopted(t, f.adopt(t), from)
	if d.CFRecordID != ids[0] || d.CFRecordIDAAAA != "" {
		t.Fatalf("%+v", d)
	}
	if a := f.recordsOf(dnsHost, "A"); len(a) != 1 || a[0].ID != ids[0] || a[0].Content != dnsIPv4 {
		t.Fatalf("A: %+v (%v)", a, f.cf.Records("z1"))
	}
}

// Dos A (o un A y un CNAME): no se toca nada, y se pide dejar uno solo.
func TestAdoptTwoA(t *testing.T) {
	f := newUCFixture(t)
	_, from := f.foreign(t, service.Record{Type: "A", Content: "198.51.100.7"}, service.Record{Type: "A", Content: "198.51.100.8"})
	out := f.adopt(t)
	if out.Status != "error" || *out.LastErrorCode != "cloudflare" || !strings.Contains(*out.LastError, "deja uno solo") || out.CFRecordID != nil {
		t.Fatalf("%+v", out)
	}
	for _, c := range f.cf.Calls()[from:] {
		if !strings.HasPrefix(c, "GET ") {
			t.Fatalf("no debía tocar nada: %v", f.cf.Calls()[from:])
		}
	}
}

// Con public_ipv6, el AAAA ajeno se adopta con su proxied; sin public_ipv6,
// no se toca.
func TestAdoptAAAA(t *testing.T) {
	f := newUCFixture(t)
	ids, from := f.foreign(t, service.Record{Type: "A", Content: "198.51.100.7", Proxied: true},
		service.Record{Type: "AAAA", Content: "2001:db8::7"})
	d := f.adopted(t, f.adopt(t), from)
	if d.CFRecordIDAAAA != ids[1] || !d.Proxied {
		t.Fatalf("%+v", d)
	}
	if aaaa := f.recordsOf(dnsHost, "AAAA"); len(aaaa) != 1 || aaaa[0].Content != dnsIPv6 || aaaa[0].Proxied {
		t.Fatalf("AAAA: %+v", aaaa)
	}

	g := newUCFixture(t)
	g.settings(t, func(p *shared.ProxySettings) { p.PublicIPv6 = "" })
	ids, from = g.foreign(t, service.Record{Type: "A", Content: "198.51.100.7"}, service.Record{Type: "AAAA", Content: "2001:db8::7"})
	d = g.adopted(t, g.adopt(t), from)
	if d.CFRecordIDAAAA != "" {
		t.Fatalf("sin IPv6: %+v", d)
	}
	if aaaa := g.recordsOf(dnsHost, "AAAA"); len(aaaa) != 1 || aaaa[0].ID != ids[1] || aaaa[0].Content != "2001:db8::7" {
		t.Fatalf("el AAAA ajeno no se toca: %+v", aaaa)
	}
}

// D32: el único registro ajeno es un AAAA.
func TestAdoptOnlyAAAA(t *testing.T) {
	// Con public_ipv6: se crea el A con su proxied y después se adopta el AAAA.
	f := newUCFixture(t)
	ids, from := f.foreign(t, service.Record{Type: "AAAA", Content: "2001:db8::7", Proxied: true})
	out := f.adopt(t)
	d := f.adopted(t, out, from)
	if d.CFRecordIDAAAA != ids[0] || !d.Proxied || !out.Proxied {
		t.Fatalf("%+v", d)
	}
	calls := f.cf.Calls()[from:]
	post := slices.IndexFunc(calls, func(c string) bool { return strings.HasPrefix(c, "POST ") })
	patch := slices.Index(calls, "PATCH /zones/z1/dns_records/"+ids[0])
	if post < 0 || patch < 0 || post > patch {
		t.Fatalf("el POST del A va antes del PATCH del AAAA: %v", calls)
	}
	if a := f.recordsOf(dnsHost, "A"); len(a) != 1 || !a[0].Proxied || a[0].Content != dnsIPv4 {
		t.Fatalf("A: %+v", a)
	}
	if aaaa := f.recordsOf(dnsHost, "AAAA"); len(aaaa) != 1 || aaaa[0].Content != dnsIPv6 || !aaaa[0].Proxied {
		t.Fatalf("AAAA: %+v", aaaa)
	}

	// Sin public_ipv6: se crea el A y el AAAA queda igual.
	g := newUCFixture(t)
	g.settings(t, func(p *shared.ProxySettings) { p.PublicIPv6 = "" })
	ids, from = g.foreign(t, service.Record{Type: "AAAA", Content: "2001:db8::7", Proxied: true})
	d = g.adopted(t, g.adopt(t), from)
	if d.CFRecordIDAAAA != "" || !d.Proxied {
		t.Fatalf("sin IPv6: %+v", d)
	}
	if aaaa := g.recordsOf(dnsHost, "AAAA"); len(aaaa) != 1 || aaaa[0].ID != ids[0] || aaaa[0].Content != "2001:db8::7" {
		t.Fatalf("AAAA: %+v", aaaa)
	}

	// Si el POST del A falla, el AAAA no se toca y no queda su id (D19).
	h := newUCFixture(t)
	ids, from = h.foreign(t, service.Record{Type: "AAAA", Content: "2001:db8::7", Proxied: true})
	h.cf.FailNext("POST", "dns_records", 400, 9005, "Content for A record is invalid.")
	out = h.adopt(t)
	hd := h.get(t, dnsHost)
	if out.Status != "error" || hd.ErrorCode != entity.CodeCloudflare || hd.CFRecordID != "" || hd.CFRecordIDAAAA != "" {
		t.Fatalf("POST que falla: %+v", hd)
	}
	if slices.Contains(h.cf.Calls()[from:], "PATCH /zones/z1/dns_records/"+ids[0]) {
		t.Fatalf("el AAAA no se toca: %v", h.cf.Calls()[from:])
	}
}

// Más de un AAAA: con public_ipv6 no se toca nada; sin public_ipv6 no
// cuentan, y el A se adopta.
func TestAdoptManyAAAA(t *testing.T) {
	recs := []service.Record{
		{Type: "A", Content: "198.51.100.7"}, {Type: "AAAA", Content: "2001:db8::7"}, {Type: "AAAA", Content: "2001:db8::8"},
	}
	f := newUCFixture(t)
	_, from := f.foreign(t, recs...)
	out := f.adopt(t)
	if out.Status != "error" || !strings.Contains(*out.LastError, "AAAA") || !strings.Contains(*out.LastError, "deja uno solo") {
		t.Fatalf("%+v", out)
	}
	for _, c := range f.cf.Calls()[from:] {
		if !strings.HasPrefix(c, "GET ") {
			t.Fatalf("no debía tocar nada: %v", f.cf.Calls()[from:])
		}
	}

	g := newUCFixture(t)
	g.settings(t, func(p *shared.ProxySettings) { p.PublicIPv6 = "" })
	ids, from := g.foreign(t, recs...)
	if d := g.adopted(t, g.adopt(t), from); d.CFRecordID != ids[0] || d.CFRecordIDAAAA != "" {
		t.Fatalf("sin IPv6: %+v", d)
	}
}

// Sin token o sin public_ip: error + cloudflare con lo que falta, y 200.
func TestAdoptMissingSettings(t *testing.T) {
	for _, c := range []struct {
		mod  func(*shared.ProxySettings)
		want string
	}{
		{func(p *shared.ProxySettings) { p.CloudflareToken = "" }, msgNoToken},
		{func(p *shared.ProxySettings) { p.PublicIP = "" }, msgNoPublicIP},
	} {
		f := newUCFixture(t)
		_, from := f.foreign(t, service.Record{Type: "A", Content: "198.51.100.7"})
		f.settings(t, c.mod)
		out := f.adopt(t)
		if out.Status != "error" || *out.LastErrorCode != "cloudflare" || *out.LastError != c.want || out.CFZoneID == nil {
			t.Fatalf("%+v", out)
		}
		if n := len(f.cf.Calls()); n != from {
			t.Fatalf("no debía llamar a Cloudflare: %v", f.cf.Calls()[from:])
		}
	}
}

// Si el ajeno ya no está, crea los suyos.
func TestAdoptGone(t *testing.T) {
	f := newUCFixture(t)
	ids, from := f.foreign(t, service.Record{Type: "A", Content: "198.51.100.7"})
	if err := cloudflareDelete(f, ids[0]); err != nil {
		t.Fatal(err)
	}
	d := f.adopted(t, f.adopt(t), from)
	if d.CFRecordID == ids[0] || d.CFRecordIDAAAA == "" {
		t.Fatalf("%+v", d)
	}
}

// Solo un record_exists se adopta; un dominio de otra app es 404.
func TestAdoptNothing(t *testing.T) {
	f := newUCFixture(t)
	f.create(t, dnsHost, false)
	uc := NewAdoptDomain(f.deps)
	if _, err := uc.Execute(context.Background(), dto.DomainRef{AppRef: apiRef, Hostname: dnsHost}); !errors.Is(err, service.ErrNothingToAdopt) {
		t.Fatalf("pending: %v", err)
	}
	if _, err := uc.Execute(context.Background(), dto.DomainRef{AppRef: infraRef, Hostname: dnsHost}); !errors.Is(err, sharedrepo.ErrNotFound) {
		t.Fatalf("otra app: %v", err)
	}
}
