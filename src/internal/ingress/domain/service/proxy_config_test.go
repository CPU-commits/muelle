package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"

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
	cfg := BuildProxyConfig(testSettings(testToken), "", domains, testApps, false)
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
	if internal := BuildProxyConfig(testSettings(""), "", nil, testApps, true); !internal.InternalCA || internal.Routes == nil {
		t.Fatalf("InternalCA y Routes nunca nil: %v", internal)
	}
}

func TestBuildProxyConfigPanel(t *testing.T) {
	p := testSettings("")
	p.PanelAllowedCIDRs = []string{"190.5.6.0/24", "2001:db8::/32"}
	cfg := BuildProxyConfig(p, "", nil, testApps, false)
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
	if cfg := BuildProxyConfig(testSettings(""), "", nil, testApps, false); cfg.Panel.AllowedCIDRs == nil || len(cfg.Panel.AllowedCIDRs) != 0 {
		t.Fatalf("sin lista: %#v", cfg.Panel.AllowedCIDRs)
	}

	p.PanelDomain = ""
	if cfg := BuildProxyConfig(p, "z1", nil, testApps, false); cfg.Panel != nil {
		t.Fatalf("sin panel_domain no hay ruta del panel: %+v", cfg.Panel)
	}
}

// §3.4: con token, cloudflare si hay zona; sin token, todo va por http.
func TestBuildProxyConfigTLS(t *testing.T) {
	domains := []entity.Domain{
		dom(1, "api.auditfy.cl", "api", 3000, entity.TLSCloudflare, entity.StatusActive),
		dom(1, "otro.ejemplo.cl", "api", 3000, entity.TLSHTTP, entity.StatusActive),
	}
	cfg := BuildProxyConfig(testSettings(testToken), "zpanel", domains, testApps, false)
	if cfg.Panel.TLS != entity.TLSCloudflare {
		t.Errorf("con token y zona, el panel va por cloudflare: %s", cfg.Panel.TLS)
	}
	if cfg.Routes[0].TLS != entity.TLSCloudflare || cfg.Routes[1].TLS != entity.TLSHTTP {
		t.Errorf("con token, cada dominio con su modo: %+v", cfg.Routes)
	}

	if cfg := BuildProxyConfig(testSettings(testToken), "", domains, testApps, false); cfg.Panel.TLS != entity.TLSHTTP {
		t.Errorf("con token y sin zona, el panel va por http: %s", cfg.Panel.TLS)
	}

	cfg = BuildProxyConfig(testSettings(""), "zpanel", domains, testApps, false)
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
	cfg := BuildProxyConfig(testSettings(""), "", domains, testApps, false)
	if len(cfg.Routes) != 1 || cfg.Routes[0].Hostname != "api.auditfy.cl" {
		t.Fatalf("Routes: %+v", cfg.Routes)
	}
	if cfg.Panel == nil || cfg.Panel.Upstream != "muelle_muelle:8080" {
		t.Fatalf("Panel: %+v", cfg.Panel)
	}
}

func TestProxyConfigHidesToken(t *testing.T) {
	cfg := BuildProxyConfig(testSettings(testToken), "z1",
		[]entity.Domain{dom(1, "api.auditfy.cl", "api", 3000, entity.TLSCloudflare, entity.StatusActive)}, testApps, false)
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
