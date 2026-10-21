package usecases

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/app/dto"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

const apiAudit = `{"app":"auditfy/staging/api"}`

// fieldErr dice si err es un *validate.Error en field con un mensaje que contiene msg.
func fieldErr(err error, field, msg string) bool {
	ve, ok := errors.AsType[*validate.Error](err)
	return ok && ve.Field == field && strings.Contains(ve.Message, msg)
}

func TestCreateDomain(t *testing.T) {
	f := newUCFixture(t)
	loads := len(f.caddy.Loads())
	out := f.create(t, dnsHost, true)
	if out.Status != "pending" || out.TLSMode != "cloudflare" || !out.Proxied || out.CFZoneID == nil || *out.CFZoneID != "z1" ||
		out.CFRecordID == nil || out.LastError != nil || out.LastErrorCode != nil || out.CertIssuer != nil || out.CertExpiresAt != nil ||
		out.TargetService != "api" || out.TargetPort != 3000 || !out.CreatedAt.Equal(f.clock.Now()) || !out.UpdatedAt.Equal(f.clock.Now()) {
		t.Fatalf("%+v", out)
	}
	want := []dto.RecordOutput{{Type: "A", Name: dnsHost, Content: dnsIPv4}, {Type: "AAAA", Name: dnsHost, Content: dnsIPv6}}
	if !slices.Equal(out.Records, want) {
		t.Fatalf("records: %+v", out.Records)
	}
	if got := f.get(t, dnsHost); got.CFRecordID != *out.CFRecordID || got.CFRecordIDAAAA == "" {
		t.Fatalf("en la base: %+v", got)
	}
	if len(f.caddy.Loads()) != loads+1 || !strings.Contains(f.lastLoad(t), dnsHost) || !strings.Contains(f.lastLoad(t), "auditfy_staging_api_api:3000") {
		t.Fatalf("Load: %s", f.lastLoad(t))
	}
	if got := f.scheduled(); !slices.Equal(got, []string{dnsHost}) {
		t.Fatalf("agendadas: %v", got)
	}
	if got := f.audits(t); !slices.Equal(got, []string{"domain.create domain:" + dnsHost + " " + apiAudit}) {
		t.Fatalf("auditoría: %v", got)
	}
}

// Sin token: http, sin zona y proxied false aunque se pidió (contrato §3.1).
func TestCreateDomainNoToken(t *testing.T) {
	f := newUCFixture(t)
	f.settings(t, func(p *shared.ProxySettings) { p.CloudflareToken = "" })
	out := f.create(t, dnsHost, true)
	if out.Status != "pending" || out.TLSMode != "http" || out.Proxied || out.CFZoneID != nil || out.CFRecordID != nil || len(out.Records) != 2 {
		t.Fatalf("%+v", out)
	}
	if calls := f.cf.Calls(); len(calls) != 0 {
		t.Fatalf("Cloudflare: %v", calls)
	}
}

func TestCreateDomainErrors(t *testing.T) {
	f := newUCFixture(t)
	ctx := context.Background()
	uc := NewCreateDomain(f.deps)
	in := func(ref dto.AppRef, host, svc string) dto.CreateDomainInput {
		return dto.CreateDomainInput{AppRef: ref, Hostname: host, TargetService: svc, TargetPort: 3000}
	}
	if _, err := uc.Execute(ctx, in(apiRef, "panel.tudominio.cl", "api")); !fieldErr(err, "hostname", "es el dominio del panel") {
		t.Errorf("el dominio del panel: %v", err)
	}
	if _, err := uc.Execute(ctx, in(apiRef, "API.tudominio.cl", "api")); !fieldErr(err, "hostname", "") {
		t.Errorf("hostname inválido: %v", err)
	}
	if _, err := uc.Execute(ctx, in(apiRef, dnsHost, "web")); !fieldErr(err, "target_service", "api") {
		t.Errorf("app de formulario: %v", err)
	}
	if _, err := uc.Execute(ctx, dto.CreateDomainInput{AppRef: apiRef, Hostname: dnsHost, TargetService: "api"}); !fieldErr(err, "target_port", "") {
		t.Errorf("sin puerto: %v", err)
	}
	if _, err := uc.Execute(ctx, in(dto.AppRef{Project: "auditfy", Stage: "staging", App: "nada"}, dnsHost, "nada")); !errors.Is(err, sharedrepo.ErrNotFound) {
		t.Errorf("app que no existe: %v", err)
	}
	if len(f.writes()) != 0 || len(f.audits(t)) != 0 {
		t.Fatalf("no debía tocar nada: %v %v", f.writes(), f.audits(t))
	}
	// En la compose, cualquier slug válido pasa.
	if _, err := uc.Execute(ctx, in(infraRef, "redis.tudominio.cl", "redis")); err != nil {
		t.Fatalf("compose: %v", err)
	}
	// Un hostname que ya usa otra app: 409, y no deja registros (D5).
	before := len(f.cf.Records("z1"))
	if _, err := uc.Execute(ctx, in(apiRef, "redis.tudominio.cl", "api")); !errors.Is(err, service.ErrHostnameTaken) {
		t.Fatalf("tomado: %v", err)
	}
	if n := len(f.cf.Records("z1")); n != before {
		t.Fatalf("registros: %d, antes %d", n, before)
	}
}

// El cliente se va: igual crea los registros y guarda, y el Sync llega a
// Caddy (D6).
func TestCreateDomainClientGone(t *testing.T) {
	f := newUCFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	loads := len(f.caddy.Loads())
	out, err := NewCreateDomain(f.deps).Execute(ctx, dto.CreateDomainInput{AppRef: apiRef, Hostname: dnsHost, TargetService: "api", TargetPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	if out.CFRecordID == nil || f.get(t, dnsHost).CFRecordID == "" {
		t.Fatalf("%+v", out)
	}
	if len(f.caddy.Loads()) != loads+1 || !f.state.Synced() {
		t.Fatalf("Loads %d, synced %v", len(f.caddy.Loads()), f.state.Synced())
	}
}

// Con Cloudflare colgado, el plazo de la operación corta y el dominio queda
// guardado en error + cloudflare.
func TestCreateDomainDeadline(t *testing.T) {
	f := newUCFixture(t)
	f.deps.DomainOp = 50 * time.Millisecond
	f.cf.Hang(200 * time.Millisecond)
	start := time.Now()
	out, err := NewCreateDomain(f.deps).Execute(context.Background(), dto.CreateDomainInput{AppRef: apiRef, Hostname: dnsHost, TargetService: "api", TargetPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el > 180*time.Millisecond {
		t.Fatalf("tardó %s", el)
	}
	if got := f.get(t, dnsHost); out.Status != "error" || got.Status != entity.StatusError || got.ErrorCode != entity.CodeCloudflare {
		t.Fatalf("%+v / %+v", out, got)
	}
}

func TestListDomains(t *testing.T) {
	f := newUCFixture(t)
	ctx := context.Background()
	uc := NewListDomains(f.deps)
	webRef := dto.AppRef{Project: "auditfy", Stage: "staging", App: "web"}
	ingresstest.App(t, f.db, "auditfy", "staging", "web", "service")
	if got, err := uc.Execute(ctx, webRef); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("sin dominios: %v %v", got, err)
	}
	f.create(t, "b.tudominio.cl", false)
	f.create(t, "a.tudominio.cl", false)
	if _, err := NewCreateDomain(f.deps).Execute(ctx, dto.CreateDomainInput{AppRef: infraRef, Hostname: "c.tudominio.cl", TargetService: "redis", TargetPort: 6379}); err != nil {
		t.Fatal(err)
	}
	got, err := uc.Execute(ctx, apiRef)
	if err != nil || len(got) != 2 || got[0].Hostname != "a.tudominio.cl" || got[1].Hostname != "b.tudominio.cl" {
		t.Fatalf("%+v %v", got, err)
	}
	// records sale de los ajustes actuales.
	f.settings(t, func(p *shared.ProxySettings) { p.PublicIP, p.PublicIPv6 = "203.0.113.99", "" })
	got, _ = uc.Execute(ctx, apiRef)
	if !slices.Equal(got[0].Records, []dto.RecordOutput{{Type: "A", Name: "a.tudominio.cl", Content: "203.0.113.99"}}) {
		t.Fatalf("records: %+v", got[0].Records)
	}
	if _, err := uc.Execute(ctx, dto.AppRef{Project: "auditfy", Stage: "staging", App: "nada"}); !errors.Is(err, sharedrepo.ErrNotFound) {
		t.Fatalf("app que no existe: %v", err)
	}
}

func TestUpdateDomain(t *testing.T) {
	f := newUCFixture(t)
	ctx := context.Background()
	uc := NewUpdateDomain(f.deps)
	f.settings(t, func(p *shared.ProxySettings) { p.CloudflareToken = "" })
	created := f.create(t, dnsHost, false)
	audits := len(f.audits(t))
	f.clock.Advance(time.Minute)

	// {} no cambia nada.
	out, err := uc.Execute(ctx, dto.UpdateDomainInput{AppRef: apiRef, Hostname: dnsHost})
	if err != nil || !out.UpdatedAt.Equal(created.UpdatedAt) || len(f.audits(t)) != audits {
		t.Fatalf("{}: %+v %v", out, err)
	}
	// Un destino nuevo: guarda, Load con el upstream nuevo y auditoría.
	out, err = uc.Execute(ctx, dto.UpdateDomainInput{AppRef: apiRef, Hostname: dnsHost, TargetPort: ptr(4000)})
	if err != nil || out.TargetPort != 4000 || !out.UpdatedAt.Equal(f.clock.Now()) || f.get(t, dnsHost).TargetPort != 4000 {
		t.Fatalf("puerto: %+v %v", out, err)
	}
	if !strings.Contains(f.lastLoad(t), "auditfy_staging_api_api:4000") {
		t.Fatalf("Load: %s", f.lastLoad(t))
	}
	if got := f.audits(t); got[len(got)-1] != "domain.update domain:"+dnsHost+" "+apiAudit {
		t.Fatalf("auditoría: %v", got)
	}
	// En la app de formulario, el servicio es el de la app (también en el PATCH).
	audits = len(f.audits(t))
	if _, err := uc.Execute(ctx, dto.UpdateDomainInput{AppRef: apiRef, Hostname: dnsHost, TargetService: ptr("otro")}); !fieldErr(err, "target_service", "api") {
		t.Fatalf("app de formulario: %v", err)
	}
	if _, err := uc.Execute(ctx, dto.UpdateDomainInput{AppRef: apiRef, Hostname: dnsHost, TargetPort: ptr(0)}); !fieldErr(err, "target_port", "") {
		t.Fatalf("puerto inválido: %v", err)
	}
	if f.get(t, dnsHost).TargetService != "api" || len(f.audits(t)) != audits {
		t.Fatal("no debía guardar ni auditar")
	}
	// proxied sin registro propio: 422 (D22).
	if _, err := uc.Execute(ctx, dto.UpdateDomainInput{AppRef: apiRef, Hostname: dnsHost, Proxied: ptr(false)}); !fieldErr(err, "proxied", "muelle no maneja el registro DNS de este dominio") {
		t.Fatalf("proxied: %v", err)
	}
	// Un dominio de otra app: 404.
	if _, err := uc.Execute(ctx, dto.UpdateDomainInput{AppRef: infraRef, Hostname: dnsHost}); !errors.Is(err, sharedrepo.ErrNotFound) {
		t.Fatalf("otra app: %v", err)
	}
	// En la compose, otro slug pasa.
	if _, err := NewCreateDomain(f.deps).Execute(ctx, dto.CreateDomainInput{AppRef: infraRef, Hostname: "redis.tudominio.cl", TargetService: "redis", TargetPort: 6379}); err != nil {
		t.Fatal(err)
	}
	if out, err := uc.Execute(ctx, dto.UpdateDomainInput{AppRef: infraRef, Hostname: "redis.tudominio.cl", TargetService: ptr("cache")}); err != nil || out.TargetService != "cache" {
		t.Fatalf("compose: %+v %v", out, err)
	}
}

// proxied con registros propios cambia el A y el AAAA, y un error +
// cloudflare vuelve a pending; si Cloudflare falla, se guarda igual y queda
// error + cloudflare.
func TestUpdateDomainProxied(t *testing.T) {
	f := newUCFixture(t)
	ctx := context.Background()
	uc := NewUpdateDomain(f.deps)
	f.create(t, dnsHost, false)
	d := f.get(t, dnsHost)
	d.Fail(entity.CodeCloudflare, "antes", f.clock.Now())
	if err := f.repo.Update(ctx, d); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	out, err := uc.Execute(ctx, dto.UpdateDomainInput{AppRef: apiRef, Hostname: dnsHost, Proxied: ptr(true)})
	if err != nil || !out.Proxied || out.Status != "pending" || !out.UpdatedAt.Equal(f.clock.Now()) {
		t.Fatalf("%+v %v", out, err)
	}
	for _, typ := range []string{"A", "AAAA"} {
		if r := f.recordsOf(dnsHost, typ); len(r) != 1 || !r[0].Proxied {
			t.Fatalf("%s: %+v", typ, r)
		}
	}

	f.cf.SetDown(true)
	out, err = uc.Execute(ctx, dto.UpdateDomainInput{AppRef: apiRef, Hostname: dnsHost, Proxied: ptr(false)})
	if got := f.get(t, dnsHost); err != nil || out.Proxied || got.Proxied || got.Status != entity.StatusError || got.ErrorCode != entity.CodeCloudflare {
		t.Fatalf("Cloudflare caído: %+v %v / %+v", out, err, got)
	}
	f.cf.SetDown(false)
	f.settings(t, func(p *shared.ProxySettings) { p.CloudflareToken = "" })
	out, err = uc.Execute(ctx, dto.UpdateDomainInput{AppRef: apiRef, Hostname: dnsHost, Proxied: ptr(true)})
	if err != nil || !out.Proxied || out.LastError == nil || *out.LastError != msgNoToken {
		t.Fatalf("sin token: %+v %v", out, err)
	}
}

// Si el A propio ya no está, D29: error + cloudflare, sin ids.
func TestUpdateDomainProxiedAGone(t *testing.T) {
	f := newUCFixture(t)
	f.create(t, dnsHost, false)
	d := f.get(t, dnsHost)
	f.cf.FailNext("PATCH", d.CFRecordID, 404, 81044, "Record does not exist.")
	out, err := NewUpdateDomain(f.deps).Execute(context.Background(), dto.UpdateDomainInput{AppRef: apiRef, Hostname: dnsHost, Proxied: ptr(true)})
	got := f.get(t, dnsHost)
	if err != nil || out.Status != "error" || got.ErrorCode != entity.CodeCloudflare || got.CFRecordID != "" || got.CFRecordIDAAAA != "" ||
		!strings.Contains(got.LastError, "ya no está") {
		t.Fatalf("%+v %v / %+v", out, err, got)
	}
	if !slices.Contains(f.cf.Calls(), "DELETE /zones/z1/dns_records/"+d.CFRecordIDAAAA) {
		t.Fatalf("el AAAA debía borrarse: %v", f.cf.Calls())
	}
}

func TestDeleteDomain(t *testing.T) {
	f := newUCFixture(t)
	ctx := context.Background()
	uc := NewDeleteDomain(f.deps)
	ref := dto.DomainRef{AppRef: apiRef, Hostname: dnsHost}
	f.create(t, dnsHost, false)
	d := f.get(t, dnsHost)
	loads := len(f.caddy.Loads())

	// Caído o rechazado: 502 y la fila queda.
	f.cf.SetDown(true)
	if err := uc.Execute(ctx, ref); !errors.Is(err, shared.ErrCloudflareUnavailable) {
		t.Fatalf("caído: %v", err)
	}
	f.cf.SetDown(false)
	f.cf.FailNext("DELETE", "dns_records", 403, 10000, "Authentication error")
	if err := uc.Execute(ctx, ref); !errors.Is(err, shared.ErrCloudflareUnavailable) {
		t.Fatalf("rechazado: %v", err)
	}
	if _, err := f.repo.Get(ctx, dnsHost); err != nil {
		t.Fatalf("la fila debía quedar: %v", err)
	}

	// Un registro que ya no está (404) no es error.
	if err := cloudflareDelete(f, d.CFRecordIDAAAA); err != nil {
		t.Fatal(err)
	}
	if err := uc.Execute(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.Get(ctx, dnsHost); !errors.Is(err, sharedrepo.ErrNotFound) {
		t.Fatalf("la fila: %v", err)
	}
	if len(f.recordsOf(dnsHost, "A")) != 0 || len(f.caddy.Loads()) != loads+1 {
		t.Fatalf("registros %v, loads %d", f.cf.Records("z1"), len(f.caddy.Loads()))
	}
	if got := f.audits(t); got[len(got)-1] != "domain.delete domain:"+dnsHost+" "+apiAudit {
		t.Fatalf("auditoría: %v", got)
	}
	if err := uc.Execute(ctx, ref); !errors.Is(err, sharedrepo.ErrNotFound) {
		t.Fatalf("otra vez: %v", err)
	}
}

// Sin token y con registros propios: se borra solo en muelle, y la
// auditoría dice que quedaron en Cloudflare.
func TestDeleteDomainNoToken(t *testing.T) {
	f := newUCFixture(t)
	f.create(t, dnsHost, false)
	f.settings(t, func(p *shared.ProxySettings) { p.CloudflareToken = "" })
	if err := NewDeleteDomain(f.deps).Execute(context.Background(), dto.DomainRef{AppRef: apiRef, Hostname: dnsHost}); err != nil {
		t.Fatal(err)
	}
	if len(f.recordsOf(dnsHost, "A")) != 1 {
		t.Fatal("el A debía quedar en Cloudflare")
	}
	if got := f.audits(t); got[len(got)-1] != "domain.delete domain:"+dnsHost+` {"app":"auditfy/staging/api","dns":"kept"}` {
		t.Fatalf("auditoría: %v", got)
	}
	// De otra app: 404.
	f.create(t, "b.tudominio.cl", false)
	if err := NewDeleteDomain(f.deps).Execute(context.Background(), dto.DomainRef{AppRef: infraRef, Hostname: "b.tudominio.cl"}); !errors.Is(err, sharedrepo.ErrNotFound) {
		t.Fatalf("otra app: %v", err)
	}
}

// cloudflareDelete borra un registro de z1 por fuera de muelle.
func cloudflareDelete(f *ucFixture, id string) error {
	return f.deps.DNS.DeleteRecord(context.Background(), dnsToken, "z1", id)
}
