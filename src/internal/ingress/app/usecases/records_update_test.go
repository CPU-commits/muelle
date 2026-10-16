package usecases

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

const (
	hostA    = "a.tudominio.cl"
	hostB    = "b.tudominio.cl"
	hostNone = "sin.tudominio.cl"
	newIPv4  = "203.0.113.50"
	newIPv6  = "2001:db8::50"
)

// putHost guarda un dominio de la app api con ese hostname, tal cual.
func (f *ucFixture) putHost(t *testing.T, host string, mod func(*entity.Domain)) entity.Domain {
	t.Helper()
	d, err := entity.NewDomain(f.apiID, host, "api", 3000, false, f.clock.Now())
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

// putManaged guarda host con su A propio en z1 (con oldIPv4) y, si aaaa, su
// AAAA (con oldIPv6), pending y con tls cloudflare.
func (f *ucFixture) putManaged(t *testing.T, host string, aaaa bool, mod func(*entity.Domain)) entity.Domain {
	t.Helper()
	return f.putHost(t, host, func(d *entity.Domain) {
		d.CFZoneID, d.TLSMode = "z1", entity.TLSCloudflare
		d.CFRecordID = f.cf.PutRecord("z1", service.Record{Type: typeA, Name: host, Content: oldIPv4})
		if aaaa {
			d.CFRecordIDAAAA = f.cf.PutRecord("z1", service.Record{Type: typeAAAA, Name: host, Content: oldIPv6})
		}
		if mod != nil {
			mod(d)
		}
	})
}

// ipChange guarda los ajustes con mod (como admin antes de llamar al hook) y
// devuelve los de antes y los nuevos.
func (f *ucFixture) ipChange(t *testing.T, mod func(*shared.ProxySettings)) (old, next shared.ProxySettings) {
	t.Helper()
	ctx := context.Background()
	old, err := f.deps.Settings.ProxySettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	next = old
	next.PanelAllowedCIDRs = []string{}
	mod(&next)
	f.settings(t, func(p *shared.ProxySettings) { *p = next })
	return old, next
}

// v4Only deja los ajustes con oldIPv4 y sin IPv6, el punto de partida de
// los tests que cambian solo la IPv4.
func (f *ucFixture) v4Only(t *testing.T) {
	t.Helper()
	f.settings(t, func(p *shared.ProxySettings) { p.PublicIP, p.PublicIPv6 = oldIPv4, "" })
}

func newRecordsUpdate(f *ucFixture) *UpdateManagedRecords { return NewUpdateManagedRecords(f.deps) }

// Un public_ip nuevo cambia el A de los dominios con registros propios; el
// que estaba en error + cloudflare vuelve a pending. El que no tiene
// registros no se toca.
func TestRecordsUpdateIPv4(t *testing.T) {
	f := newUCFixture(t)
	f.v4Only(t)
	a := f.putManaged(t, hostA, false, nil)
	b := f.putManaged(t, hostB, false, func(d *entity.Domain) { d.Fail(entity.CodeCloudflare, msgCloudflareDown, f.clock.Now()) })
	none := f.putHost(t, hostNone, nil)
	f.clock.Advance(time.Minute)
	old, next := f.ipChange(t, func(p *shared.ProxySettings) { p.PublicIP = newIPv4 })
	if err := newRecordsUpdate(f).Execute(context.Background(), old, next); err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{hostA, hostB} {
		if r := f.one(t, h, typeA); r.Content != newIPv4 || r.Proxied {
			t.Fatalf("A de %s: %+v", h, r)
		}
	}
	if got := f.get(t, hostA); got.Status != entity.StatusPending || got.UpdatedAt != a.UpdatedAt {
		t.Fatalf("%s no cambia de estado: %+v", hostA, got)
	}
	if got := f.get(t, hostB); got.Status != entity.StatusPending || got.ErrorCode != "" || got.CFRecordID != b.CFRecordID {
		t.Fatalf("%s vuelve a pending: %+v", hostB, got)
	}
	if got := f.get(t, hostNone); got != none {
		t.Fatalf("%s no se toca: %+v", hostNone, got)
	}
	for _, c := range f.cf.Calls() {
		if strings.Contains(c, hostNone) {
			t.Fatalf("no se llama a Cloudflare por %s: %v", hostNone, f.cf.Calls())
		}
	}
}

// Uno que falla queda en error + cloudflare, y los demás se actualizan igual.
func TestRecordsUpdateOneFails(t *testing.T) {
	f := newUCFixture(t)
	f.v4Only(t)
	a := f.putManaged(t, hostA, false, nil)
	f.putManaged(t, hostB, false, nil)
	f.cf.FailNext("PATCH", a.CFRecordID, 400, 9005, "Content for A record is invalid")
	old, next := f.ipChange(t, func(p *shared.ProxySettings) { p.PublicIP = newIPv4 })
	if err := newRecordsUpdate(f).Execute(context.Background(), old, next); err != nil {
		t.Fatal(err)
	}
	if got := f.get(t, hostA); got.Status != entity.StatusError || got.ErrorCode != entity.CodeCloudflare ||
		!strings.Contains(got.LastError, "Content for A record is invalid") || got.CFRecordID != a.CFRecordID {
		t.Fatalf("%s: %+v", hostA, got)
	}
	if r := f.one(t, hostB, typeA); r.Content != newIPv4 {
		t.Fatalf("A de %s: %+v", hostB, r)
	}
}

// Un A propio que ya no está (404) limpia el id y deja error + cloudflare,
// así retry lo crea. Con su AAAA propio, el AAAA se borra y los dos ids
// quedan vacíos (D29): el CHECK de 0003 no deja un AAAA sin A.
func TestRecordsUpdateAGone(t *testing.T) {
	for _, aaaa := range []bool{false, true} {
		name := map[bool]string{false: "solo A", true: "con AAAA"}[aaaa]
		t.Run(name, func(t *testing.T) {
			f := newUCFixture(t)
			if !aaaa {
				f.v4Only(t)
			}
			d := f.putManaged(t, hostA, aaaa, nil)
			f.cf.FailNext("PATCH", d.CFRecordID, 404, 81044, "Record does not exist.")
			old, next := f.ipChange(t, func(p *shared.ProxySettings) { p.PublicIP = newIPv4 })
			if err := newRecordsUpdate(f).Execute(context.Background(), old, next); err != nil {
				t.Fatal(err)
			}
			got := f.get(t, hostA)
			if got.Status != entity.StatusError || got.ErrorCode != entity.CodeCloudflare || !strings.Contains(got.LastError, "ya no está") ||
				got.CFRecordID != "" || got.CFRecordIDAAAA != "" || got.CFZoneID != "z1" {
				t.Fatalf("%+v", got)
			}
			if aaaa && len(f.recordsOf(hostA, typeAAAA)) != 0 {
				t.Fatalf("el AAAA sigue: %+v", f.cf.Records("z1"))
			}
		})
	}
}

// Borrar public_ip no toca ningún registro (contrato §3.5).
func TestRecordsUpdateIPv4Removed(t *testing.T) {
	f := newUCFixture(t)
	f.putManaged(t, hostA, true, nil)
	before := f.get(t, hostA)
	old, next := f.ipChange(t, func(p *shared.ProxySettings) { p.PublicIP = "" })
	if err := newRecordsUpdate(f).Execute(context.Background(), old, next); err != nil {
		t.Fatal(err)
	}
	if calls := f.cf.Calls(); len(calls) != 0 {
		t.Fatalf("no llama a Cloudflare: %v", calls)
	}
	if got := f.get(t, hostA); got != before {
		t.Fatalf("el dominio no cambia: %+v", got)
	}
}

// D30: los registros van a los ajustes guardados al correr, no al next que
// recibió el hook (otro PATCH llegó entretanto).
func TestRecordsUpdateReadsCurrent(t *testing.T) {
	f := newUCFixture(t)
	f.v4Only(t)
	f.putManaged(t, hostA, false, nil)
	f.putManaged(t, hostB, false, nil)
	old, next := f.ipChange(t, func(p *shared.ProxySettings) { p.PublicIP = newIPv4 })
	f.settings(t, func(p *shared.ProxySettings) { p.PublicIP, p.PublicIPv6 = "203.0.113.60", "" })
	if err := newRecordsUpdate(f).Execute(context.Background(), old, next); err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{hostA, hostB} {
		if r := f.one(t, h, typeA); r.Content != "203.0.113.60" {
			t.Fatalf("A de %s: %+v", h, r)
		}
	}
}

// Cada dominio va en su Ops.Do, con DomainOp: con Cloudflare colgado, cada
// uno queda en error + cloudflare. Con el plazo del hook, el segundo no
// espera otro DomainOp entero (D6).
func TestRecordsUpdateDeadline(t *testing.T) {
	setup := func(t *testing.T) (*ucFixture, *UpdateManagedRecords, shared.ProxySettings, shared.ProxySettings) {
		f := newUCFixture(t)
		f.v4Only(t)
		f.putManaged(t, hostA, false, nil)
		f.putManaged(t, hostB, false, nil)
		old, next := f.ipChange(t, func(p *shared.ProxySettings) { p.PublicIP = newIPv4 })
		f.cf.Hang(400 * time.Millisecond)
		d := f.deps
		d.DomainOp = 100 * time.Millisecond
		return f, NewUpdateManagedRecords(d), old, next
	}
	check := func(t *testing.T, f *ucFixture) {
		t.Helper()
		for _, h := range []string{hostA, hostB} {
			if got := f.get(t, h); got.Status != entity.StatusError || got.ErrorCode != entity.CodeCloudflare {
				t.Fatalf("%s: %+v", h, got)
			}
		}
	}
	t.Run("sin plazo", func(t *testing.T) {
		f, uc, old, next := setup(t)
		start := time.Now()
		if err := uc.Execute(context.Background(), old, next); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d < 200*time.Millisecond || d > 350*time.Millisecond {
			t.Fatalf("volvió en %s", d)
		}
		check(t, f)
	})
	t.Run("plazo del hook", func(t *testing.T) {
		f, uc, old, next := setup(t)
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
		defer cancel()
		start := time.Now()
		if err := uc.Execute(ctx, old, next); err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d > 170*time.Millisecond {
			t.Fatalf("volvió en %s: el segundo esperó otro DomainOp", d)
		}
		check(t, f)
	})
}

// Un public_ipv6 nuevo crea el AAAA (con el proxied del dominio) de los que
// tienen A propio; si ya hay un AAAA ajeno, error + cloudflare.
func TestRecordsUpdateIPv6Added(t *testing.T) {
	f := newUCFixture(t)
	f.v4Only(t)
	f.putManaged(t, hostA, false, func(d *entity.Domain) { d.Proxied = true })
	f.putManaged(t, hostB, false, nil)
	f.cf.PutRecord("z1", service.Record{Type: typeAAAA, Name: hostB, Content: "2001:db8::99"})
	f.putHost(t, hostNone, nil)
	old, next := f.ipChange(t, func(p *shared.ProxySettings) { p.PublicIPv6 = newIPv6 })
	if err := newRecordsUpdate(f).Execute(context.Background(), old, next); err != nil {
		t.Fatal(err)
	}
	aaaa := f.one(t, hostA, typeAAAA)
	if aaaa.Content != newIPv6 || !aaaa.Proxied {
		t.Fatalf("AAAA de %s: %+v", hostA, aaaa)
	}
	if got := f.get(t, hostA); got.CFRecordIDAAAA != aaaa.ID || got.Status != entity.StatusPending {
		t.Fatalf("%s: %+v", hostA, got)
	}
	if got := f.get(t, hostB); got.Status != entity.StatusError || got.ErrorCode != entity.CodeCloudflare ||
		!strings.Contains(got.LastError, "AAAA") || got.CFRecordIDAAAA != "" {
		t.Fatalf("%s: %+v", hostB, got)
	}
	if len(f.recordsOf(hostNone, typeAAAA)) != 0 {
		t.Fatal("sin A propio no se crea el AAAA")
	}
}

// Borrar public_ipv6 borra los AAAA propios y limpia los ids.
func TestRecordsUpdateIPv6Removed(t *testing.T) {
	f := newUCFixture(t)
	f.settings(t, func(p *shared.ProxySettings) { p.PublicIP, p.PublicIPv6 = oldIPv4, oldIPv6 })
	f.putManaged(t, hostA, true, nil)
	old, next := f.ipChange(t, func(p *shared.ProxySettings) { p.PublicIPv6 = "" })
	if err := newRecordsUpdate(f).Execute(context.Background(), old, next); err != nil {
		t.Fatal(err)
	}
	if recs := f.recordsOf(hostA, typeAAAA); len(recs) != 0 {
		t.Fatalf("el AAAA sigue: %+v", recs)
	}
	if got := f.get(t, hostA); got.CFRecordIDAAAA != "" || got.CFRecordID == "" || got.Status != entity.StatusPending {
		t.Fatalf("%+v", got)
	}
}

// Sin token, los dominios con registros propios quedan en error +
// cloudflare con el mensaje del token (D18), sin llamar a Cloudflare.
func TestRecordsUpdateNoToken(t *testing.T) {
	f := newUCFixture(t)
	f.v4Only(t)
	f.putManaged(t, hostA, false, nil)
	f.putHost(t, hostNone, nil)
	old, next := f.ipChange(t, func(p *shared.ProxySettings) { p.PublicIP, p.CloudflareToken = newIPv4, "" })
	if err := newRecordsUpdate(f).Execute(context.Background(), old, next); err != nil {
		t.Fatal(err)
	}
	if calls := f.cf.Calls(); len(calls) != 0 {
		t.Fatalf("no llama a Cloudflare: %v", calls)
	}
	if got := f.get(t, hostA); got.Status != entity.StatusError || got.ErrorCode != entity.CodeCloudflare || got.LastError != msgNoToken {
		t.Fatalf("%s: %+v", hostA, got)
	}
	if got := f.get(t, hostNone); got.Status != entity.StatusPending {
		t.Fatalf("%s: %+v", hostNone, got)
	}
}

// Sin cambios de IP no hace nada.
func TestRecordsUpdateNothingChanged(t *testing.T) {
	f := newUCFixture(t)
	f.putManaged(t, hostA, true, nil)
	old, next := f.ipChange(t, func(p *shared.ProxySettings) { p.ACMEEmail = "otro@correo.cl" })
	if err := newRecordsUpdate(f).Execute(context.Background(), old, next); err != nil {
		t.Fatal(err)
	}
	if calls := f.cf.Calls(); len(calls) != 0 {
		t.Fatalf("no llama a Cloudflare: %v", calls)
	}
}
