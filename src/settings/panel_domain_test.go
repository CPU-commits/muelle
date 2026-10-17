package settings

import (
	"strings"
	"testing"
)

// install escribe MUELLE_PANEL_DOMAIN (el dominio que enruta muelle-caddy) y
// no MUELLE_PANEL_ORIGIN: el origen del panel sale de los ajustes, con el
// puerto HTTPS de cada momento. MUELLE_PANEL_ORIGIN queda opcional.
func TestLoadPanelDomain(t *testing.T) {
	clearEnv(t)
	t.Setenv("MUELLE_PANEL_DOMAIN", "panel.tudominio.cl")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Domain != "panel.tudominio.cl" || c.PanelOrigin != "" || c.PanelDomain() != "panel.tudominio.cl" || !c.SecureCookies {
		t.Fatalf("con MUELLE_PANEL_DOMAIN: %+v (PanelDomain %q)", c, c.PanelDomain())
	}

	// Con los dos, MUELLE_PANEL_ORIGIN es un origen más (detrás de otro
	// proxy), y el dominio del panel es el de MUELLE_PANEL_DOMAIN.
	t.Setenv("MUELLE_PANEL_ORIGIN", "https://muelle.otro.cl")
	c, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.PanelOrigin != "https://muelle.otro.cl" || c.PanelDomain() != "panel.tudominio.cl" {
		t.Fatalf("con los dos: %+v (PanelDomain %q)", c, c.PanelDomain())
	}
	// Si viene, MUELLE_PANEL_ORIGIN tiene que ser canónico igual.
	t.Setenv("MUELLE_PANEL_ORIGIN", "https://muelle.otro.cl:443")
	if _, err := Load(); err == nil || !strings.HasPrefix(err.Error(), "MUELLE_PANEL_ORIGIN:") {
		t.Fatalf("origen no canónico con MUELLE_PANEL_DOMAIN: %v", err)
	}
}

func TestLoadRejectsBadPanelDomain(t *testing.T) {
	clearEnv(t)
	for _, d := range []string{"Panel.tudominio.cl", "panel.tudominio.cl.", "localhost", "127.0.0.1", "https://panel.tudominio.cl", "panel.tudominio.cl:8443", "*.tudominio.cl"} {
		t.Setenv("MUELLE_PANEL_DOMAIN", d)
		if _, err := Load(); err == nil || !strings.HasPrefix(err.Error(), "MUELLE_PANEL_DOMAIN:") {
			t.Errorf("MUELLE_PANEL_DOMAIN=%q: %v", d, err)
		}
	}
}

// Sin ninguna de las dos, muelle no arranca, y el error nombra las dos.
func TestLoadRequiresOriginOrDomain(t *testing.T) {
	clearEnv(t)
	_, err := Load()
	if err == nil || !strings.HasPrefix(err.Error(), "MUELLE_PANEL_ORIGIN:") || !strings.Contains(err.Error(), "MUELLE_PANEL_DOMAIN") {
		t.Fatalf("sin origen ni dominio: %v", err)
	}
}

// El panel en MUELLE_PANEL_DOMAIN va por https: la cookie tiene que ser Secure.
func TestLoadPanelDomainRequiresSecureCookies(t *testing.T) {
	clearEnv(t)
	t.Setenv("MUELLE_PANEL_DOMAIN", "panel.tudominio.cl")
	t.Setenv("MUELLE_PANEL_ORIGIN", "http://localhost:5173")
	t.Setenv("MUELLE_SECURE_COOKIES", "false")
	if _, err := Load(); err == nil || !strings.HasPrefix(err.Error(), "MUELLE_SECURE_COOKIES:") {
		t.Fatalf("MUELLE_PANEL_DOMAIN con MUELLE_SECURE_COOKIES=false: %v", err)
	}
}

func TestPanelDomainPrefersDomain(t *testing.T) {
	for _, c := range []struct {
		s    Settings
		want string
	}{
		{Settings{Domain: "panel.tudominio.cl"}, "panel.tudominio.cl"},
		{Settings{Domain: "panel.tudominio.cl", PanelOrigin: "https://otro.tudominio.cl"}, "panel.tudominio.cl"},
		{Settings{Domain: "panel.tudominio.cl", PanelOrigin: "http://127.0.0.1:18080"}, "panel.tudominio.cl"},
		{Settings{PanelOrigin: "https://otro.tudominio.cl:8443"}, "otro.tudominio.cl"},
		{Settings{PanelOrigin: "http://127.0.0.1:18080"}, ""},
	} {
		if got := c.s.PanelDomain(); got != c.want {
			t.Errorf("%+v: PanelDomain() = %q, esperaba %q", c.s, got, c.want)
		}
	}
}
