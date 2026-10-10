// Package config lee la configuración de muelle desde variables de entorno.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
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
	return c, nil
}

// validateOrigin exige un origen exacto (esquema + host[:puerto]), sin ruta,
// porque se compara literalmente contra el header Origin.
func validateOrigin(origin string) error {
	if origin == "" {
		return fmt.Errorf("es obligatorio (ej. https://panel.tudominio.cl)")
	}
	u, err := url.Parse(origin)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("el esquema debe ser http o https")
	}
	if u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return fmt.Errorf("debe ser solo esquema y host, sin ruta ni barra final (ej. https://panel.tudominio.cl)")
	}
	return nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
