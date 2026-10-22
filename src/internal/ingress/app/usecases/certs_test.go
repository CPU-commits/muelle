package usecases

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"slices"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// Antes del primer Load (el estado inicial) no se sabe nada: no se sondea.
func TestCheckNotSynced(t *testing.T) {
	f := newUCFixture(t)
	f.state = NewProxyState()
	f.certs = NewCertChecker(f.probe, f.repo, f.deps.Settings, f.origin, f.syncer, f.state, f.ops, f.clock, time.Second, time.Second, time.Second, nil)
	d := f.put(t, nil)
	f.probe.def = f.goodCert()
	if err := f.certs.Check(context.Background(), dnsHost); err != nil {
		t.Fatal(err)
	}
	if got := f.get(t, dnsHost); got.Status != entity.StatusPending || got.UpdatedAt != d.UpdatedAt || len(f.probe.probed()) != 0 {
		t.Fatalf("%+v; sondas %v", got, f.probe.probed())
	}
}

// Sin llegar a Caddy no se sabe: nada cambia.
func TestCheckUnreachable(t *testing.T) {
	f := newUCFixture(t)
	f.put(t, func(d *entity.Domain) { d.Status = entity.StatusActive })
	f.probe.setErr(fmt.Errorf("%w: x", service.ErrCaddyUnreachable))
	if err := f.certs.Check(context.Background(), dnsHost); err != nil {
		t.Fatal(err)
	}
	if got := f.get(t, dnsHost); got.Status != entity.StatusActive {
		t.Fatalf("%+v", got)
	}
}

func TestCheckAppliesRule(t *testing.T) {
	f := newUCFixture(t)
	f.put(t, nil)
	good := f.goodCert()
	f.probe.set(dnsHost, good)
	f.clock.Advance(30 * time.Second)
	if err := f.certs.Check(context.Background(), dnsHost); err != nil {
		t.Fatal(err)
	}
	got := f.get(t, dnsHost)
	if got.Status != entity.StatusActive || got.CertIssuer != "Let's Encrypt" || got.CertExpiresAt == nil ||
		!got.CertExpiresAt.Equal(good.NotAfter) || !got.UpdatedAt.Equal(f.clock.Now()) {
		t.Fatalf("%+v", got)
	}
	// Un dominio que ya no existe no es un error.
	if err := f.certs.Check(context.Background(), "otro.tudominio.cl"); err != nil {
		t.Fatal(err)
	}
}

// Si el dominio cambió entre la sonda y la aplicación (un PATCH), el
// resultado de la sonda no se aplica.
func TestCheckSkipsChangedMeanwhile(t *testing.T) {
	f := newUCFixture(t)
	d := f.put(t, nil)
	f.probe.set(dnsHost, f.goodCert())
	f.probe.block = make(chan struct{})
	f.probe.probing = make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- f.certs.Check(context.Background(), dnsHost) }()
	<-f.probe.probing
	f.clock.Advance(time.Second)
	d.TargetPort = 4000
	d.Touch(f.clock.Now())
	if err := f.repo.Update(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	close(f.probe.block)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := f.get(t, dnsHost); got.Status != entity.StatusPending || got.CertIssuer != "" || got.TargetPort != 4000 {
		t.Fatalf("%+v", got)
	}
}

// checkLocked (retry, con el lock tomado): un error + certificate con la
// sonda inválida vuelve a pending con otros 10 min, y pide reenviar la
// configuración forzando la recarga.
func TestCheckLockedRetry(t *testing.T) {
	f := newUCFixture(t)
	f.put(t, func(d *entity.Domain) { d.Fail(entity.CodeCertificate, "vencido", f.clock.Now()) })
	f.clock.Advance(time.Hour)
	var repush bool
	err := f.ops.Do(context.Background(), time.Second, func(opCtx, dbCtx context.Context) error {
		d, err := f.repo.Get(dbCtx, dnsHost)
		if err != nil {
			return err
		}
		repush, err = f.certs.checkLocked(opCtx, dbCtx, &d)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	got := f.get(t, dnsHost)
	if !repush || got.Status != entity.StatusPending || !got.UpdatedAt.Equal(f.clock.Now()) || got.LastError != "" {
		t.Fatalf("repush %v, %+v", repush, got)
	}

	// Un pending con la sonda inválida no cambia (retry no reinicia el plazo).
	f.clock.Advance(time.Minute)
	err = f.ops.Do(context.Background(), time.Second, func(opCtx, dbCtx context.Context) error {
		d, _ := f.repo.Get(dbCtx, dnsHost)
		repush, err = f.certs.checkLocked(opCtx, dbCtx, &d)
		return err
	})
	if again := f.get(t, dnsHost); err != nil || repush || !again.UpdatedAt.Equal(got.UpdatedAt) {
		t.Fatalf("pending: %v %v %+v", err, repush, again)
	}
}

// La verificación de cada hora mira pending, active y error + certificate;
// no mira los error + cloudflare ni record_exists.
func TestCheckAll(t *testing.T) {
	f := newUCFixture(t)
	add := func(host string, mod func(*entity.Domain)) {
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
	}
	now := f.clock.Now()
	add("a.tudominio.cl", nil)
	add("b.tudominio.cl", func(d *entity.Domain) { d.MarkActive(now) })
	add("c.tudominio.cl", func(d *entity.Domain) { d.Fail(entity.CodeCertificate, "x", now) })
	add("d.tudominio.cl", func(d *entity.Domain) { d.Fail(entity.CodeCloudflare, "x", now) })
	add("e.tudominio.cl", func(d *entity.Domain) { d.Fail(entity.CodeRecordExists, "x", now) })
	f.probe.def = f.goodCert()
	if err := f.certs.CheckAll(context.Background(), EligibleHourly); err != nil {
		t.Fatal(err)
	}
	if got := f.probe.probed(); !slices.Equal(got, []string{"a.tudominio.cl", "b.tudominio.cl", "c.tudominio.cl"}) {
		t.Fatalf("sondas: %v", got)
	}
	for _, h := range []string{"a.tudominio.cl", "b.tudominio.cl", "c.tudominio.cl"} {
		if got := f.get(t, h); got.Status != entity.StatusActive {
			t.Errorf("%s: %+v", h, got)
		}
	}
	if got := f.get(t, "d.tudominio.cl"); got.Status != entity.StatusError {
		t.Errorf("d: %+v", got)
	}
}

// Run: ScheduleFirst dispara una sonda de ese host a los FirstCheck; el
// barrido de cada PendingEvery sondea los pending; vuelve al cancelar.
func TestCertRun(t *testing.T) {
	f := newUCFixture(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	f.certs = NewCertChecker(f.probe, f.repo, f.deps.Settings, f.origin, f.syncer, f.state, f.ops, f.clock, 10*time.Millisecond, time.Hour, time.Second, log)
	d, err := entity.NewDomain(f.apiID, "x.tudominio.cl", "api", 3000, false, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	d.MarkActive(f.clock.Now())
	if err := f.repo.Create(context.Background(), &d); err != nil {
		t.Fatal(err)
	}
	f.probe.set("x.tudominio.cl", f.goodCert())
	runInBackground(t, f.certs.Run)
	start := time.Now()
	f.certs.ScheduleFirst("x.tudominio.cl")
	eventually(t, time.Second, "la sonda de ScheduleFirst", func() bool { return slices.Contains(f.probe.probed(), "x.tudominio.cl") })
	if el := time.Since(start); el < 10*time.Millisecond {
		t.Fatalf("sondeó a los %s, antes de FirstCheck", el)
	}

	// El barrido de los pending.
	g := newUCFixture(t)
	g.certs = NewCertChecker(g.probe, g.repo, g.deps.Settings, g.origin, g.syncer, g.state, g.ops, g.clock, time.Hour, 30*time.Millisecond, time.Second, log)
	g.put(t, nil)
	g.probe.set(dnsHost, g.goodCert())
	runInBackground(t, g.certs.Run)
	eventually(t, time.Second, "el barrido de los pending", func() bool {
		d, err := g.repo.Get(context.Background(), dnsHost)
		return err == nil && d.Status == entity.StatusActive
	})

	// ScheduleFirst no bloquea aunque la cola esté llena.
	h := newUCFixture(t)
	for range 200 {
		h.certs.ScheduleFirst(dnsHost)
	}
}

// testLeaf es una hoja autofirmada para host, en PEM y DER.
func testLeaf(t *testing.T, host string, notAfter time.Time) (string, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host},
		DNSNames: []string{host}, NotBefore: notAfter.Add(-90 * 24 * time.Hour), NotAfter: notAfter}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), der
}

// originServed deja dnsHost como un dominio que se sirve con Origin CA (la
// opción encendida, manejado y proxied, visto con el proxy y con un active
// de 90 días), y devuelve el DER de su hoja.
func (f *ucFixture) originServed(t *testing.T) []byte {
	t.Helper()
	ctx := context.Background()
	f.settings(t, func(p *shared.ProxySettings) { p.CloudflareOriginCA = true })
	f.put(t, func(d *entity.Domain) {
		d.Status, d.TLSMode, d.CFZoneID, d.CFRecordID, d.Proxied = entity.StatusActive, entity.TLSCloudflare, "z1", "rec-a", true
	})
	certPEM, der := testLeaf(t, dnsHost, f.clock.Now().Add(90*24*time.Hour))
	c := entity.OriginCert{Hostname: dnsHost, ZoneID: "z1", CFCertID: "100", CertPEM: certPEM, KeyPEM: "KEY",
		ExpiresAt: f.clock.Now().Add(90 * 24 * time.Hour), CreatedAt: f.clock.Now()}
	if err := f.origin.Certs.Insert(ctx, &c); err != nil {
		t.Fatal(err)
	}
	if err := f.origin.Hosts.SetObserved(ctx, dnsHost, entity.ObservedProxied, f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	return der
}

// samePins dice si got son las hojas want, en ese orden.
func samePins(got [][]byte, want ...[]byte) bool { return slices.EqualFunc(got, want, bytes.Equal) }

// La verificación fija la hoja de Origin CA cuando el host se sirve con ella
// (§8 del diseño de Origin CA), con la misma regla que la configuración de
// Caddy; si no, pinned está vacío.
func TestCheckPinsOriginCA(t *testing.T) {
	f := newUCFixture(t)
	der := f.originServed(t)
	ctx := context.Background()
	if err := f.certs.Check(ctx, dnsHost); err != nil {
		t.Fatal(err)
	}
	if got := f.probe.pinned(dnsHost); !samePins(got, der) {
		t.Fatalf("pinned: %x", got)
	}
	// La de retry (checkLocked) también.
	f.probe.pins = nil
	err := f.ops.Do(ctx, time.Second, func(opCtx, dbCtx context.Context) error {
		d, err := f.repo.Get(dbCtx, dnsHost)
		if err != nil {
			return err
		}
		_, err = f.certs.checkLocked(opCtx, dbCtx, &d)
		return err
	})
	if err != nil || !samePins(f.probe.pinned(dnsHost), der) {
		t.Fatalf("checkLocked: %v %x", err, f.probe.pinned(dnsHost))
	}

	// Visto en DNS only: Let's Encrypt, sin fijar.
	if err := f.origin.Hosts.SetObserved(ctx, dnsHost, entity.ObservedDNSOnly, f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	if err := f.certs.CheckAll(ctx, EligibleHourly); err != nil {
		t.Fatal(err)
	}
	if got := f.probe.pinned(dnsHost); len(got) != 0 {
		t.Fatalf("DNS only: %x", got)
	}
	// La opción apagada: tampoco.
	if err := f.origin.Hosts.SetObserved(ctx, dnsHost, entity.ObservedProxied, f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	f.settings(t, nil)
	if err := f.certs.Check(ctx, dnsHost); err != nil {
		t.Fatal(err)
	}
	if got := f.probe.pinned(dnsHost); len(got) != 0 {
		t.Fatalf("apagada: %x", got)
	}
}

// En una renovación, Insert deja active la hoja nueva, pero Caddy sirve la
// vieja hasta el Sync del final de la pasada. Mientras ningún Sync bueno
// empezó después del retiro de la vieja, la verificación fija las dos: la
// que Caddy todavía sirve no deja el dominio en error.
func TestCheckPinsRenewal(t *testing.T) {
	f := newUCFixture(t)
	ctx := context.Background()
	old := f.originServed(t)
	f.probe.set(dnsHost, f.goodCert())
	if err := f.syncer.Sync(ctx, false); err != nil { // Caddy sirve la vieja
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	certPEM, renewed := testLeaf(t, dnsHost, f.clock.Now().Add(90*24*time.Hour))
	c := entity.OriginCert{Hostname: dnsHost, ZoneID: "z1", CFCertID: "101", CertPEM: certPEM, KeyPEM: "KEY",
		ExpiresAt: f.clock.Now().Add(90 * 24 * time.Hour), CreatedAt: f.clock.Now()}
	if err := f.origin.Certs.Insert(ctx, &c); err != nil {
		t.Fatal(err)
	}
	if err := f.certs.Check(ctx, dnsHost); err != nil {
		t.Fatal(err)
	}
	if got := f.probe.pinned(dnsHost); !samePins(got, renewed, old) {
		t.Fatalf("antes de la carga: %d hojas", len(got))
	}
	// Un Sync bueno que empezó después del retiro: Caddy ya sirve la nueva.
	f.clock.Advance(time.Second)
	if err := f.syncer.Sync(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := f.certs.CheckAll(ctx, EligibleHourly); err != nil {
		t.Fatal(err)
	}
	if got := f.probe.pinned(dnsHost); !samePins(got, renewed) {
		t.Fatalf("después de la carga: %d hojas", len(got))
	}
	// La opción apagada: la nueva no se sirve, y la vieja ya no está en Caddy.
	f.settings(t, nil)
	if err := f.certs.Check(ctx, dnsHost); err != nil {
		t.Fatal(err)
	}
	if got := f.probe.pinned(dnsHost); len(got) != 0 {
		t.Fatalf("apagada: %d hojas", len(got))
	}
}
