package entity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func fullProxySettings() ProxySettings {
	return ProxySettings{
		CloudflareToken:   "cf-token",
		PublicIP:          "203.0.113.7",
		PublicIPv6:        "2001:db8::7",
		ACMEEmail:         "tu@correo.cl",
		PanelDomain:       "panel.tudominio.cl",
		PanelAllowedCIDRs: []string{"203.0.113.0/24", "2001:db8::/32"},
		HTTPPort:          8080,
		HTTPSPort:         8443,
		CaddyImage:        "ghcr.io/cpu-commits/muelle-caddy:v0.1.0",
	}
}

// Lo que admin guarda es exactamente lo que ingress lee.
func TestProxySettingsRoundTrip(t *testing.T) {
	p := fullProxySettings()
	raw := p.Encode()
	if _, ok := raw[SettingCloudflareToken]; ok {
		t.Fatal("Encode no incluye el token: va cifrado aparte")
	}
	want := map[string]string{
		SettingPublicIP:          "203.0.113.7",
		SettingPublicIPv6:        "2001:db8::7",
		SettingACMEEmail:         "tu@correo.cl",
		SettingPanelDomain:       "panel.tudominio.cl",
		SettingPanelAllowedCIDRs: `["203.0.113.0/24","2001:db8::/32"]`,
		SettingProxyHTTPPort:     "8080",
		SettingProxyHTTPSPort:    "8443",
		SettingCaddyImage:        "ghcr.io/cpu-commits/muelle-caddy:v0.1.0",
	}
	if !maps.Equal(raw, want) {
		t.Fatalf("Encode = %v\nesperaba   %v", raw, want)
	}
	if got := slices.Sorted(maps.Keys(raw)); !slices.Equal(got, slices.Sorted(slices.Values(ProxySettingKeys))) {
		t.Fatalf("ProxySettingKeys no coincide con lo que guarda Encode: %v", got)
	}
	back, err := DecodeProxySettings(raw, "cf-token")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, p) {
		t.Fatalf("ida y vuelta:\n%+v\n%+v", back, p)
	}
}

// Lo que no está guardado toma su valor por defecto: 80/443 y sin restricción de IPs.
func TestProxySettingsDefaults(t *testing.T) {
	p, err := DecodeProxySettings(map[string]string{}, "")
	if err != nil {
		t.Fatal(err)
	}
	want := DefaultProxySettings()
	if !reflect.DeepEqual(p, want) {
		t.Fatalf("por defecto = %+v, esperaba %+v", p, want)
	}
	if p.HTTPPort != 80 || p.HTTPSPort != 443 || p.PanelAllowedCIDRs == nil || len(p.PanelAllowedCIDRs) != 0 {
		t.Fatalf("por defecto: %+v", p)
	}
	// Una lista vacía se guarda como [] y vuelve vacía (no nil).
	if got := p.Encode()[SettingPanelAllowedCIDRs]; got != "[]" {
		t.Fatalf("lista vacía guardada como %q", got)
	}
	// Las claves vacías también son "sin valor".
	p, err = DecodeProxySettings(map[string]string{SettingProxyHTTPPort: "", SettingPanelAllowedCIDRs: ""}, "")
	if err != nil || p.HTTPPort != 80 || len(p.PanelAllowedCIDRs) != 0 {
		t.Fatalf("claves vacías: %+v %v", p, err)
	}
}

func TestDecodeProxySettingsRejectsGarbage(t *testing.T) {
	for key, value := range map[string]string{
		SettingProxyHTTPPort:     "ochenta",
		SettingProxyHTTPSPort:    "70000",
		SettingPanelAllowedCIDRs: "10.0.0.0/8",
	} {
		_, err := DecodeProxySettings(map[string]string{key: value}, "")
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s=%q: el error debe nombrar la clave: %v", key, value, err)
		}
	}
}

func TestProxySettingsChangedKeys(t *testing.T) {
	old := fullProxySettings()
	if got := old.ChangedKeys(old); len(got) != 0 {
		t.Fatalf("sin cambios: %v", got)
	}
	next := old
	next.CloudflareToken = "otro"
	next.HTTPPort = 80
	next.PanelAllowedCIDRs = []string{"203.0.113.0/24"}
	got := next.ChangedKeys(old)
	want := []string{SettingCloudflareToken, SettingPanelAllowedCIDRs, SettingProxyHTTPPort}
	if !slices.Equal(got, want) {
		t.Fatalf("ChangedKeys = %v, esperaba %v (ordenadas)", got, want)
	}
	// Mismos rangos en otro slice no es un cambio.
	same := old
	same.PanelAllowedCIDRs = slices.Clone(old.PanelAllowedCIDRs)
	if got := same.ChangedKeys(old); len(got) != 0 {
		t.Fatalf("misma lista: %v", got)
	}
}

// ingress maneja muelle-caddy solo con el dominio del panel y una imagen de
// Caddy: la fijada en caddy_image o la de esta versión de muelle. En
// desarrollo (un origen con IP y un binario sin versión) no toca los puertos del host.
func TestProxySettingsEnabled(t *testing.T) {
	const def = "ghcr.io/cpu-commits/muelle-caddy:v0.2.0"
	p := fullProxySettings()
	if !p.Enabled("") || p.CaddyImageOr(def) != p.CaddyImage {
		t.Fatal("con dominio e imagen fijada debe estar activo y usar la fijada")
	}
	p.CaddyImage = ""
	if !p.Enabled(def) || p.CaddyImageOr(def) != def {
		t.Fatal("sin imagen fijada usa la de la versión")
	}
	if p.Enabled("") {
		t.Fatal("sin imagen fijada ni de versión no está activo")
	}
	p = fullProxySettings()
	p.PanelDomain = ""
	if p.Enabled(def) {
		t.Fatal("sin dominio del panel no está activo")
	}
}

// El token de Cloudflare nunca sale en un %v, en un log de slog (main usa el
// JSONHandler, que serializa los structs con encoding/json e ignora String) ni
// en un JSON, ni solo ni dentro de otro valor: admin e ingress se pasan
// ProxySettings todo el tiempo (ProxySettingsChanged(old, next)).
func TestProxySettingsDoesNotPrintToken(t *testing.T) {
	const token = "cf-token-1234567890abcdef"
	p := fullProxySettings()
	p.CloudflareToken = token
	nested := struct {
		Next ProxySettings
		Old  *ProxySettings
	}{p, &p}
	var out []string
	for _, v := range []any{p, &p, nested, &nested} {
		out = append(out, fmt.Sprintf("%v", v), fmt.Sprintf("%+v", v), fmt.Sprintf("%#v", v), fmt.Sprint(v))
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(b))
	}
	var buf bytes.Buffer
	for _, h := range []slog.Handler{slog.NewJSONHandler(&buf, nil), slog.NewTextHandler(&buf, nil)} {
		slog.New(h).Info("ajustes", "next", p, "ptr", &p, "nested", nested, slog.Group("g", "p", p))
	}
	out = append(out, buf.String())
	for _, o := range out {
		if strings.Contains(o, token) {
			t.Fatalf("filtra el token: %s", o)
		}
	}
	// Se esconde solo el token, y solo si hay.
	if s := p.String(); !strings.Contains(s, "203.0.113.7") || !strings.Contains(s, "***") {
		t.Fatalf("String esconde solo el token: %s", s)
	}
	if !strings.Contains(buf.String(), "panel.tudominio.cl") {
		t.Fatalf("el log no trae el resto de los ajustes: %s", buf.String())
	}
	if s := DefaultProxySettings().String(); strings.Contains(s, "***") {
		t.Fatalf("sin token no hay nada que esconder: %s", s)
	}
}

// PanelOrigin es el origen en que muelle-caddy sirve el panel con estos
// ajustes: el dominio del panel en el puerto HTTPS del proxy.
func TestProxySettingsPanelOrigin(t *testing.T) {
	p := DefaultProxySettings()
	if got := p.PanelOrigin(); got != "" {
		t.Fatalf("sin dominio del panel: %q", got)
	}
	p.PanelDomain = "panel.tudominio.cl"
	if got := p.PanelOrigin(); got != "https://panel.tudominio.cl" {
		t.Fatalf("puerto 443: %q", got)
	}
	p.HTTPSPort = 8443
	if got := p.PanelOrigin(); got != "https://panel.tudominio.cl:8443" {
		t.Fatalf("puerto 8443: %q", got)
	}
}
