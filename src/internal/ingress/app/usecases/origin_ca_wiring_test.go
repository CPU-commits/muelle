package usecases

import (
	"context"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/app/dto"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	reposqlite "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/repository/sqlite"
	outservice "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// kicked dice si hay un Kick pendiente, y lo saca.
func kicked(u *EnsureOriginCA) bool {
	select {
	case <-u.kick:
		return true
	default:
		return false
	}
}

// Crear, editar, adoptar, reintentar y borrar un dominio despiertan al
// reconciliador de Origin CA, después de su Sync (contrato §3.2).
func TestDomainOpsKickOrigin(t *testing.T) {
	f := newUCFixture(t)
	ctx := context.Background()
	o := f.deps.Origin
	f.create(t, dnsHost, false)
	if !kicked(o) {
		t.Fatal("crear")
	}
	if _, err := NewUpdateDomain(f.deps).Execute(ctx, dto.UpdateDomainInput{AppRef: apiRef, Hostname: dnsHost, TargetPort: ptr(4000)}); err != nil || !kicked(o) {
		t.Fatalf("editar: %v", err)
	}
	// {} no cambia nada: no despierta.
	if _, err := NewUpdateDomain(f.deps).Execute(ctx, dto.UpdateDomainInput{AppRef: apiRef, Hostname: dnsHost}); err != nil || kicked(o) {
		t.Fatalf("sin cambios: %v", err)
	}
	f.retry(t)
	if !kicked(o) {
		t.Fatal("reintentar")
	}
	if err := NewDeleteDomain(f.deps).Execute(ctx, dto.DomainRef{AppRef: apiRef, Hostname: dnsHost}); err != nil || !kicked(o) {
		t.Fatalf("borrar: %v", err)
	}
	f.foreign(t, service.Record{Type: "A", Content: "198.51.100.7", Proxied: true})
	kicked(o)
	f.adopt(t)
	if !kicked(o) {
		t.Fatal("adoptar")
	}
}

// Un PATCH que cambia proxied olvida lo que se vio en Cloudflare de ese
// hostname, pero no su falla (contrato §3.7).
func TestUpdateProxiedForgetsObserved(t *testing.T) {
	f := newUCFixture(t)
	ctx := context.Background()
	f.create(t, dnsHost, true)
	if err := f.origin.Hosts.SetObserved(ctx, dnsHost, entity.ObservedProxied, f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	next := f.clock.Now().Add(6 * time.Hour)
	if err := f.origin.Hosts.RecordFailure(ctx, dnsHost, 1, entity.OriginPermission, "sin permiso", next, f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	// Cambiar el destino no lo olvida.
	if _, err := NewUpdateDomain(f.deps).Execute(ctx, dto.UpdateDomainInput{AppRef: apiRef, Hostname: dnsHost, TargetPort: ptr(4000)}); err != nil {
		t.Fatal(err)
	}
	if h, _ := f.origin.Hosts.Get(ctx, dnsHost); h.Observed != entity.ObservedProxied {
		t.Fatalf("destino: %+v", h)
	}
	if _, err := NewUpdateDomain(f.deps).Execute(ctx, dto.UpdateDomainInput{AppRef: apiRef, Hostname: dnsHost, Proxied: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	h, _ := f.origin.Hosts.Get(ctx, dnsHost)
	if h.Observed != "" || h.Failures != 1 || !h.NextAttemptAt.Equal(next) {
		t.Fatalf("proxied: %+v", h)
	}
}

// retry reinicia el backoff de Origin CA de ese dominio (la falla queda) y
// despierta al reconciliador.
func TestRetryResetsOriginBackoff(t *testing.T) {
	f := newUCFixture(t)
	ctx := context.Background()
	f.create(t, dnsHost, true)
	for _, h := range []string{dnsHost, "otro.tudominio.cl"} {
		if err := f.origin.Hosts.RecordFailure(ctx, h, 2, entity.OriginPermission, "sin permiso", f.clock.Now().Add(6*time.Hour), f.clock.Now()); err != nil {
			t.Fatal(err)
		}
	}
	f.clock.Advance(time.Minute)
	kicked(f.deps.Origin)
	f.retry(t)
	if h, _ := f.origin.Hosts.Get(ctx, dnsHost); h.Failures != 2 || !h.NextAttemptAt.Equal(f.clock.Now()) {
		t.Fatalf("reinicio: %+v", h)
	}
	if h, _ := f.origin.Hosts.Get(ctx, "otro.tudominio.cl"); h.NextAttemptAt.Equal(f.clock.Now()) {
		t.Fatalf("reinició otro host: %+v", h)
	}
	if !kicked(f.deps.Origin) {
		t.Fatal("no despertó")
	}
}

// newOriginFor arma un EnsureOriginCA sobre la base del lifecycleFixture.
func newOriginFor(f *lifecycleFixture) (*EnsureOriginCA, OriginRepos) {
	repos := OriginRepos{Certs: reposqlite.NewOriginCerts(f.db, f.cipher), Hosts: reposqlite.NewOriginHosts(f.db)}
	client := cloudflare.New(f.cf.URL())
	return NewEnsureOriginCA(OriginDeps{
		Settings: f.settings, Domains: f.repo, Repos: repos, DNS: client, CA: client, Keys: outservice.OriginKeys{},
		Syncer: f.syncer, Clock: f.clock, DefImage: defCaddyImage, Pass: 5 * time.Minute, Grace: time.Minute, Log: f.slog,
	}), repos
}

// ProxySettingsChanged (contrato §3.5): cloudflare_origin_ca hace Sync,
// reinicia el backoff de todos y despierta al reconciliador; el token con la
// opción encendida también; apagarla olvida lo observado y las fallas (no
// los retirados).
func TestSettingsChangedOriginCA(t *testing.T) {
	f := newHookFixture(t, time.Hour)
	o, repos := newOriginFor(f.lifecycleFixture)
	f.uc = NewProxySettingsChanged(f.recreator, f.uc.records, f.panel, f.syncer, o, f.state, 2*time.Minute, f.slog)
	f.withToken(t)
	ctx := context.Background()
	if err := repos.Hosts.RecordFailure(ctx, "api.auditfy.cl", 1, entity.OriginPermission, "sin permiso", f.clock.Now().Add(6*time.Hour), f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	loads := len(f.caddy.Loads())
	old, next := f.change(t, func(p *shared.ProxySettings) { p.CloudflareOriginCA = true })
	f.uc.Execute(ctx, old, next)
	if len(f.caddy.Loads()) != loads+1 || !kicked(o) {
		t.Fatalf("encender: loads %d → %d", loads, len(f.caddy.Loads()))
	}
	if h, _ := repos.Hosts.Get(ctx, "api.auditfy.cl"); !h.NextAttemptAt.Equal(f.clock.Now()) || h.Failures != 1 {
		t.Fatalf("backoff: %+v", h)
	}

	// Otro token con la opción encendida: reinicia y despierta.
	f.clock.Advance(time.Minute)
	f.cf.AddToken("cf-token-otro-1234567890abcdef")
	old, next = f.change(t, func(p *shared.ProxySettings) { p.CloudflareToken = "cf-token-otro-1234567890abcdef" })
	f.uc.Execute(ctx, old, next)
	if !kicked(o) {
		t.Fatal("token con la opción encendida")
	}
	if h, _ := repos.Hosts.Get(ctx, "api.auditfy.cl"); !h.NextAttemptAt.Equal(f.clock.Now()) {
		t.Fatalf("backoff con el token: %+v", h)
	}
	// Un cambio de otra cosa no despierta.
	old, next = f.change(t, func(p *shared.ProxySettings) { p.ACMEEmail = "otro@auditfy.cl" })
	f.uc.Execute(ctx, old, next)
	if kicked(o) {
		t.Fatal("el email no despierta")
	}

	// Apagarla: Sync, Forget (lo observado y las fallas) y Kick; los
	// retirados quedan.
	retired := entity.OriginCert{Hostname: "viejo.auditfy.cl", ZoneID: "z1", CFCertID: "77", CertPEM: "PEM",
		ExpiresAt: f.clock.Now().Add(80 * 24 * time.Hour), CreatedAt: f.clock.Now()}
	if err := repos.Certs.InsertRetired(ctx, &retired, f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	loads = len(f.caddy.Loads())
	old, next = f.change(t, func(p *shared.ProxySettings) { p.CloudflareOriginCA = false })
	f.uc.Execute(ctx, old, next)
	if len(f.caddy.Loads()) != loads+1 || !kicked(o) {
		t.Fatalf("apagar: loads %d → %d", loads, len(f.caddy.Loads()))
	}
	if all, _ := repos.Hosts.All(ctx); len(all) != 0 {
		t.Fatalf("no olvidó: %v", all)
	}
	if r, _ := repos.Certs.Retired(ctx); len(r) != 1 {
		t.Fatalf("los retirados: %+v", r)
	}
	// Con la opción apagada, otro token no despierta.
	old, next = f.change(t, func(p *shared.ProxySettings) { p.CloudflareToken = syncToken })
	f.uc.Execute(ctx, old, next)
	if kicked(o) {
		t.Fatal("token con la opción apagada")
	}
}

// El job "proxy", con el proxy habilitado, despierta al final al
// reconciliador (también con la opción apagada: revoca lo que quedó
// retirado).
func TestProxyJobKicksOrigin(t *testing.T) {
	f := newLifecycleFixture(t)
	o, _ := newOriginFor(f)
	job := newProxyJob(f)
	job.origin = o
	if err := job.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if kicked(o) {
		t.Fatal("deshabilitado no despierta")
	}
	f.enableCaddy(t, nil)
	if err := job.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !kicked(o) {
		t.Fatal("no despertó")
	}
}
