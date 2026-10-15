package entity

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func TestNewDomain(t *testing.T) {
	d, err := NewDomain(7, "api.tudominio.cl", "api", 3000, true, t0)
	if err != nil {
		t.Fatal(err)
	}
	want := Domain{
		AppID: 7, Hostname: "api.tudominio.cl", TargetService: "api", TargetPort: 3000, Proxied: true,
		TLSMode: TLSHTTP, Status: StatusPending, CreatedAt: t0, UpdatedAt: t0,
	}
	if d != want {
		t.Fatalf("dominio:\n%+v\nesperaba\n%+v", d, want)
	}
	if d.Managed() {
		t.Fatal("un dominio nuevo no tiene registros que maneje muelle")
	}
	if d, _ := NewDomain(7, "api.tudominio.cl", "api", 3000, false, t0); d.Proxied {
		t.Fatal("proxied pedido false")
	}

	for _, c := range []struct {
		field, hostname, service string
		port                     int
	}{
		{"hostname", "", "api", 80},
		{"hostname", "API.x.cl", "api", 80},
		{"hostname", "*.x.cl", "api", 80},
		{"hostname", "x.cl.", "api", 80},
		{"hostname", "1.2.3.4", "api", 80},
		{"target_service", "a.x.cl", "Api", 80},
		{"target_service", "a.x.cl", "-a", 80},
		{"target_service", "a.x.cl", strings.Repeat("a", 17), 80},
		{"target_port", "a.x.cl", "api", 0},
		{"target_port", "a.x.cl", "api", 65536},
	} {
		_, err := NewDomain(7, c.hostname, c.service, c.port, false, t0)
		var ve *validate.Error
		if !errors.As(err, &ve) || ve.Field != c.field {
			t.Errorf("NewDomain(%q, %q, %d): %v, esperaba un *validate.Error en %s", c.hostname, c.service, c.port, err, c.field)
		}
	}
}

func TestDomainTransitions(t *testing.T) {
	d, err := NewDomain(7, "api.tudominio.cl", "api", 3000, false, t0)
	if err != nil {
		t.Fatal(err)
	}
	t1, t2, t3, t4 := t0.Add(time.Minute), t0.Add(2*time.Minute), t0.Add(3*time.Minute), t0.Add(4*time.Minute)

	d.Fail(CodeCloudflare, "m", t1)
	if d.Status != StatusError || d.ErrorCode != CodeCloudflare || d.LastError != "m" || !d.UpdatedAt.Equal(t1) {
		t.Fatalf("Fail: %+v", d)
	}
	d.MarkPending(t2)
	if d.Status != StatusPending || d.ErrorCode != "" || d.LastError != "" || !d.UpdatedAt.Equal(t2) {
		t.Fatalf("MarkPending: %+v", d)
	}
	d.Fail(CodeCertificate, "vence", t2)
	d.MarkActive(t3)
	if d.Status != StatusActive || d.ErrorCode != "" || d.LastError != "" || !d.UpdatedAt.Equal(t3) {
		t.Fatalf("MarkActive: %+v", d)
	}
	before := d
	d.Touch(t4)
	before.UpdatedAt = t4
	if d != before {
		t.Fatalf("Touch solo mueve UpdatedAt: %+v", d)
	}

	// Una verificación que solo ve el certificado no es un cambio del dominio
	// (contrato §3.1): UpdatedAt no se mueve.
	exp := t0.Add(90 * 24 * time.Hour)
	if !d.SetCertificate("Let's Encrypt", exp) {
		t.Fatal("SetCertificate: el primero es un cambio")
	}
	if d.CertIssuer != "Let's Encrypt" || d.CertExpiresAt == nil || !d.CertExpiresAt.Equal(exp) || !d.UpdatedAt.Equal(t4) {
		t.Fatalf("SetCertificate: %+v", d)
	}
	if d.SetCertificate("Let's Encrypt", exp.In(time.FixedZone("CLT", -3*3600))) {
		t.Fatal("SetCertificate: el mismo certificado no es un cambio")
	}
	if !d.SetCertificate("Let's Encrypt", exp.Add(time.Hour)) || !d.SetCertificate("ZeroSSL", exp.Add(time.Hour)) {
		t.Fatal("SetCertificate: otro vencimiento u otro emisor es un cambio")
	}
	if d.CertExpiresAt.Location() != time.UTC {
		t.Fatalf("CertExpiresAt en UTC: %v", d.CertExpiresAt)
	}
}

func TestSetTarget(t *testing.T) {
	d, err := NewDomain(7, "api.tudominio.cl", "api", 3000, false, t0)
	if err != nil {
		t.Fatal(err)
	}
	t1 := t0.Add(time.Minute)
	if changed, err := d.SetTarget("api", 3000, t1); err != nil || changed || !d.UpdatedAt.Equal(t0) {
		t.Fatalf("mismo destino: changed=%v err=%v %+v", changed, err, d)
	}
	if changed, err := d.SetTarget("web", 3000, t1); err != nil || !changed || d.TargetService != "web" || !d.UpdatedAt.Equal(t1) {
		t.Fatalf("otro servicio: changed=%v err=%v %+v", changed, err, d)
	}
	if changed, err := d.SetTarget("web", 8080, t1.Add(time.Minute)); err != nil || !changed || d.TargetPort != 8080 {
		t.Fatalf("otro puerto: changed=%v err=%v %+v", changed, err, d)
	}
	for field, f := range map[string]func() (bool, error){
		"target_service": func() (bool, error) { return d.SetTarget("Web", 8080, t1) },
		"target_port":    func() (bool, error) { return d.SetTarget("web", 0, t1) },
	} {
		before := d
		changed, err := f()
		var ve *validate.Error
		if !errors.As(err, &ve) || ve.Field != field || changed || d != before {
			t.Errorf("%s inválido: changed=%v err=%v %+v", field, changed, err, d)
		}
	}
}
