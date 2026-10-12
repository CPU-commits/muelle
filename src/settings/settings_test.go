package settings

import (
	"strings"
	"testing"
)

// clearEnv aísla el test del entorno del que lo ejecuta (un valor vacío cuenta como no definido).
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"MUELLE_LISTEN", "MUELLE_DATA_DIR", "MUELLE_MASTER_KEY_FILE", "MUELLE_PANEL_ORIGIN", "MUELLE_SECURE_COOKIES"} {
		t.Setenv(k, "")
	}
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
