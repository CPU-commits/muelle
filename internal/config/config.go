// Package config lee la configuración de muelle desde variables de entorno.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Listen        string // MUELLE_LISTEN, por defecto ":8080"
	DataDir       string // MUELLE_DATA_DIR, por defecto "/data"
	MasterKeyFile string // MUELLE_MASTER_KEY_FILE, por defecto "/run/secrets/muelle_master_key"
	PanelOrigin   string // MUELLE_PANEL_ORIGIN, obligatorio, ej. "https://panel.tudominio.cl"
	SecureCookies bool   // MUELLE_SECURE_COOKIES, por defecto true
}

func Load() (Config, error) {
	c := Config{
		Listen:        getenv("MUELLE_LISTEN", ":8080"),
		DataDir:       getenv("MUELLE_DATA_DIR", "/data"),
		MasterKeyFile: getenv("MUELLE_MASTER_KEY_FILE", "/run/secrets/muelle_master_key"),
		PanelOrigin:   os.Getenv("MUELLE_PANEL_ORIGIN"),
		SecureCookies: true,
	}
	if v := os.Getenv("MUELLE_SECURE_COOKIES"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("MUELLE_SECURE_COOKIES: %w", err)
		}
		c.SecureCookies = b
	}
	if err := validateOrigin(c.PanelOrigin); err != nil {
		return Config{}, fmt.Errorf("MUELLE_PANEL_ORIGIN: %w", err)
	}
	if strings.HasPrefix(c.PanelOrigin, "https://") && !c.SecureCookies {
		return Config{}, fmt.Errorf("MUELLE_SECURE_COOKIES: no puede ser false con un MUELLE_PANEL_ORIGIN https")
	}
	return c, nil
}

// validateOrigin exige el origen canónico (esquema + host[:puerto]), tal como
// lo envía un navegador, porque se compara literalmente contra el header Origin.
// Rechaza en vez de normalizar: un valor no canónico nunca coincidiría.
func validateOrigin(origin string) error {
	if origin == "" {
		return fmt.Errorf("es obligatorio (ej. https://panel.tudominio.cl)")
	}
	const forma = "debe ser solo esquema y host[:puerto], sin ruta, query, fragmento ni usuario (ej. https://panel.tudominio.cl)"
	u, err := url.Parse(origin)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("el esquema debe ser http o https")
	}
	if u.Host == "" || u.Opaque != "" || u.Path != "" || u.RawQuery != "" || u.ForceQuery ||
		u.Fragment != "" || strings.Contains(origin, "#") || u.User != nil {
		return fmt.Errorf(forma)
	}

	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		if net.ParseIP(host) == nil {
			return fmt.Errorf("la dirección IPv6 no es válida")
		}
		host = "[" + host + "]"
	} else {
		if host == "" {
			return fmt.Errorf(forma)
		}
		for _, r := range host {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '.') {
				return fmt.Errorf("el host solo admite letras ASCII, dígitos, '-' y '.'")
			}
		}
	}

	canon := u.Scheme + "://" + host
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != p {
			return fmt.Errorf("el puerto debe ser un decimal de 1 a 65535 sin ceros a la izquierda")
		}
		if !(u.Scheme == "https" && n == 443 || u.Scheme == "http" && n == 80) {
			canon += ":" + p
		}
	}
	if origin != canon {
		return fmt.Errorf("no es un origen canónico; usa %q", canon)
	}
	return nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
