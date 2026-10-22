package service

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

var originNow = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

// managedDomain es un dominio de una app que califica: tls cloudflare, con
// zona, con el A de muelle y proxied.
func managedDomain() entity.Domain {
	return entity.Domain{Hostname: "api.auditfy.cl", TLSMode: entity.TLSCloudflare, CFZoneID: "z1", CFRecordID: "rec-a",
		Proxied: true, Status: entity.StatusActive}
}

func activeCert(host, zone string, expires time.Time) *entity.OriginCert {
	return &entity.OriginCert{ID: 1, Hostname: host, ZoneID: zone, CFCertID: "100", CertPEM: "CERT", KeyPEM: "KEY",
		ExpiresAt: expires, State: entity.OriginActive}
}

func observed(o entity.Observation) entity.OriginHost {
	return entity.OriginHost{Hostname: "api.auditfy.cl", Observed: o}
}

// La opción vale solo encendida, con token y con el proxy habilitado (§4.1 y
// §4.3 del diseño).
func TestOriginOn(t *testing.T) {
	p := shared.DefaultProxySettings()
	p.CloudflareOriginCA, p.CloudflareToken = true, "cf-token-1234567890abcdef"
	if !OriginOn(p, true) {
		t.Fatal("encendida, con token y con el proxy")
	}
	if OriginOn(p, false) {
		t.Error("con el proxy deshabilitado no hay Caddy al que darle el certificado")
	}
	off := p
	off.CloudflareOriginCA = false
	noToken := p
	noToken.CloudflareToken = ""
	if OriginOn(off, true) || OriginOn(noToken, true) {
		t.Error("apagada, o sin token, no vale")
	}
}

// Candidato (§4.1): un dominio de una app con tls cloudflare, zona, el A de
// muelle y proxied en muelle; el panel con su zona, aunque el registro sea
// ajeno. El status del dominio no cuenta.
func TestOriginTargets(t *testing.T) {
	cases := []struct {
		name               string
		mod                func(*entity.Domain)
		eligible, proxied  bool
		candidate, isPanel bool
	}{
		{"manejado y proxied", nil, true, true, true, false},
		{"en error por Cloudflare", func(d *entity.Domain) { d.Status, d.ErrorCode = entity.StatusError, entity.CodeCloudflare }, true, true, true, false},
		{"sin proxied (DNS only en muelle)", func(d *entity.Domain) { d.Proxied = false }, true, false, false, false},
		{"tls http", func(d *entity.Domain) { d.TLSMode, d.CFZoneID, d.CFRecordID = entity.TLSHTTP, "", "" }, false, true, false, false},
		{"sin zona", func(d *entity.Domain) { d.CFZoneID = "" }, false, true, false, false},
		{"registro ajeno (sin cf_record_id)", func(d *entity.Domain) { d.CFRecordID = "" }, false, true, false, false},
	}
	for _, c := range cases {
		d := managedDomain()
		if c.mod != nil {
			c.mod(&d)
		}
		tg := DomainOriginTarget(d)
		if tg.Eligible != c.eligible || tg.Proxied != c.proxied || tg.Candidate() != c.candidate || tg.Panel ||
			tg.Hostname != d.Hostname || tg.ZoneID != d.CFZoneID || tg.RecordID != d.CFRecordID {
			t.Errorf("%s: %+v (candidato %v)", c.name, tg, tg.Candidate())
		}
	}
	p := PanelOriginTarget("panel.auditfy.cl", "z1")
	if !p.Panel || !p.Eligible || !p.Proxied || !p.Candidate() || p.RecordID != "" || p.ZoneID != "z1" {
		t.Errorf("panel con zona: %+v", p)
	}
	if p := PanelOriginTarget("panel.auditfy.cl", ""); p.Eligible || p.Candidate() {
		t.Errorf("panel sin zona: %+v", p)
	}
	if p := PanelOriginTarget("", "z1"); p.Eligible || p.Candidate() {
		t.Errorf("sin panel_domain: %+v", p)
	}
}

// Lo observado sale de los A, AAAA y CNAME del nombre (§4.1): todos con el
// proxy (y en un dominio, el A de muelle entre ellos) es proxied.
func TestObserveRecords(t *testing.T) {
	a := func(id string, proxied bool) Record { return Record{ID: id, Type: "A", Proxied: proxied} }
	aaaa := func(id string, proxied bool) Record { return Record{ID: id, Type: "AAAA", Proxied: proxied} }
	cname := func(id string, proxied bool) Record { return Record{ID: id, Type: "CNAME", Proxied: proxied} }
	txt := Record{ID: "t", Type: "TXT"}
	cases := []struct {
		name    string
		records []Record
		managed string
		want    entity.Observation
	}{
		{"el A de muelle con proxy", []Record{a("rec-a", true)}, "rec-a", entity.ObservedProxied},
		{"A y AAAA con proxy", []Record{a("rec-a", true), aaaa("rec-6", true), txt}, "rec-a", entity.ObservedProxied},
		{"el AAAA sin proxy", []Record{a("rec-a", true), aaaa("rec-6", false)}, "rec-a", entity.ObservedDNSOnly},
		{"el A de muelle sin proxy", []Record{a("rec-a", false)}, "rec-a", entity.ObservedDNSOnly},
		{"sin registros", nil, "rec-a", entity.ObservedMissing},
		{"solo un TXT", []Record{txt}, "", entity.ObservedMissing},
		{"con proxy pero sin el A de muelle", []Record{a("otro", true)}, "rec-a", entity.ObservedMissing},
		{"el panel con un CNAME ajeno con proxy", []Record{cname("c", true)}, "", entity.ObservedProxied},
		{"el panel con un A ajeno sin proxy", []Record{a("x", false)}, "", entity.ObservedDNSOnly},
	}
	for _, c := range cases {
		if got := ObserveRecords(c.records, c.managed); got != c.want {
			t.Errorf("%s: %s, esperaba %s", c.name, got, c.want)
		}
	}
}

// Se sirve con Origin CA (§4.1): candidato, observado proxied, con un active
// de ese host y de esa zona que vence en más de 14 días.
func TestOriginServes(t *testing.T) {
	tg := DomainOriginTarget(managedDomain())
	good := activeCert("api.auditfy.cl", "z1", originNow.Add(60*24*time.Hour))
	cases := []struct {
		name   string
		tg     OriginTarget
		h      entity.OriginHost
		active *entity.OriginCert
		want   bool
	}{
		{"todo bien", tg, observed(entity.ObservedProxied), good, true},
		{"sin certificado", tg, observed(entity.ObservedProxied), nil, false},
		{"sin mirar Cloudflare", tg, observed(""), good, false},
		{"DNS only en Cloudflare", tg, observed(entity.ObservedDNSOnly), good, false},
		{"sin el registro", tg, observed(entity.ObservedMissing), good, false},
		{"otra zona", tg, observed(entity.ObservedProxied), activeCert("api.auditfy.cl", "z2", originNow.Add(60*24*time.Hour)), false},
		{"otro hostname", tg, observed(entity.ObservedProxied), activeCert("web.auditfy.cl", "z1", originNow.Add(60*24*time.Hour)), false},
		{"vence en 15 días", tg, observed(entity.ObservedProxied), activeCert("api.auditfy.cl", "z1", originNow.Add(15*24*time.Hour)), true},
		{"vence en 14 días justos", tg, observed(entity.ObservedProxied), activeCert("api.auditfy.cl", "z1", originNow.Add(14*24*time.Hour)), false},
		{"vencido", tg, observed(entity.ObservedProxied), activeCert("api.auditfy.cl", "z1", originNow.Add(-time.Hour)), false},
		{"no es candidato", func() OriginTarget { d := managedDomain(); d.Proxied = false; return DomainOriginTarget(d) }(),
			observed(entity.ObservedProxied), good, false},
	}
	for _, c := range cases {
		if got := OriginServes(c.tg, c.h, c.active, originNow); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}

// Se emite sin un active, con uno de otra zona o de otro hostname, y se
// renueva cuando faltan menos de 30 días (§5.3).
func TestNeedsOriginCert(t *testing.T) {
	tg := DomainOriginTarget(managedDomain())
	cases := []struct {
		name   string
		active *entity.OriginCert
		want   bool
	}{
		{"sin certificado", nil, true},
		{"vence en 60 días", activeCert("api.auditfy.cl", "z1", originNow.Add(60*24*time.Hour)), false},
		{"vence en 31 días", activeCert("api.auditfy.cl", "z1", originNow.Add(31*24*time.Hour)), false},
		{"vence en 29 días", activeCert("api.auditfy.cl", "z1", originNow.Add(29*24*time.Hour)), true},
		{"vencido", activeCert("api.auditfy.cl", "z1", originNow.Add(-time.Hour)), true},
		{"otra zona", activeCert("api.auditfy.cl", "z2", originNow.Add(60*24*time.Hour)), true},
		{"otro hostname", activeCert("web.auditfy.cl", "z1", originNow.Add(60*24*time.Hour)), true},
	}
	for _, c := range cases {
		if got := NeedsOriginCert(tg, c.active, originNow); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}

// Cada fila de §9.1.
func TestClassifyOriginError(t *testing.T) {
	dnsErr := func(status, code int) error {
		return fmt.Errorf("Cloudflare, creando el certificado: %w", &DNSError{Status: status, Code: code, Message: "x"})
	}
	cases := []struct {
		name string
		err  error
		want entity.OriginErrorCode
	}{
		{"sin red", fmt.Errorf("%w: sin respuesta", shared.ErrCloudflareUnavailable), entity.OriginCloudflare},
		{"429", fmt.Errorf("%w: respondió 429", shared.ErrCloudflareUnavailable), entity.OriginCloudflare},
		{"401", dnsErr(401, 1000), entity.OriginPermission},
		{"403", dnsErr(403, 9109), entity.OriginPermission},
		{"10000", dnsErr(400, 10000), entity.OriginPermission},
		{"1010", dnsErr(400, 1010), entity.OriginPermission},
		{"1100", dnsErr(400, 1100), entity.OriginCloudflare},
		{"1101", dnsErr(400, 1101), entity.OriginCloudflare},
		{"1200", dnsErr(400, 1200), entity.OriginCloudflare},
		{"1201", dnsErr(400, 1201), entity.OriginCloudflare},
		{"1011 (CSR)", dnsErr(400, 1011), entity.OriginRejected},
		{"1007", dnsErr(400, 1007), entity.OriginRejected},
		{"404", fmt.Errorf("%w: %w", ErrDNSNotFound, &DNSError{Status: 404, Code: 7003, Message: "x"}), entity.OriginRejected},
		{"un certificado que no sirve", &OriginCertError{Reason: "no es para api.auditfy.cl"}, entity.OriginRejected},
		{"otro error", errors.New("algo raro"), entity.OriginCloudflare},
	}
	for _, c := range cases {
		if got := ClassifyOriginError(c.err); got != c.want {
			t.Errorf("%s: %s, esperaba %s", c.name, got, c.want)
		}
	}
}

// 1, 2, 4… min hasta 1 h si Cloudflare no respondió; 6 h fijo si lo rechazó (§9.2).
func TestOriginBackoff(t *testing.T) {
	for n, want := range map[int]time.Duration{1: time.Minute, 2: 2 * time.Minute, 3: 4 * time.Minute, 6: 32 * time.Minute,
		7: time.Hour, 8: time.Hour, 100: time.Hour, 0: time.Minute} {
		if got := OriginBackoff(n, entity.OriginCloudflare); got != want {
			t.Errorf("cloudflare, %d fallas: %s, esperaba %s", n, got, want)
		}
	}
	for _, code := range []entity.OriginErrorCode{entity.OriginPermission, entity.OriginRejected} {
		for _, n := range []int{1, 2, 50} {
			if got := OriginBackoff(n, code); got != 6*time.Hour {
				t.Errorf("%s, %d fallas: %s", code, n, got)
			}
		}
	}
}

// La causa (§9.3) según el código, con el mensaje de Cloudflare cortado a
// 300 caracteres. La del panel, que no tiene retry, no nombra Reintentar.
func TestOriginFailureCause(t *testing.T) {
	dom := DomainOriginTarget(managedDomain())
	panel := PanelOriginTarget("panel.auditfy.cl", "z1")
	perm := fmt.Errorf("x: %w", &DNSError{Status: 400, Code: 10000, Message: "Authentication error"})
	if got, want := OriginFailureCause(perm, dom), "el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona de api.auditfy.cl (Cloudflare: Authentication error); agrégalo al token y pulsa Reintentar"; got != want {
		t.Errorf("permission:\n%s\n%s", got, want)
	}
	if got, want := OriginFailureCause(perm, panel), "el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona de panel.auditfy.cl (Cloudflare: Authentication error); agrégalo al token: muelle lo reintenta solo cada 6 h"; got != want {
		t.Errorf("permission en el panel:\n%s\n%s", got, want)
	}
	down := fmt.Errorf("Cloudflare, creando el certificado: %w: respondió 503", shared.ErrCloudflareUnavailable)
	if got, want := OriginFailureCause(down, dom), "Cloudflare no respondió (HTTP 503); se reintenta solo"; got != want {
		t.Errorf("cloudflare:\n%s\n%s", got, want)
	}
	limited := fmt.Errorf("x: %w: respondió 429: demasiadas peticiones, reintenta en unos minutos", shared.ErrCloudflareUnavailable)
	if got := OriginFailureCause(limited, panel); got != "Cloudflare no respondió (HTTP 429: demasiadas peticiones, reintenta en unos minutos); se reintenta solo" {
		t.Errorf("429: %s", got)
	}
	noNet := fmt.Errorf("x: %w: sin respuesta: dial tcp: i/o timeout", shared.ErrCloudflareUnavailable)
	if got := OriginFailureCause(noNet, dom); got != "Cloudflare no respondió (sin respuesta: dial tcp: i/o timeout); se reintenta solo" {
		t.Errorf("sin red: %s", got)
	}
	rej := fmt.Errorf("x: %w", &DNSError{Status: 400, Code: 1011, Message: "Failed to read CSR"})
	if got := OriginFailureCause(rej, dom); got != "Cloudflare lo rechazó (Failed to read CSR)" {
		t.Errorf("rejected: %s", got)
	}
	bad := &OriginCertError{Reason: "el certificado no es para api.auditfy.cl"}
	if got := OriginFailureCause(bad, dom); got != "Cloudflare devolvió un certificado que no sirve (el certificado no es para api.auditfy.cl)" {
		t.Errorf("certificado que no sirve: %s", got)
	}
	long := fmt.Errorf("x: %w", &DNSError{Status: 400, Code: 1011, Message: strings.Repeat("ñ", 400)})
	if got := OriginFailureCause(long, dom); strings.Count(got, "ñ") != 300 || !strings.Contains(got, "…") {
		t.Errorf("el mensaje de Cloudflare se corta a 300: %d ñ en %q", strings.Count(got, "ñ"), got)
	}
}

// El status de origin_ca en el orden de §10.1, con error_code, error y
// retry_at solo con failed y con active (una renovación que falló).
func TestOriginCAView(t *testing.T) {
	dom := DomainOriginTarget(managedDomain())
	exp := time.Date(2027, 1, 19, 12, 0, 0, 0, time.UTC)
	now := exp.Add(-60 * 24 * time.Hour)
	good := activeCert("api.auditfy.cl", "z1", exp)
	retry := now.Add(time.Minute)
	failed := entity.OriginHost{Hostname: "api.auditfy.cl", Observed: entity.ObservedProxied, Failures: 2, ErrorCode: entity.OriginCloudflare,
		LastError: "Cloudflare no respondió (HTTP 503); se reintenta solo", NextAttemptAt: &retry}
	notProxied := func() OriginTarget { d := managedDomain(); d.Proxied = false; return DomainOriginTarget(d) }()
	notManaged := func() OriginTarget { d := managedDomain(); d.CFRecordID = ""; return DomainOriginTarget(d) }()

	if v := OriginCAView(false, dom, good, observed(entity.ObservedProxied), now); v != nil {
		t.Fatalf("1. apagada: %+v", v)
	}
	cases := []struct {
		name   string
		tg     OriginTarget
		active *entity.OriginCert
		h      entity.OriginHost
		want   entity.OriginStatus
		errs   bool
	}{
		{"2. no es elegible", notManaged, good, failed, entity.OriginStatusNotEligible, false},
		{"2. el panel sin zona", PanelOriginTarget("panel.auditfy.cl", ""), nil, entity.OriginHost{}, entity.OriginStatusNotEligible, false},
		{"3. proxied false en muelle", notProxied, good, failed, entity.OriginStatusDNSOnly, false},
		{"4. DNS only en Cloudflare", dom, good, func() entity.OriginHost { h := failed; h.Observed = entity.ObservedDNSOnly; return h }(), entity.OriginStatusDNSOnly, false},
		{"5. sin el registro", dom, good, observed(entity.ObservedMissing), entity.OriginStatusNotEligible, false},
		{"5. lectura rechazada", dom, nil, func() entity.OriginHost { h := failed; h.Observed = entity.ObservedUnreadable; return h }(), entity.OriginStatusNotEligible, false},
		{"6. se sirve", dom, good, observed(entity.ObservedProxied), entity.OriginStatusActive, false},
		{"6. se sirve, falló la renovación", dom, good, failed, entity.OriginStatusActive, true},
		{"7. falló la emisión", dom, nil, failed, entity.OriginStatusFailed, true},
		{"7. falló, y el que hay vence en menos de 14 días", dom, activeCert("api.auditfy.cl", "z1", now.Add(10*24*time.Hour)), failed, entity.OriginStatusFailed, true},
		{"8. emitiendo", dom, nil, observed(entity.ObservedProxied), entity.OriginStatusIssuing, false},
		{"8. sin mirar Cloudflare", dom, nil, entity.OriginHost{}, entity.OriginStatusIssuing, false},
		{"8. sin mirar, con un certificado", dom, good, entity.OriginHost{}, entity.OriginStatusIssuing, false},
	}
	for _, c := range cases {
		v := OriginCAView(true, c.tg, c.active, c.h, now)
		if v == nil || v.Status != c.want {
			t.Errorf("%s: %+v", c.name, v)
			continue
		}
		if c.errs != (v.ErrorCode != "") || c.errs != (v.Error != "") || c.errs != (v.RetryAt != nil) {
			t.Errorf("%s: los campos de la falla: %+v", c.name, v)
		}
		if c.errs && (v.ErrorCode != entity.OriginCloudflare || !v.RetryAt.Equal(retry)) {
			t.Errorf("%s: %+v", c.name, v)
		}
		if (c.active != nil) != (v.ExpiresAt != nil) || (c.active != nil && !v.ExpiresAt.Equal(c.active.ExpiresAt)) {
			t.Errorf("%s: expires_at %v", c.name, v.ExpiresAt)
		}
		wantSource := "letsencrypt"
		if c.want == entity.OriginStatusActive {
			wantSource = "cloudflare_origin_ca"
		}
		if got := CertSource(v); got != wantSource {
			t.Errorf("%s: cert_source %s", c.name, got)
		}
	}
	if CertSource(nil) != "letsencrypt" {
		t.Error("cert_source con origin_ca null")
	}
}

// La plantilla sigue al status (§9.3): con active va la de renovación, con
// las fechas, y con failed la de emisión, también cuando lo que falló fue
// una renovación y al actual le quedan menos de 14 días.
func TestOriginCAViewMessages(t *testing.T) {
	dom := DomainOriginTarget(managedDomain())
	exp := time.Date(2027, 1, 19, 12, 0, 0, 0, time.UTC)
	retry := exp.Add(-30 * 24 * time.Hour)
	h := entity.OriginHost{Hostname: "api.auditfy.cl", Observed: entity.ObservedProxied, Failures: 1, ErrorCode: entity.OriginCloudflare,
		LastError: "Cloudflare no respondió (HTTP 503); se reintenta solo", NextAttemptAt: &retry}

	v := OriginCAView(true, dom, activeCert("api.auditfy.cl", "z1", exp), h, exp.Add(-30*24*time.Hour))
	if want := "muelle no pudo renovar el certificado de Cloudflare, que vence el 19 ene 2027: Cloudflare no respondió (HTTP 503); se reintenta solo; si no lo renueva antes del 5 ene 2027, vuelve a Let's Encrypt"; v.Error != want {
		t.Errorf("renovación:\n%s\n%s", v.Error, want)
	}
	v = OriginCAView(true, dom, activeCert("api.auditfy.cl", "z1", exp), h, exp.Add(-10*24*time.Hour))
	if want := "muelle no pudo emitir el certificado de Cloudflare: Cloudflare no respondió (HTTP 503); se reintenta solo; mientras tanto usa Let's Encrypt"; v.Status != entity.OriginStatusFailed || v.Error != want {
		t.Errorf("a menos de 14 días:\n%s\n%s", v.Error, want)
	}
	h.ErrorCode = entity.OriginPermission
	h.LastError = OriginFailureCause(&DNSError{Status: 400, Code: 10000, Message: "Authentication error"}, dom)
	v = OriginCAView(true, dom, nil, h, exp)
	if want := "muelle no pudo emitir el certificado de Cloudflare: el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona de api.auditfy.cl (Cloudflare: Authentication error); agrégalo al token y pulsa Reintentar; mientras tanto usa Let's Encrypt"; v.Error != want {
		t.Errorf("emisión (el ejemplo del contrato §3.7):\n%s\n%s", v.Error, want)
	}
}

// Las zonas que faltan, ordenadas por nombre (contrato §4.1).
func TestZonesPhrase(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want string
	}{
		{[]string{"a.cl"}, "en la zona a.cl"},
		{[]string{"b.cl", "a.cl"}, "en las zonas a.cl y b.cl"},
		{[]string{"c.cl", "a.cl", "b.cl"}, "en las zonas a.cl, b.cl y 1 más"},
		{[]string{"e.cl", "d.cl", "c.cl", "b.cl", "a.cl"}, "en las zonas a.cl, b.cl y 3 más"},
	} {
		if got := ZonesPhrase(c.in); got != c.want {
			t.Errorf("%v: %q, esperaba %q", c.in, got, c.want)
		}
	}
}

// OriginPEM lleva la clave en claro: ninguna de sus vistas la muestra.
func TestOriginPEMHidesKey(t *testing.T) {
	p := OriginPEM{CertPEM: "CERT", KeyPEM: "-----BEGIN PRIVATE KEY-----\nSECRETO\n-----END PRIVATE KEY-----\n"}
	for _, s := range []string{p.String(), fmt.Sprintf("%+v", p), fmt.Sprintf("%#v", p), fmt.Sprint(&p)} {
		if strings.Contains(s, "SECRETO") || !strings.Contains(s, "CERT") {
			t.Errorf("%s", s)
		}
	}
	b, err := p.MarshalJSON()
	if err != nil || strings.Contains(string(b), "SECRETO") {
		t.Errorf("%s %v", b, err)
	}
}
