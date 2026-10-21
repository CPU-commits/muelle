package sqlite

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func newDomain(t *testing.T, appID int64, hostname string) entity.Domain {
	t.Helper()
	d, err := entity.NewDomain(appID, hostname, "api", 3000, false, t0)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDomainsCRUD(t *testing.T) {
	db := sqlitetest.New(t)
	app := ingresstest.App(t, db, "auditfy", "staging", "api", "service")
	other := ingresstest.App(t, db, "auditfy", "prod", "api", "service")
	repo := NewDomains(db)
	ctx := context.Background()

	if l, err := repo.List(ctx); err != nil || l == nil || len(l) != 0 {
		t.Fatalf("List vacío: %#v %v", l, err)
	}
	if l, err := repo.ListByApp(ctx, app); err != nil || l == nil || len(l) != 0 {
		t.Fatalf("ListByApp vacío: %#v %v", l, err)
	}

	// Todos los NULL: sin zona, sin registros, sin error y sin certificado.
	www := newDomain(t, app, "www.auditfy.cl")
	if err := repo.Create(ctx, &www); err != nil {
		t.Fatal(err)
	}
	if www.ID == 0 {
		t.Fatal("Create no asignó el id")
	}
	got, err := repo.Get(ctx, "www.auditfy.cl")
	if err != nil {
		t.Fatal(err)
	}
	if got != www || got.CFZoneID != "" || got.CertExpiresAt != nil {
		t.Fatalf("ida y vuelta con NULL:\n%+v\nesperaba\n%+v", got, www)
	}

	// Todos llenos.
	exp := time.Date(2027, 1, 1, 0, 0, 0, 123_000_000, time.UTC)
	api := entity.Domain{
		AppID: app, Hostname: "api.auditfy.cl", TargetService: "web", TargetPort: 8080, Proxied: true,
		TLSMode: entity.TLSCloudflare, CFZoneID: "z1", CFRecordID: "r4", CFRecordIDAAAA: "r6",
		Status: entity.StatusError, ErrorCode: entity.CodeCertificate, LastError: "vence pronto",
		CertIssuer: "Let's Encrypt", CertExpiresAt: &exp,
		CreatedAt: t0, UpdatedAt: t0.Add(time.Minute),
	}
	if err := repo.Create(ctx, &api); err != nil {
		t.Fatal(err)
	}
	got, err = repo.Get(ctx, "api.auditfy.cl")
	if err != nil {
		t.Fatal(err)
	}
	if got.CertExpiresAt == nil || !got.CertExpiresAt.Equal(exp) || got.CertExpiresAt.Location() != time.UTC {
		t.Fatalf("CertExpiresAt: %v", got.CertExpiresAt)
	}
	want := api
	want.CertExpiresAt = got.CertExpiresAt
	if got != want {
		t.Fatalf("ida y vuelta llena:\n%+v\nesperaba\n%+v", got, want)
	}

	b := newDomain(t, other, "b.auditfy.cl")
	if err := repo.Create(ctx, &b); err != nil {
		t.Fatal(err)
	}
	hosts := func(l []entity.Domain, err error) []string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, d := range l {
			out = append(out, d.Hostname)
		}
		return out
	}
	if h := hosts(repo.List(ctx)); !reflect.DeepEqual(h, []string{"api.auditfy.cl", "b.auditfy.cl", "www.auditfy.cl"}) {
		t.Fatalf("List: %v", h)
	}
	if h := hosts(repo.ListByApp(ctx, app)); !reflect.DeepEqual(h, []string{"api.auditfy.cl", "www.auditfy.cl"}) {
		t.Fatalf("ListByApp: %v", h)
	}

	// Update cambia estado, ids, certificado y destino; no el hostname ni la app.
	exp2 := exp.Add(24 * time.Hour)
	www.TargetService, www.TargetPort, www.Proxied = "web", 8080, true
	www.TLSMode, www.CFZoneID, www.CFRecordID, www.CFRecordIDAAAA = entity.TLSCloudflare, "z1", "r1", "r2"
	www.SetCertificate("ZeroSSL", exp2)
	www.Fail(entity.CodeCloudflare, "Cloudflare no responde", t0.Add(time.Hour))
	if err := repo.Update(ctx, www); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.Get(ctx, "www.auditfy.cl"); err != nil || !sameDomain(got, www) {
		t.Fatalf("Update:\n%+v\nesperaba\n%+v (%v)", got, www, err)
	}
	// Y de vuelta a NULL.
	www.CFZoneID, www.CFRecordID, www.CFRecordIDAAAA, www.TLSMode = "", "", "", entity.TLSHTTP
	www.CertIssuer, www.CertExpiresAt = "", nil
	www.MarkPending(t0.Add(2 * time.Hour))
	if err := repo.Update(ctx, www); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.Get(ctx, "www.auditfy.cl"); err != nil || got != www {
		t.Fatalf("Update a NULL:\n%+v\nesperaba\n%+v (%v)", got, www, err)
	}

	if err := repo.Delete(ctx, www.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, "www.auditfy.cl"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Get después de Delete: %v", err)
	}
	if h := hosts(repo.ListByApp(ctx, app)); !reflect.DeepEqual(h, []string{"api.auditfy.cl"}) {
		t.Fatalf("ListByApp después de Delete: %v", h)
	}
}

// sameDomain compara dos dominios con el vencimiento por valor.
func sameDomain(a, b entity.Domain) bool {
	if (a.CertExpiresAt == nil) != (b.CertExpiresAt == nil) {
		return false
	}
	if a.CertExpiresAt != nil && !a.CertExpiresAt.Equal(*b.CertExpiresAt) {
		return false
	}
	a.CertExpiresAt, b.CertExpiresAt = nil, nil
	return a == b
}

func TestDomainsHostnameTaken(t *testing.T) {
	db := sqlitetest.New(t)
	app := ingresstest.App(t, db, "auditfy", "staging", "api", "service")
	other := ingresstest.App(t, db, "otro", "prod", "web", "compose")
	repo := NewDomains(db)
	ctx := context.Background()

	d := newDomain(t, app, "api.auditfy.cl")
	if err := repo.Create(ctx, &d); err != nil {
		t.Fatal(err)
	}
	dup := newDomain(t, other, "api.auditfy.cl")
	err := repo.Create(ctx, &dup)
	if !errors.Is(err, service.ErrHostnameTaken) || !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("hostname repetido en otra app: %v", err)
	}
	if dup.ID != 0 {
		t.Fatalf("un Create que falla no asigna id: %d", dup.ID)
	}
}

func TestDomainsNotFound(t *testing.T) {
	db := sqlitetest.New(t)
	app := ingresstest.App(t, db, "auditfy", "staging", "api", "service")
	repo := NewDomains(db)
	ctx := context.Background()

	if _, err := repo.Get(ctx, "nada.auditfy.cl"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Get: %v", err)
	}
	d := newDomain(t, app, "nada.auditfy.cl")
	d.ID = 999
	if err := repo.Update(ctx, d); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Update: %v", err)
	}
	if err := repo.Delete(ctx, 999); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Delete: %v", err)
	}
}

// Los CHECK de 0003 (D19) también protegen al repositorio.
func TestDomainsInvariants(t *testing.T) {
	db := sqlitetest.New(t)
	app := ingresstest.App(t, db, "auditfy", "staging", "api", "service")
	repo := NewDomains(db)
	ctx := context.Background()

	d := newDomain(t, app, "api.auditfy.cl")
	if err := repo.Create(ctx, &d); err != nil {
		t.Fatal(err)
	}
	bad := d
	bad.TLSMode, bad.CFZoneID, bad.CFRecordIDAAAA = entity.TLSCloudflare, "z1", "r6"
	if err := repo.Update(ctx, bad); err == nil {
		t.Fatal("Update con un AAAA sin A debió fallar")
	}
	if got, err := repo.Get(ctx, "api.auditfy.cl"); err != nil || got != d {
		t.Fatalf("el Update que falló cambió la fila: %+v %v", got, err)
	}
}
