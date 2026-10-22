package settings

import (
	"bytes"
	"encoding/json"
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
		"MUELLE_INIT_PROXY_HTTPS_PORT", "MUELLE_INIT_CLOUDFLARE_TOKEN_FILE", "MUELLE_INIT_CADDY_IMAGE", "MUELLE_INIT_REGISTRY_FILE"} {
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

// { o } en el email ACME o en el token de Cloudflare dejarían a Caddy sin
// configuración (out_of_sync, contrato §3.4), y el panel va por Caddy: muelle
// no arranca con ellos, y el error nombra la variable sin citar el token.
func TestLoadRejectsInitPlaceholders(t *testing.T) {
	const token = "cf-token-{1234567890abcdef}"
	for key, value := range map[string]string{
		"MUELLE_INIT_ACME_EMAIL":            "a{x}@correo.cl",
		"MUELLE_INIT_CLOUDFLARE_TOKEN_FILE": writeFile(t, "token", token+"\n"),
	} {
		clearEnv(t)
		t.Setenv("MUELLE_PANEL_ORIGIN", "https://panel.tudominio.cl")
		t.Setenv(key, value)
		_, err := Load()
		if err == nil || !strings.HasPrefix(err.Error(), key+": ") || !strings.Contains(err.Error(), "Caddy los toma como placeholders") {
			t.Errorf("%s: %v", key, err)
			continue
		}
		if strings.Contains(err.Error(), "1234567890abcdef") {
			t.Errorf("%s: el error cita el token: %v", key, err)
		}
	}
}

// El token de MUELLE_INIT_CLOUDFLARE_TOKEN_FILE tiene la forma que piden
// --cloudflare-token y PATCH /settings (contrato §5.2): de 20 a 512
// caracteres ASCII visibles, en una sola línea y sin espacios (los de los
// extremos se recortan). Uno que no la tiene no arranca muelle, y el error
// nombra la variable sin citar el token.
func TestLoadInitTokenShape(t *testing.T) {
	for name, content := range map[string]string{
		"corto":      "cf-token-123\n",
		"largo":      strings.Repeat("x", 513) + "\n",
		"no ASCII":   "cf-token-1234567890abcdeñ\n",
		"dos líneas": "cf-token-1234567890abcdef\nsobra-1234567890abcdef\n",
		"espacios":   "cf-token 1234567890abcdef\n",
	} {
		clearEnv(t)
		t.Setenv("MUELLE_PANEL_ORIGIN", "https://panel.tudominio.cl")
		t.Setenv("MUELLE_INIT_CLOUDFLARE_TOKEN_FILE", writeFile(t, "token", content))
		_, err := Load()
		if err == nil || !strings.HasPrefix(err.Error(), "MUELLE_INIT_CLOUDFLARE_TOKEN_FILE: ") ||
			!strings.Contains(err.Error(), "de 20 a 512 caracteres") {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if strings.Contains(err.Error(), "1234567890") || strings.Contains(err.Error(), "xxxxxxxx") {
			t.Errorf("%s: el error cita el token: %v", name, err)
		}
	}
	// Los espacios de los extremos se recortan, como antes.
	clearEnv(t)
	t.Setenv("MUELLE_PANEL_ORIGIN", "https://panel.tudominio.cl")
	t.Setenv("MUELLE_INIT_CLOUDFLARE_TOKEN_FILE", writeFile(t, "token", "  cf-token-1234567890ab \r\n"))
	c, err := Load()
	if err != nil || c.Init.CloudflareToken != "cf-token-1234567890ab" {
		t.Fatalf("recortado: %q %v", c.Init.CloudflareToken, err)
	}
}

// writeFile escribe content en un archivo nuevo del test y devuelve su ruta.
func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// El registry inicial de install (MUELLE_INIT_REGISTRY_FILE, contrato §2.10):
// un JSON {"host", "username", "password"} en un secreto Swarm.
func TestLoadInitRegistry(t *testing.T) {
	clearEnv(t)
	t.Setenv("MUELLE_PANEL_ORIGIN", "https://panel.tudominio.cl")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Init.Registry != nil {
		t.Fatalf("sin la variable: %v", c.Init.Registry)
	}
	t.Setenv("MUELLE_INIT_REGISTRY_FILE", writeFile(t, "registry.json",
		`{"host": "ghcr.io", "username": "bot", "password": "ghp_s3cr3t-pass"}`+"\n"))
	c, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	want := &InitRegistry{Host: "ghcr.io", Username: "bot", Password: "ghp_s3cr3t-pass"}
	if !reflect.DeepEqual(c.Init.Registry, want) {
		t.Fatalf("Registry = %#v", c.Init.Registry)
	}
}

// Un archivo que no sirve no deja arrancar: el error nombra la variable y
// nunca trae la contraseña ni el contenido del archivo.
func TestLoadRejectsBadInitRegistry(t *testing.T) {
	const password = "s3cr3t-pass"
	for name, content := range map[string]string{
		"sin cerrar":      `{"host":"ghcr.io","username":"bot","password":"s3cr3t-pass"`,
		"campo de más":    `{"host":"ghcr.io","username":"bot","password":"s3cr3t-pass","x":"s3cr3t-pass"}`,
		"repetido":        `{"host":"ghcr.io","username":"bot","password":"s3cr3t-pass","password":"s3cr3t-pass"}`,
		"lista":           `["s3cr3t-pass"]`,
		"nulo":            `null`,
		"basura después":  `{"host":"ghcr.io","username":"bot","password":"s3cr3t-pass"} s3cr3t-pass`,
		"host mayúsculas": `{"host":"GHCR.io","username":"bot","password":"s3cr3t-pass"}`,
		"alias de hub":    `{"host":"index.docker.io","username":"bot","password":"s3cr3t-pass"}`,
		"sin usuario":     `{"host":"ghcr.io","username":"","password":"s3cr3t-pass"}`,
		"salto de línea":  `{"host":"ghcr.io","username":"bot","password":"s3cr3t-pass\n"}`,
		"vacío":           ``,
		"muy grande":      `{"host":"ghcr.io","username":"bot","password":"s3cr3t-pass` + strings.Repeat("x", 16<<10) + `"}`,
	} {
		clearEnv(t)
		t.Setenv("MUELLE_PANEL_ORIGIN", "https://panel.tudominio.cl")
		t.Setenv("MUELLE_INIT_REGISTRY_FILE", writeFile(t, "registry.json", content))
		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "MUELLE_INIT_REGISTRY_FILE") {
			t.Errorf("%s: el error debe nombrar la variable: %v", name, err)
			continue
		}
		if strings.Contains(err.Error(), password) {
			t.Errorf("%s: el error trae la contraseña: %v", name, err)
		}
	}
	clearEnv(t)
	t.Setenv("MUELLE_PANEL_ORIGIN", "https://panel.tudominio.cl")
	t.Setenv("MUELLE_INIT_REGISTRY_FILE", "/no/existe/registry.json")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "MUELLE_INIT_REGISTRY_FILE") || !strings.Contains(err.Error(), "/no/existe/registry.json") {
		t.Errorf("un archivo que no existe: %v", err)
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
	const token, password = "cf-token-1234567890abcdef", "ghp_s3cr3t-pass-del-registry"
	s := Settings{PanelOrigin: "https://panel.tudominio.cl", Init: Init{PublicIP: "203.0.113.7", CloudflareToken: token,
		Registry: &InitRegistry{Host: "ghcr.io", Username: "bot", Password: password}}}
	var out []string
	for _, v := range []any{s.Init, &s.Init, s, &s, s.Init.Registry, *s.Init.Registry} {
		out = append(out, fmt.Sprintf("%v", v), fmt.Sprintf("%+v", v), fmt.Sprintf("%#v", v), fmt.Sprint(v))
	}
	var buf bytes.Buffer
	for _, h := range []slog.Handler{slog.NewJSONHandler(&buf, nil), slog.NewTextHandler(&buf, nil)} {
		slog.New(h).Info("arranque", "init", s.Init, "settings", s, "registry", s.Init.Registry)
	}
	out = append(out, buf.String())
	for _, o := range out {
		if strings.Contains(o, token) || strings.Contains(o, password) {
			t.Fatalf("filtra el token o la contraseña: %s", o)
		}
	}
	if !strings.Contains(s.Init.String(), "203.0.113.7") || !strings.Contains(s.Init.String(), "***") || !strings.Contains(s.Init.String(), "ghcr.io") {
		t.Fatalf("String esconde solo el token y la contraseña: %s", s.Init.String())
	}
	if got := s.Init.Registry.String(); got != "{Host:ghcr.io Username:bot Password:***}" {
		t.Fatalf("InitRegistry.String = %s", got)
	}
	// Aparte: Init no tiene MarshalJSON (el token saldría), pero el registry sí,
	// también dentro de Settings.
	for _, v := range []any{s.Init.Registry, s} {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), password) {
			t.Fatalf("el JSON filtra la contraseña: %s", b)
		}
	}
	if b, _ := json.Marshal(InitRegistry{Host: "docker.io"}); !strings.Contains(string(b), `"password":""`) {
		t.Fatalf("una contraseña vacía sale \"\": %s", b)
	}
}
