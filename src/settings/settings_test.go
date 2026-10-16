package settings

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// clearEnv aísla el test del entorno del que lo ejecuta (un valor vacío cuenta como no definido).
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"MUELLE_LISTEN", "MUELLE_DATA_DIR", "MUELLE_MASTER_KEY_FILE", "MUELLE_PANEL_ORIGIN", "MUELLE_SECURE_COOKIES",
		"MUELLE_INIT_PUBLIC_IP", "MUELLE_INIT_PUBLIC_IPV6", "MUELLE_INIT_ACME_EMAIL", "MUELLE_INIT_PROXY_HTTP_PORT",
		"MUELLE_INIT_PROXY_HTTPS_PORT", "MUELLE_INIT_CLOUDFLARE_TOKEN_FILE", "MUELLE_INIT_CADDY_IMAGE"} {
		t.Setenv(k, "")
	}
	t.Setenv("MUELLE_PANEL_DOMAIN", "")
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("MUELLE_PANEL_ORIGIN", "https://panel.example.com")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != ":8080" || c.DataDir != "/data" || c.MasterKeyFile != "/run/secrets/muelle_master_key" {
		t.Fatalf("defaults incorrectos: %+v", c)
	}
	if !c.SecureCookies {
		t.Fatal("SecureCookies debe ser true por defecto")
	}
}

func TestLoadOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("MUELLE_PANEL_ORIGIN", "http://localhost:8080")
	t.Setenv("MUELLE_LISTEN", "127.0.0.1:9000")
	t.Setenv("MUELLE_DATA_DIR", "/tmp/m")
	t.Setenv("MUELLE_MASTER_KEY_FILE", "/tmp/key")
	t.Setenv("MUELLE_SECURE_COOKIES", "false")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != "127.0.0.1:9000" || c.DataDir != "/tmp/m" || c.MasterKeyFile != "/tmp/key" || c.SecureCookies {
		t.Fatalf("overrides incorrectos: %+v", c)
	}
}

func TestLoadRejectsBadOrigin(t *testing.T) {
	clearEnv(t)
	bad := []string{
		"", "panel.example.com", "ftp://x.com", "https://x.com/", "https://x.com/panel",
		"HTTPS://panel.example.com", "https://Panel.Example.com", "https://x.com:443", "http://x.com:80",
		"https://x.com:", "https://x.com:0443", "https://x.com:99999", "https://x.com:0",
		"https://x.com?", "https://x.com#", "https://panél.example.com", "https://*.example.com", "null",
		"https://u@x.com", "https://x.com?a=1", "https://x.com#f", "https:x.com",
		// IP literales no canónicas o no soportadas.
		"https://[0:0:0:0:0:0:0:1]", "https://[2001:0db8::1]", "https://[::ffff:127.0.0.1]",
		"https://[fe80::1%25eth0]", "https://[127.0.0.1]",
		"http://127.1:8080", "http://0x7f.0.0.1", "http://0177.0.0.1", "http://2130706433", "http://256.0.0.1",
	}
	for _, origin := range bad {
		t.Setenv("MUELLE_PANEL_ORIGIN", origin)
		_, err := Load()
		if err == nil {
			t.Errorf("origen %q debió fallar", origin)
			continue
		}
		if !strings.HasPrefix(err.Error(), "MUELLE_PANEL_ORIGIN:") {
			t.Errorf("origen %q: el error no menciona la variable: %v", origin, err)
		}
	}
}

func TestLoadAcceptsGoodOrigin(t *testing.T) {
	clearEnv(t)
	for _, origin := range []string{
		"https://[::1]:8080", "http://localhost:8080", "https://panel.example.com:8443",
		"http://127.0.0.1:18080", "https://panel.example.com", "https://[2001:db8::1]",
	} {
		t.Setenv("MUELLE_PANEL_ORIGIN", origin)
		t.Setenv("MUELLE_SECURE_COOKIES", "true")
		c, err := Load()
		if err != nil {
			t.Errorf("origen %q debió pasar: %v", origin, err)
			continue
		}
		if c.PanelOrigin != origin {
			t.Errorf("origen %q alterado: %q", origin, c.PanelOrigin)
		}
	}
}

func TestLoadSuggestsCanonicalOrigin(t *testing.T) {
	clearEnv(t)
	cases := map[string]string{
		"https://Panel.Example.com": "https://panel.example.com",
		"HTTPS://panel.example.com": "https://panel.example.com",
		"https://x.com:443":         "https://x.com",
		"http://x.com:80":           "http://x.com",
		"https://[0:0:0:0:0:0:0:1]": "https://[::1]",
		"https://[2001:0db8::1]":    "https://[2001:db8::1]",
	}
	for in, want := range cases {
		t.Setenv("MUELLE_PANEL_ORIGIN", in)
		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("origen %q: el error debe sugerir %q, obtuvo %v", in, want, err)
		}
	}
}

func TestLoadRejectsNonCanonicalIPv4(t *testing.T) {
	clearEnv(t)
	for _, origin := range []string{
		"http://127.1:8080", "http://0x7f.0.0.1", "http://0177.0.0.1", "http://2130706433", "http://256.0.0.1",
	} {
		t.Setenv("MUELLE_PANEL_ORIGIN", origin)
		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "decimal con puntos") {
			t.Errorf("origen %q: esperaba el error de IP v4, obtuvo %v", origin, err)
		}
	}
}

func TestLoadRejectsHTTPSWithInsecureCookies(t *testing.T) {
	clearEnv(t)
	t.Setenv("MUELLE_PANEL_ORIGIN", "https://panel.example.com")
	t.Setenv("MUELLE_SECURE_COOKIES", "false")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "MUELLE_SECURE_COOKIES") {
		t.Fatalf("debió fallar mencionando MUELLE_SECURE_COOKIES: %v", err)
	}
}

func TestLoadAcceptsHTTPWithInsecureCookies(t *testing.T) {
	clearEnv(t)
	t.Setenv("MUELLE_PANEL_ORIGIN", "http://localhost:8080")
	t.Setenv("MUELLE_SECURE_COOKIES", "false")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.SecureCookies {
		t.Fatal("SecureCookies debe ser false")
	}
}

func TestLoadRejectsBadBool(t *testing.T) {
	clearEnv(t)
	t.Setenv("MUELLE_PANEL_ORIGIN", "https://panel.example.com")
	t.Setenv("MUELLE_SECURE_COOKIES", "quizás")
	if _, err := Load(); err == nil {
		t.Fatal("debió fallar")
	}
}

// La configuración inicial de install (MUELLE_INIT_*, Fase 5) se valida al
// arrancar: un valor que no sirve impide arrancar, con el nombre de la variable.
func TestLoadInit(t *testing.T) {
	clearEnv(t)
	t.Setenv("MUELLE_PANEL_ORIGIN", "https://panel.tudominio.cl:8443")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Init, Init{}) {
		t.Fatalf("sin MUELLE_INIT_*: %+v", c.Init)
	}

	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("cf-token-1234567890abcdef\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MUELLE_INIT_PUBLIC_IP", "203.0.113.7")
	t.Setenv("MUELLE_INIT_PUBLIC_IPV6", "2001:db8::7")
	t.Setenv("MUELLE_INIT_ACME_EMAIL", "tu@correo.cl")
	t.Setenv("MUELLE_INIT_PROXY_HTTP_PORT", "8080")
	t.Setenv("MUELLE_INIT_PROXY_HTTPS_PORT", "8443")
	t.Setenv("MUELLE_INIT_CLOUDFLARE_TOKEN_FILE", tokenFile)
	t.Setenv("MUELLE_INIT_CADDY_IMAGE", "ghcr.io/cpu-commits/muelle-caddy:v0.1.0")
	c, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	want := Init{
		PublicIP: "203.0.113.7", PublicIPv6: "2001:db8::7", ACMEEmail: "tu@correo.cl",
		ProxyHTTPPort: 8080, ProxyHTTPSPort: 8443, CloudflareToken: "cf-token-1234567890abcdef",
		CaddyImage: "ghcr.io/cpu-commits/muelle-caddy:v0.1.0",
	}
	if !reflect.DeepEqual(c.Init, want) {
		t.Fatalf("Init=%+v\nesperaba %+v", c.Init, want)
	}
}

func TestLoadRejectsBadInit(t *testing.T) {
	for key, value := range map[string]string{
		"MUELLE_INIT_PUBLIC_IP":             "203.0.113",
		"MUELLE_INIT_PUBLIC_IPV6":           "203.0.113.7",
		"MUELLE_INIT_ACME_EMAIL":            "sin-arroba",
		"MUELLE_INIT_PROXY_HTTP_PORT":       "ochenta",
		"MUELLE_INIT_PROXY_HTTPS_PORT":      "70000",
		"MUELLE_INIT_CLOUDFLARE_TOKEN_FILE": "/no/existe/token",
		"MUELLE_INIT_CADDY_IMAGE":           "-mala",
	} {
		clearEnv(t)
		t.Setenv("MUELLE_PANEL_ORIGIN", "https://panel.tudominio.cl")
		t.Setenv(key, value)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s=%q: el error debe nombrar la variable: %v", key, value, err)
		}
	}
	// Los puertos del proxy tienen que ser distintos (con los por defecto, 80 y 443).
	clearEnv(t)
	t.Setenv("MUELLE_PANEL_ORIGIN", "https://panel.tudominio.cl")
	t.Setenv("MUELLE_INIT_PROXY_HTTP_PORT", "443")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "MUELLE_INIT_PROXY_HTTPS_PORT") {
		t.Errorf("puertos iguales: %v", err)
	}
	// Un archivo de token vacío es un error, no "sin token".
	empty := filepath.Join(t.TempDir(), "vacio")
	os.WriteFile(empty, []byte("\n"), 0o600)
	clearEnv(t)
	t.Setenv("MUELLE_PANEL_ORIGIN", "https://panel.tudominio.cl")
	t.Setenv("MUELLE_INIT_CLOUDFLARE_TOKEN_FILE", empty)
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "MUELLE_INIT_CLOUDFLARE_TOKEN_FILE") {
		t.Errorf("token vacío: %v", err)
	}
}

// panel_domain sale del origen: el host si es un dominio, "" si es una IP o
// localhost (desarrollo, sin proxy).
func TestPanelDomain(t *testing.T) {
	for origin, want := range map[string]string{
		"https://panel.tudominio.cl":      "panel.tudominio.cl",
		"https://panel.tudominio.cl:8443": "panel.tudominio.cl",
		"http://127.0.0.1:18080":          "",
		"http://localhost:8080":           "",
		"https://[2001:db8::1]":           "",
	} {
		if got := (Settings{PanelOrigin: origin}).PanelDomain(); got != want {
			t.Errorf("PanelDomain(%q) = %q, esperaba %q", origin, got, want)
		}
	}
}

// El token de Cloudflare de MUELLE_INIT_CLOUDFLARE_TOKEN_FILE nunca sale en
// un %v ni en un log, ni solo ni dentro de Settings.
func TestInitDoesNotPrintToken(t *testing.T) {
	const token = "cf-token-1234567890abcdef"
	s := Settings{PanelOrigin: "https://panel.tudominio.cl", Init: Init{PublicIP: "203.0.113.7", CloudflareToken: token}}
	var out []string
	for _, v := range []any{s.Init, &s.Init, s, &s} {
		out = append(out, fmt.Sprintf("%v", v), fmt.Sprintf("%+v", v), fmt.Sprintf("%#v", v), fmt.Sprint(v))
	}
	var buf bytes.Buffer
	for _, h := range []slog.Handler{slog.NewJSONHandler(&buf, nil), slog.NewTextHandler(&buf, nil)} {
		slog.New(h).Info("arranque", "init", s.Init, "settings", s)
	}
	out = append(out, buf.String())
	for _, o := range out {
		if strings.Contains(o, token) {
			t.Fatalf("filtra el token: %s", o)
		}
	}
	if !strings.Contains(s.Init.String(), "203.0.113.7") || !strings.Contains(s.Init.String(), "***") {
		t.Fatalf("String esconde solo el token: %s", s.Init.String())
	}
}
