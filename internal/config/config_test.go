package config

import "testing"

func TestLoadDefaults(t *testing.T) {
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
	for _, origin := range []string{"", "panel.example.com", "ftp://x.com", "https://x.com/", "https://x.com/panel"} {
		t.Setenv("MUELLE_PANEL_ORIGIN", origin)
		if _, err := Load(); err == nil {
			t.Errorf("origen %q debió fallar", origin)
		}
	}
}

func TestLoadRejectsBadBool(t *testing.T) {
	t.Setenv("MUELLE_PANEL_ORIGIN", "https://panel.example.com")
	t.Setenv("MUELLE_SECURE_COOKIES", "quizás")
	if _, err := Load(); err == nil {
		t.Fatal("debió fallar")
	}
}
