package usecases

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/app/dto"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

func (f *ucFixture) retry(t *testing.T) dto.DomainOutput {
	t.Helper()
	out, err := NewRetryDomain(f.deps).Execute(context.Background(), dto.DomainRef{AppRef: apiRef, Hostname: dnsHost})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// set cambia el dominio en la base.
func (f *ucFixture) set(t *testing.T, mod func(*entity.Domain)) entity.Domain {
	t.Helper()
	d := f.get(t, dnsHost)
	mod(&d)
	if err := f.repo.Update(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	return d
}

// cloudflare con el A propio: lo actualiza y crea el AAAA que falta.
func TestRetryCloudflareManaged(t *testing.T) {
	f := newUCFixture(t)
	f.settings(t, func(p *shared.ProxySettings) { p.PublicIPv6 = "" })
	f.create(t, dnsHost, false)
	f.settings(t, func(p *shared.ProxySettings) { p.PublicIP = "203.0.113.99" })
	f.set(t, func(d *entity.Domain) { d.Fail(entity.CodeCloudflare, "antes", f.clock.Now()) })
	f.clock.Advance(time.Minute)
	f.scheduled()
	out := f.retry(t)
	d := f.get(t, dnsHost)
	if out.Status != "pending" || d.CFRecordIDAAAA == "" || !d.UpdatedAt.Equal(f.clock.Now()) {
		t.Fatalf("%+v", d)
	}
	if a := f.recordsOf(dnsHost, "A"); len(a) != 1 || a[0].Content != "203.0.113.99" {
		t.Fatalf("A: %+v", a)
	}
	if got := f.scheduled(); !slices.Equal(got, []string{dnsHost}) {
		t.Fatalf("agendadas: %v", got)
	}
	if got := f.audits(t); got[len(got)-1] != "domain.retry domain:"+dnsHost+" "+apiAudit {
		t.Fatalf("auditoría: %v", got)
	}
}

// record_exists vuelve a mirar: si el ajeno sigue, igual; si ya no está,
// crea los suyos y queda pending.
func TestRetryRecordExists(t *testing.T) {
	f := newUCFixture(t)
	ids, _ := f.foreign(t, service.Record{Type: "CNAME", Content: "viejo.example.com"})
	out := f.retry(t)
	if out.Status != "error" || *out.LastErrorCode != "record_exists" || *out.CFZoneID != "z1" || out.TLSMode != "cloudflare" {
		t.Fatalf("sigue: %+v", out)
	}
	if err := cloudflareDelete(f, ids[0]); err != nil {
		t.Fatal(err)
	}
	out = f.retry(t)
	if out.Status != "pending" || out.CFRecordID == nil || len(f.recordsOf(dnsHost, "A")) != 1 {
		t.Fatalf("ya no está: %+v", out)
	}
}

// pending: verifica ahora; con la sonda inválida sigue igual (retry no
// reinicia el plazo).
func TestRetryPending(t *testing.T) {
	f := newUCFixture(t)
	f.create(t, dnsHost, false)
	before := f.get(t, dnsHost)
	f.clock.Advance(time.Minute)
	if out := f.retry(t); out.Status != "pending" || !out.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("inválido: %+v", out)
	}
	f.probe.set(dnsHost, f.goodCert())
	if out := f.retry(t); out.Status != "active" || out.CertIssuer == nil || *out.CertIssuer != "Let's Encrypt" {
		t.Fatalf("válido: %+v", out)
	}
}

// certificate con la sonda inválida: pending con otros 10 min, y un Load
// forzado para que Caddy reintente la emisión; con la válida, active.
func TestRetryCertificate(t *testing.T) {
	f := newUCFixture(t)
	f.create(t, dnsHost, false)
	f.set(t, func(d *entity.Domain) { d.Fail(entity.CodeCertificate, "vencido", f.clock.Now()) })
	f.clock.Advance(time.Hour)
	loads := len(f.caddy.Loads())
	out := f.retry(t)
	if out.Status != "pending" || !out.UpdatedAt.Equal(f.clock.Now()) {
		t.Fatalf("%+v", out)
	}
	forced := f.caddy.Forced()
	if len(forced) != loads+1 || !forced[len(forced)-1] {
		t.Fatalf("Load forzado: %v", forced)
	}

	f.set(t, func(d *entity.Domain) { d.Fail(entity.CodeCertificate, "vencido", f.clock.Now()) })
	f.probe.set(dnsHost, f.goodCert())
	if out := f.retry(t); out.Status != "active" {
		t.Fatalf("válido: %+v", out)
	}
	if forced := f.caddy.Forced(); forced[len(forced)-1] {
		t.Fatal("sin volver a pending, el Load no se fuerza")
	}
}

// Sin zona (creado sin token): con un token y una zona que ahora sí están,
// crea los registros y pasa a tls cloudflare.
func TestRetryNoZone(t *testing.T) {
	f := newUCFixture(t)
	f.settings(t, func(p *shared.ProxySettings) { p.CloudflareToken = "" })
	f.create(t, dnsHost, false)
	f.settings(t, nil)
	f.scheduled()
	loads := len(f.caddy.Loads())
	out := f.retry(t)
	if out.Status != "pending" || out.TLSMode != "cloudflare" || out.CFZoneID == nil || out.CFRecordID == nil {
		t.Fatalf("%+v", out)
	}
	if len(f.caddy.Loads()) != loads+1 || !strings.Contains(f.lastLoad(t), `"cloudflare"`) {
		t.Fatalf("Load: %d", len(f.caddy.Loads()))
	}
	if got := f.scheduled(); !slices.Equal(got, []string{dnsHost}) {
		t.Fatalf("agendadas: %v", got)
	}
}

// Sin synced, o sin llegar a Caddy, la verificación no cambia nada.
func TestRetryUnknown(t *testing.T) {
	f := newUCFixture(t)
	f.create(t, dnsHost, false)
	f.set(t, func(d *entity.Domain) { d.Fail(entity.CodeCertificate, "vencido", f.clock.Now()) })
	f.clock.Advance(time.Hour)
	f.probe.setErr(fmt.Errorf("%w: x", service.ErrCaddyUnreachable))
	if out := f.retry(t); out.Status != "error" || *out.LastErrorCode != "certificate" {
		t.Fatalf("sin llegar: %+v", out)
	}
	f.probe.setErr(nil)
	f.probe.set(dnsHost, f.goodCert())
	f.state.BeginRecreate()
	if out := f.retry(t); out.Status != "error" {
		t.Fatalf("sin synced: %+v", out)
	}
}

// El plazo de retry (20 s, Timing.DomainRetry) se respeta.
func TestRetryDeadline(t *testing.T) {
	f := newUCFixture(t)
	f.create(t, dnsHost, false)
	f.set(t, func(d *entity.Domain) { d.Fail(entity.CodeCloudflare, "antes", f.clock.Now()) })
	f.deps.DomainRetry = 50 * time.Millisecond
	f.cf.Hang(200 * time.Millisecond)
	start := time.Now()
	out := f.retry(t)
	if el := time.Since(start); el > 180*time.Millisecond {
		t.Fatalf("tardó %s", el)
	}
	if out.Status != "error" || *out.LastErrorCode != "cloudflare" || *out.LastError != msgCloudflareDown {
		t.Fatalf("%+v", out)
	}
}
