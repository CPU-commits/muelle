package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

const testToken = "cf-token-1234567890abcdef"

// testApps son una app de formulario y una compose del mismo stage.
var testApps = map[int64]App{
	1: {ID: 1, Project: "auditfy", Stage: "staging", Slug: "api", Kind: KindService},
	2: {ID: 2, Project: "auditfy", Stage: "staging", Slug: "infra", Kind: KindCompose},
}

func testSettings(token string) shared.ProxySettings {
	p := shared.DefaultProxySettings()
	p.PanelDomain = "panel.auditfy.cl"
	p.ACMEEmail = "ops@auditfy.cl"
	p.CloudflareToken = token
	return p
}

func dom(appID int64, host, svc string, port int, tls entity.TLSMode, st entity.Status) entity.Domain {
	d := entity.Domain{AppID: appID, Hostname: host, TargetService: svc, TargetPort: port, TLSMode: tls, Status: st}
	if tls == entity.TLSCloudflare {
		d.CFZoneID = "z1"
	}
	return d
}

func TestBuildProxyConfigRoutes(t *testing.T) {
	domains := []entity.Domain{
		dom(2, "grafana.auditfy.cl", "grafana", 3000, entity.TLSHTTP, entity.StatusActive),
		dom(1, "api.auditfy.cl", "api", 3000, entity.TLSCloudflare, entity.StatusPending),
		dom(2, "caido.auditfy.cl", "web", 8080, entity.TLSCloudflare, entity.StatusError),
	}
	cfg := BuildProxyConfig(testSettings(testToken), "", domains, testApps, false, OriginInputs{})
	want := []Route{
		{Hostname: "api.auditfy.cl", Upstream: "auditfy_staging_api_api:3000", TLS: entity.TLSCloudflare},
		{Hostname: "caido.auditfy.cl", Upstream: "auditfy_staging_infra_web:8080", TLS: entity.TLSCloudflare},
		{Hostname: "grafana.auditfy.cl", Upstream: "auditfy_staging_infra_grafana:3000", TLS: entity.TLSHTTP},
	}
	if !reflect.DeepEqual(cfg.Routes, want) {
		t.Fatalf("Routes:\n%+v\nesperaba\n%+v", cfg.Routes, want)
	}
	if cfg.ACMEEmail != "ops@auditfy.cl" || cfg.CloudflareToken != testToken || cfg.InternalCA {
		t.Fatalf("cfg: %v", cfg)
	}
	if internal := BuildProxyConfig(testSettings(""), "", nil, testApps, true, OriginInputs{}); !internal.InternalCA || internal.Routes == nil {
		t.Fatalf("InternalCA y Routes nunca nil: %v", internal)
	}
}

func TestBuildProxyConfigPanel(t *testing.T) {
	p := testSettings("")
	p.PanelAllowedCIDRs = []string{"190.5.6.0/24", "2001:db8::/32"}
	cfg := BuildProxyConfig(p, "", nil, testApps, false, OriginInputs{})
	want := &PanelRoute{Hostname: "panel.auditfy.cl", Upstream: "muelle_muelle:8080",
		AllowedCIDRs: []string{"190.5.6.0/24", "2001:db8::/32"}, TLS: entity.TLSHTTP}
	if !reflect.DeepEqual(cfg.Panel, want) {
		t.Fatalf("Panel: %+v", cfg.Panel)
	}
	// La lista es una copia: cambiar los ajustes después no cambia la configuración.
	p.PanelAllowedCIDRs[0] = "10.0.0.0/8"
	if cfg.Panel.AllowedCIDRs[0] != "190.5.6.0/24" {
		t.Fatal("AllowedCIDRs comparte el arreglo de los ajustes")
	}

	// Sin lista: [] (cualquiera), nunca nil.
	if cfg := BuildProxyConfig(testSettings(""), "", nil, testApps, false, OriginInputs{}); cfg.Panel.AllowedCIDRs == nil || len(cfg.Panel.AllowedCIDRs) != 0 {
		t.Fatalf("sin lista: %#v", cfg.Panel.AllowedCIDRs)
	}

	p.PanelDomain = ""
	if cfg := BuildProxyConfig(p, "z1", nil, testApps, false, OriginInputs{}); cfg.Panel != nil {
		t.Fatalf("sin panel_domain no hay ruta del panel: %+v", cfg.Panel)
	}
}

// §3.4: con token, cloudflare si hay zona; sin token, todo va por http.
func TestBuildProxyConfigTLS(t *testing.T) {
	domains := []entity.Domain{
		dom(1, "api.auditfy.cl", "api", 3000, entity.TLSCloudflare, entity.StatusActive),
		dom(1, "otro.ejemplo.cl", "api", 3000, entity.TLSHTTP, entity.StatusActive),
	}
	cfg := BuildProxyConfig(testSettings(testToken), "zpanel", domains, testApps, false, OriginInputs{})
	if cfg.Panel.TLS != entity.TLSCloudflare {
		t.Errorf("con token y zona, el panel va por cloudflare: %s", cfg.Panel.TLS)
	}
	if cfg.Routes[0].TLS != entity.TLSCloudflare || cfg.Routes[1].TLS != entity.TLSHTTP {
		t.Errorf("con token, cada dominio con su modo: %+v", cfg.Routes)
	}

	if cfg := BuildProxyConfig(testSettings(testToken), "", domains, testApps, false, OriginInputs{}); cfg.Panel.TLS != entity.TLSHTTP {
		t.Errorf("con token y sin zona, el panel va por http: %s", cfg.Panel.TLS)
	}

	cfg = BuildProxyConfig(testSettings(""), "zpanel", domains, testApps, false, OriginInputs{})
	if cfg.Panel.TLS != entity.TLSHTTP {
		t.Errorf("sin token, el panel va por http: %s", cfg.Panel.TLS)
	}
	for _, r := range cfg.Routes {
		if r.TLS != entity.TLSHTTP {
			t.Errorf("sin token, %s va por http aunque diga cloudflare: %s", r.Hostname, r.TLS)
		}
	}
}

// Un dominio con el hostname del panel no pisa la ruta del panel, y uno cuya
// app ya no está no tiene upstream.
func TestBuildProxyConfigSkips(t *testing.T) {
	domains := []entity.Domain{
		dom(1, "panel.auditfy.cl", "api", 3000, entity.TLSHTTP, entity.StatusActive),
		dom(99, "huerfano.auditfy.cl", "api", 3000, entity.TLSHTTP, entity.StatusActive),
		dom(1, "api.auditfy.cl", "api", 3000, entity.TLSHTTP, entity.StatusActive),
	}
	cfg := BuildProxyConfig(testSettings(""), "", domains, testApps, false, OriginInputs{})
	if len(cfg.Routes) != 1 || cfg.Routes[0].Hostname != "api.auditfy.cl" {
		t.Fatalf("Routes: %+v", cfg.Routes)
	}
	if cfg.Panel == nil || cfg.Panel.Upstream != "muelle_muelle:8080" {
		t.Fatalf("Panel: %+v", cfg.Panel)
	}
}

func TestProxyConfigHidesToken(t *testing.T) {
	cfg := BuildProxyConfig(testSettings(testToken), "z1",
		[]entity.Domain{dom(1, "api.auditfy.cl", "api", 3000, entity.TLSCloudflare, entity.StatusActive)}, testApps, false, OriginInputs{})
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("x", "cfg", cfg, "ptr", &cfg)
	slog.New(slog.NewTextHandler(&buf, nil)).Info("x", "cfg", cfg)
	j, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	wrapped, err := json.Marshal(struct{ C ProxyConfig }{cfg})
	if err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]string{
		"Sprint": fmt.Sprint(cfg), "%+v": fmt.Sprintf("%+v", cfg), "%#v": fmt.Sprintf("%#v", cfg), "&": fmt.Sprint(&cfg),
		"slog": buf.String(), "json": string(j), "json anidado": string(wrapped),
	} {
		if strings.Contains(s, testToken) {
			t.Errorf("%s muestra el token: %s", name, s)
		}
		if !strings.Contains(s, "***") || !strings.Contains(s, "api.auditfy.cl") {
			t.Errorf("%s debería mostrar el resto, con el token como ***: %s", name, s)
		}
	}
	if cfg.CloudflareToken != testToken {
		t.Fatal("ocultarlo no cambia el valor")
	}
}

const originKey = "-----BEGIN PRIVATE KEY-----\nMIGHAgEAMBMGByqGSM49AgEGClaveSecretaDelOrigen\n-----END PRIVATE KEY-----\n"

// originInputs: un active de host en zone, con clave, que vence en days
// días desde originNow, y lo observado de host.
func originInputs() OriginInputs {
	return OriginInputs{Certs: map[string]entity.OriginCert{}, Hosts: map[string]entity.OriginHost{}, Now: originNow}
}

func (in OriginInputs) with(host, zone string, days int, o entity.Observation) OriginInputs {
	if zone != "" {
		in.Certs[host] = entity.OriginCert{Hostname: host, ZoneID: zone, CFCertID: "cf-" + host, CertPEM: "CERT " + host,
			KeyPEM: originKey, ExpiresAt: originNow.Add(time.Duration(days) * 24 * time.Hour), State: entity.OriginActive}
	}
	in.Hosts[host] = entity.OriginHost{Hostname: host, Observed: o}
	return in
}

// OriginCA en Route y PanelRoute (§4.1 del diseño de Origin CA): solo el
// host que es candidato, se ve con el proxy en Cloudflare y tiene un active
// de su zona que vence en más de 14 días; la misma regla que la API.
func TestBuildProxyConfigOriginCA(t *testing.T) {
	managed := func(host string, proxied bool) entity.Domain {
		d := dom(1, host, "api", 3000, entity.TLSCloudflare, entity.StatusActive)
		d.CFRecordID, d.Proxied = "rec-"+host, proxied
		return d
	}
	ajeno := dom(1, "ajeno.auditfy.cl", "api", 3000, entity.TLSCloudflare, entity.StatusError)
	ajeno.Proxied = true
	domains := []entity.Domain{
		managed("api.auditfy.cl", true),
		managed("web.auditfy.cl", false),
		ajeno,
		managed("gris.auditfy.cl", true),
		managed("zona.auditfy.cl", true),
		managed("pronto.auditfy.cl", true),
		managed("sinver.auditfy.cl", true),
		dom(2, "grafana.ejemplo.com", "grafana", 3000, entity.TLSHTTP, entity.StatusActive),
	}
	in := originInputs().
		with("api.auditfy.cl", "z1", 90, entity.ObservedProxied).
		with("web.auditfy.cl", "z1", 90, entity.ObservedProxied).
		with("ajeno.auditfy.cl", "z1", 90, entity.ObservedProxied).
		with("gris.auditfy.cl", "z1", 90, entity.ObservedDNSOnly).
		with("zona.auditfy.cl", "z9", 90, entity.ObservedProxied).
		with("pronto.auditfy.cl", "z1", 14, entity.ObservedProxied).
		with("sinver.auditfy.cl", "z1", 90, "").
		with("grafana.ejemplo.com", "z1", 90, entity.ObservedProxied).
		with("panel.auditfy.cl", "zpanel", 90, entity.ObservedProxied)
	p := testSettings(testToken)
	p.CloudflareOriginCA = true

	cfg := BuildProxyConfig(p, "zpanel", domains, testApps, false, in)
	served := map[string]bool{}
	for _, r := range cfg.Routes {
		if r.OriginCA != nil {
			served[r.Hostname] = true
			if r.OriginCA.CertPEM != "CERT "+r.Hostname || r.OriginCA.KeyPEM != originKey {
				t.Errorf("%s: %+v", r.Hostname, r.OriginCA)
			}
		}
	}
	if len(served) != 1 || !served["api.auditfy.cl"] {
		t.Fatalf("con Origin CA: %v", served)
	}
	if cfg.Panel.OriginCA == nil || cfg.Panel.OriginCA.CertPEM != "CERT panel.auditfy.cl" {
		t.Fatalf("panel: %+v", cfg.Panel)
	}
	// Sin la zona del panel, el panel no califica.
	if cfg := BuildProxyConfig(p, "", domains, testApps, false, in); cfg.Panel.OriginCA != nil {
		t.Fatalf("panel sin zona: %+v", cfg.Panel)
	}

	// Con la opción apagada, o sin token, nada va con Origin CA.
	off := p
	off.CloudflareOriginCA = false
	noToken := p
	noToken.CloudflareToken = ""
	for name, ps := range map[string]shared.ProxySettings{"apagada": off, "sin token": noToken} {
		cfg := BuildProxyConfig(ps, "zpanel", domains, testApps, false, in)
		if cfg.Panel.OriginCA != nil {
			t.Errorf("%s: el panel", name)
		}
		for _, r := range cfg.Routes {
			if r.OriginCA != nil {
				t.Errorf("%s: %s", name, r.Hostname)
			}
		}
	}
}

// La clave de un certificado de Origin CA nunca sale de ProxyConfig ni de
// OriginPEM en un %v, un log ni un JSON (§5.4 del diseño de Origin CA).
func TestProxyConfigHidesOriginKeys(t *testing.T) {
	d := dom(1, "api.auditfy.cl", "api", 3000, entity.TLSCloudflare, entity.StatusActive)
	d.CFRecordID, d.Proxied = "rec-a", true
	p := testSettings(testToken)
	p.CloudflareOriginCA = true
	in := originInputs().with("api.auditfy.cl", "z1", 90, entity.ObservedProxied).with("panel.auditfy.cl", "zpanel", 90, entity.ObservedProxied)
	cfg := BuildProxyConfig(p, "zpanel", []entity.Domain{d}, testApps, false, in)
	if cfg.Routes[0].OriginCA == nil || cfg.Panel.OriginCA == nil {
		t.Fatalf("sin Origin CA: %v", cfg)
	}
	pem := *cfg.Routes[0].OriginCA
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("x", "cfg", cfg, "ptr", &cfg, "pem", pem, "route", cfg.Routes[0], "panel", cfg.Panel)
	slog.New(slog.NewTextHandler(&buf, nil)).Info("x", "cfg", cfg, "pem", pem)
	out := map[string]string{
		"Sprint": fmt.Sprint(cfg), "%+v": fmt.Sprintf("%+v", cfg), "%#v": fmt.Sprintf("%#v", cfg), "&": fmt.Sprint(&cfg),
		"pem %+v": fmt.Sprintf("%+v", pem), "pem %#v": fmt.Sprintf("%#v", pem), "&pem": fmt.Sprint(&pem),
		"route %+v": fmt.Sprintf("%+v", cfg.Routes[0]), "panel %+v": fmt.Sprintf("%+v", *cfg.Panel), "slog": buf.String(),
	}
	for name, v := range map[string]any{"json": cfg, "json anidado": struct{ C ProxyConfig }{cfg}, "json pem": pem, "json route": cfg.Routes[0]} {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		out[name] = string(b)
	}
	for name, s := range out {
		if strings.Contains(s, "ClaveSecretaDelOrigen") || strings.Contains(s, testToken) {
			t.Errorf("%s muestra la clave o el token: %s", name, s)
		}
		if !strings.Contains(s, "***") || !strings.Contains(s, "CERT") {
			t.Errorf("%s debería mostrar el certificado, con la clave como ***: %s", name, s)
		}
	}
	if cfg.Routes[0].OriginCA.KeyPEM != originKey {
		t.Fatal("ocultarla no cambia el valor")
	}
}
