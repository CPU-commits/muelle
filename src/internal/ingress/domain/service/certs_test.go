package service

import (
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
)

// La regla de los certificados del contrato §3.1: pending → active con uno
// válido que vence en más de 14 días; pending → error a los 10 min de su
// último cambio; active → error si es inválido o vence pronto; error +
// certificate → active con uno válido, o pending con retry.
func TestApplyCert(t *testing.T) {
	t0 := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	good := CertObservation{Valid: true, Issuer: "Let's Encrypt", NotAfter: t0.Add(60 * 24 * time.Hour)}
	soon := CertObservation{Valid: true, Issuer: "Let's Encrypt", NotAfter: t0.Add(10 * 24 * time.Hour)}
	soon13 := CertObservation{Valid: true, Issuer: "Let's Encrypt", NotAfter: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)}
	invalid := CertObservation{Problem: "el certificado no lo firmó una CA de confianza"}
	otherIssuer := good
	otherIssuer.Issuer = "ZeroSSL"

	type want struct {
		changed   bool
		status    entity.Status
		code      entity.ErrorCode
		msg       []string // fragmentos del mensaje
		updated   time.Time
		issuer    string
		expires   *time.Time
		untouched bool // el dominio quedó idéntico
	}
	base := func(status entity.Status, code entity.ErrorCode, updated time.Time) entity.Domain {
		d := entity.Domain{ID: 1, Hostname: "api.tudominio.cl", TLSMode: entity.TLSHTTP, Status: status, ErrorCode: code, UpdatedAt: updated}
		if code != "" {
			d.LastError = "antes"
		}
		return d
	}
	ptr := func(t time.Time) *time.Time { u := t.UTC().Truncate(time.Millisecond); return &u }
	cases := []struct {
		name  string
		d     entity.Domain
		o     CertObservation
		now   time.Time
		retry bool
		want  want
	}{
		{"pending y bueno", base(entity.StatusPending, "", t0.Add(-time.Minute)), good, t0, false,
			want{changed: true, status: entity.StatusActive, updated: t0, issuer: "Let's Encrypt", expires: ptr(good.NotAfter)}},
		{"pending a los 5 min, inválido", base(entity.StatusPending, "", t0.Add(-5*time.Minute)), invalid, t0, false,
			want{untouched: true}},
		{"pending a los 5 min, vence en 10 días", base(entity.StatusPending, "", t0.Add(-5*time.Minute)), soon, t0, false,
			want{changed: true, status: entity.StatusPending, updated: t0.Add(-5 * time.Minute), issuer: "Let's Encrypt", expires: ptr(soon.NotAfter)}},
		{"pending a los 10 min, inválido", base(entity.StatusPending, "", t0.Add(-10*time.Minute)), invalid, t0, false,
			want{changed: true, status: entity.StatusError, code: entity.CodeCertificate, updated: t0,
				msg: []string{"Caddy no consiguió un certificado válido en 10 minutos: el certificado no lo firmó una CA de confianza", "puertos 80 y 443"}}},
		{"pending a los 10 min, vence en 10 días", base(entity.StatusPending, "", t0.Add(-10*time.Minute)), soon, t0, false,
			want{changed: true, status: entity.StatusError, code: entity.CodeCertificate, updated: t0, issuer: "Let's Encrypt", expires: ptr(soon.NotAfter),
				msg: []string{"Caddy no consiguió un certificado válido en 10 minutos", "vence el 24 oct 2026"}}},
		{"active, otro issuer", base(entity.StatusActive, "", t0.Add(-time.Hour)), otherIssuer, t0, false,
			want{changed: true, status: entity.StatusActive, updated: t0.Add(-time.Hour), issuer: "ZeroSSL", expires: ptr(good.NotAfter)}},
		{"active, vence en 13 días", base(entity.StatusActive, "", t0.Add(-time.Hour)), soon13, t0, false,
			want{changed: true, status: entity.StatusError, code: entity.CodeCertificate, updated: t0, issuer: "Let's Encrypt", expires: ptr(soon13.NotAfter),
				msg: []string{"el certificado vence el 27 oct 2026 y Caddy no lo renovó"}}},
		{"active, inválido", base(entity.StatusActive, "", t0.Add(-time.Hour)), invalid, t0, false,
			want{changed: true, status: entity.StatusError, code: entity.CodeCertificate, updated: t0,
				msg: []string{"el certificado no lo firmó una CA de confianza"}}},
		{"error certificate, bueno", base(entity.StatusError, entity.CodeCertificate, t0.Add(-time.Hour)), good, t0, false,
			want{changed: true, status: entity.StatusActive, updated: t0, issuer: "Let's Encrypt", expires: ptr(good.NotAfter)}},
		{"error certificate, inválido", base(entity.StatusError, entity.CodeCertificate, t0.Add(-time.Hour)), invalid, t0, false,
			want{untouched: true}},
		{"error certificate, inválido con retry", base(entity.StatusError, entity.CodeCertificate, t0.Add(-time.Hour)), invalid, t0, true,
			want{changed: true, status: entity.StatusPending, updated: t0}},
		{"pending, inválido con retry", base(entity.StatusPending, "", t0.Add(-time.Minute)), invalid, t0, true,
			want{untouched: true}},
		{"error cloudflare, bueno", base(entity.StatusError, entity.CodeCloudflare, t0.Add(-time.Hour)), good, t0, true,
			want{untouched: true}},
		{"error record_exists, inválido", base(entity.StatusError, entity.CodeRecordExists, t0.Add(-time.Hour)), invalid, t0, true,
			want{untouched: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := c.d
			before := c.d
			changed := ApplyCert(&d, c.o, c.now, c.retry)
			if c.want.untouched {
				if changed || d.Status != before.Status || d.UpdatedAt != before.UpdatedAt || d.LastError != before.LastError ||
					d.CertIssuer != before.CertIssuer || d.CertExpiresAt != nil {
					t.Fatalf("no debía cambiar: changed=%v %+v", changed, d)
				}
				return
			}
			if changed != c.want.changed || d.Status != c.want.status || d.ErrorCode != c.want.code || !d.UpdatedAt.Equal(c.want.updated) {
				t.Fatalf("changed=%v status=%s code=%s updated=%s, esperaba %+v", changed, d.Status, d.ErrorCode, d.UpdatedAt, c.want)
			}
			if c.want.code == "" && d.LastError != "" {
				t.Fatalf("sin error, pero LastError = %q", d.LastError)
			}
			for _, m := range c.want.msg {
				if !strings.Contains(d.LastError, m) {
					t.Errorf("el mensaje %q no dice %q", d.LastError, m)
				}
			}
			if d.CertIssuer != c.want.issuer {
				t.Errorf("issuer %q, esperaba %q", d.CertIssuer, c.want.issuer)
			}
			switch {
			case c.want.expires == nil && d.CertExpiresAt != nil:
				t.Errorf("vencimiento %v, esperaba nil", d.CertExpiresAt)
			case c.want.expires != nil && (d.CertExpiresAt == nil || !d.CertExpiresAt.Equal(*c.want.expires)):
				t.Errorf("vencimiento %v, esperaba %v", d.CertExpiresAt, c.want.expires)
			}
		})
	}
}

// El mensaje del plazo vencido dice qué revisar según el modo de TLS.
func TestApplyCertDeadlineHint(t *testing.T) {
	t0 := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	invalid := CertObservation{Problem: "Caddy no presentó un certificado para api.tudominio.cl"}
	for mode, hint := range map[entity.TLSMode]string{
		entity.TLSHTTP:       "revisa que el registro DNS apunte a la IP pública y que los puertos 80 y 443 lleguen a muelle-caddy",
		entity.TLSCloudflare: "revisa que el token de Cloudflare pueda editar el DNS de la zona",
	} {
		d := entity.Domain{Hostname: "api.tudominio.cl", TLSMode: mode, Status: entity.StatusPending, UpdatedAt: t0.Add(-11 * time.Minute)}
		if mode == entity.TLSCloudflare {
			d.CFZoneID = "z1"
		}
		if !ApplyCert(&d, invalid, t0, false) || !strings.Contains(d.LastError, hint) {
			t.Errorf("%s: %q", mode, d.LastError)
		}
	}
}

// Con retry, un error + certificate con uno válido que vence pronto vuelve a
// pending (otros 10 min), y guarda lo que vio.
func TestApplyCertRetryExpiring(t *testing.T) {
	t0 := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	d := entity.Domain{Status: entity.StatusError, ErrorCode: entity.CodeCertificate, LastError: "x", UpdatedAt: t0.Add(-time.Hour)}
	o := CertObservation{Valid: true, Issuer: "R3", NotAfter: t0.Add(5 * 24 * time.Hour)}
	if !ApplyCert(&d, o, t0, true) || d.Status != entity.StatusPending || !d.UpdatedAt.Equal(t0) || d.CertIssuer != "R3" {
		t.Fatalf("%+v", d)
	}
}

func TestShortDate(t *testing.T) {
	for in, want := range map[time.Time]string{
		time.Date(2027, 1, 2, 23, 0, 0, 0, time.UTC):                       "2 ene 2027",
		time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC):                      "31 dic 2026",
		time.Date(2026, 9, 1, 22, 0, 0, 0, time.FixedZone("CLT", -3*3600)): "2 sep 2026",
	} {
		if got := shortDate(in); got != want {
			t.Errorf("shortDate(%s) = %q, esperaba %q", in, got, want)
		}
	}
}
