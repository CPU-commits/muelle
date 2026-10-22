package usecases

import (
	"context"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/app/dto"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// listOne es el único dominio de GET {app}/domains.
func (f *ucFixture) listOne(t *testing.T) dto.DomainOutput {
	t.Helper()
	out, err := NewListDomains(f.deps).Execute(context.Background(), apiRef)
	if err != nil || len(out) != 1 {
		t.Fatalf("ListDomains: %+v %v", out, err)
	}
	return out[0]
}

// seedServed deja dnsHost con un active de 90 días y visto con el proxy.
func (f *ucFixture) seedServed(t *testing.T) entity.OriginCert {
	t.Helper()
	ctx := context.Background()
	certPEM, _ := testLeaf(t, dnsHost, f.clock.Now().Add(90*24*time.Hour))
	c := entity.OriginCert{Hostname: dnsHost, ZoneID: "z1", CFCertID: "100", CertPEM: certPEM, KeyPEM: "KEY",
		ExpiresAt: f.clock.Now().Add(90 * 24 * time.Hour), CreatedAt: f.clock.Now()}
	if err := f.origin.Certs.Insert(ctx, &c); err != nil {
		t.Fatal(err)
	}
	if err := f.origin.Hosts.SetObserved(ctx, dnsHost, entity.ObservedProxied, f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	return c
}

// cert_source y origin_ca en el dominio (contrato §3.1 y §3.7), con la misma
// regla que la configuración de Caddy.
func TestDomainOutputOriginCA(t *testing.T) {
	f := newUCFixture(t)
	ctx := context.Background()
	f.create(t, dnsHost, true)
	// La opción apagada: null y letsencrypt.
	if out := f.listOne(t); out.OriginCA != nil || out.CertSource != "letsencrypt" {
		t.Fatalf("apagada: %+v %s", out.OriginCA, out.CertSource)
	}
	f.settings(t, func(p *shared.ProxySettings) { p.CloudflareOriginCA = true })
	// Todavía no se miró Cloudflare: issuing.
	if out := f.listOne(t); out.OriginCA == nil || out.OriginCA.Status != "issuing" || out.CertSource != "letsencrypt" ||
		out.OriginCA.ErrorCode != nil || out.OriginCA.Error != nil || out.OriginCA.RetryAt != nil || out.OriginCA.ExpiresAt != nil {
		t.Fatalf("issuing: %+v", out.OriginCA)
	}
	c := f.seedServed(t)
	out := f.listOne(t)
	if out.OriginCA == nil || out.OriginCA.Status != "active" || out.CertSource != "cloudflare_origin_ca" ||
		out.OriginCA.ExpiresAt == nil || !out.OriginCA.ExpiresAt.Equal(c.ExpiresAt) || out.OriginCA.ExpiresAt.Location() != time.UTC {
		t.Fatalf("active: %+v %s", out.OriginCA, out.CertSource)
	}
	// Una renovación que falló: active con la falla y el mensaje de renovación.
	next := f.clock.Now().Add(time.Minute)
	if err := f.origin.Hosts.RecordFailure(ctx, dnsHost, 1, entity.OriginCloudflare, "Cloudflare no respondió (HTTP 503); se reintenta solo", next, f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	out = f.listOne(t)
	if out.OriginCA.Status != "active" || out.OriginCA.ErrorCode == nil || *out.OriginCA.ErrorCode != "cloudflare" ||
		out.OriginCA.RetryAt == nil || !out.OriginCA.RetryAt.Equal(next) || out.OriginCA.Error == nil ||
		*out.OriginCA.Error != "muelle no pudo renovar el certificado de Cloudflare, que vence el 12 ene 2027: Cloudflare no respondió (HTTP 503); se reintenta solo; si no lo renueva antes del 29 dic 2026, vuelve a Let's Encrypt" {
		t.Fatalf("renovación: %+v %v", out.OriginCA, *out.OriginCA.Error)
	}
	// Las demás operaciones también lo traen (Update, con su hostname).
	upd, err := NewUpdateDomain(f.deps).Execute(ctx, dto.UpdateDomainInput{AppRef: apiRef, Hostname: dnsHost, TargetPort: ptr(4000)})
	if err != nil || upd.OriginCA == nil || upd.OriginCA.Status != "active" || upd.CertSource != "cloudflare_origin_ca" {
		t.Fatalf("update: %+v %v", upd.OriginCA, err)
	}
	// proxied: false en muelle: dns_only desde la respuesta.
	upd, err = NewUpdateDomain(f.deps).Execute(ctx, dto.UpdateDomainInput{AppRef: apiRef, Hostname: dnsHost, Proxied: ptr(false)})
	if err != nil || upd.OriginCA == nil || upd.OriginCA.Status != "dns_only" || upd.CertSource != "letsencrypt" ||
		upd.OriginCA.Error != nil || upd.OriginCA.ExpiresAt == nil {
		t.Fatalf("dns_only: %+v %v", upd.OriginCA, err)
	}
	// Sin token, o con el proxy deshabilitado: null.
	f.settings(t, func(p *shared.ProxySettings) { p.CloudflareOriginCA, p.PanelDomain = true, "" })
	if out := f.listOne(t); out.OriginCA != nil || out.CertSource != "letsencrypt" {
		t.Fatalf("deshabilitado: %+v", out.OriginCA)
	}
	f.settings(t, func(p *shared.ProxySettings) { p.CloudflareOriginCA, p.CloudflareToken = true, "" })
	if out := f.listOne(t); out.OriginCA != nil {
		t.Fatalf("sin token: %+v", out.OriginCA)
	}
}

// Un dominio fuera de Cloudflare: not_eligible.
func TestDomainOutputOriginCANotEligible(t *testing.T) {
	f := newUCFixture(t)
	f.settings(t, func(p *shared.ProxySettings) { p.CloudflareOriginCA = true })
	f.put(t, func(d *entity.Domain) { d.TLSMode = entity.TLSHTTP })
	if out := f.listOne(t); out.OriginCA == nil || out.OriginCA.Status != "not_eligible" || out.CertSource != "letsencrypt" {
		t.Fatalf("%+v", out.OriginCA)
	}
}

// retry responde con origin_ca después de reiniciar el backoff: failed,
// con retry_at en la hora del pedido.
func TestRetryOutputOriginCA(t *testing.T) {
	f := newUCFixture(t)
	ctx := context.Background()
	f.settings(t, func(p *shared.ProxySettings) { p.CloudflareOriginCA = true })
	f.create(t, dnsHost, true)
	if err := f.origin.Hosts.RecordFailure(ctx, dnsHost, 1, entity.OriginPermission, "sin permiso", f.clock.Now().Add(6*time.Hour), f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	out := f.retry(t)
	if out.OriginCA == nil || out.OriginCA.Status != "failed" || out.OriginCA.RetryAt == nil || !out.OriginCA.RetryAt.Equal(f.clock.Now()) ||
		*out.OriginCA.Error != "muelle no pudo emitir el certificado de Cloudflare: sin permiso; mientras tanto usa Let's Encrypt" {
		t.Fatalf("%+v", out.OriginCA)
	}
}
