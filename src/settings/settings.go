// Package settings lee la configuración de muelle desde variables de entorno.
package settings

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Settings struct {
	Listen        string // MUELLE_LISTEN, por defecto ":8080"
	DataDir       string // MUELLE_DATA_DIR, por defecto "/data"
	MasterKeyFile string // MUELLE_MASTER_KEY_FILE, por defecto "/run/secrets/muelle_master_key"
	PanelOrigin   string // MUELLE_PANEL_ORIGIN, obligatorio, ej. "https://panel.tudominio.cl"
	SecureCookies bool   // MUELLE_SECURE_COOKIES, por defecto true
}

func Load() (Settings, error) {
	c := Settings{
		Listen:        getenv("MUELLE_LISTEN", ":8080"),
		DataDir:       getenv("MUELLE_DATA_DIR", "/data"),
		MasterKeyFile: getenv("MUELLE_MASTER_KEY_FILE", "/run/secrets/muelle_master_key"),
		PanelOrigin:   os.Getenv("MUELLE_PANEL_ORIGIN"),
		SecureCookies: true,
	}
	if v := os.Getenv("MUELLE_SECURE_COOKIES"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Settings{}, fmt.Errorf("MUELLE_SECURE_COOKIES: %w", err)
		}
		c.SecureCookies = b
	}
	if err := validateOrigin(c.PanelOrigin); err != nil {
		return Settings{}, fmt.Errorf("MUELLE_PANEL_ORIGIN: %w", err)
	}
	if strings.HasPrefix(c.PanelOrigin, "https://") && !c.SecureCookies {
		return Settings{}, errors.New("MUELLE_SECURE_COOKIES: no puede ser false con un MUELLE_PANEL_ORIGIN https")
	}
	return c, nil
}

// validateOrigin exige el origen canónico (esquema + host[:puerto]), tal como
// lo envía un navegador, porque se compara literalmente contra el header Origin.
// Rechaza en vez de normalizar: un valor no canónico nunca coincidiría.
func validateOrigin(origin string) error {
	if origin == "" {
		return errors.New("es obligatorio (ej. https://panel.tudominio.cl)")
	}
	const forma = "debe ser solo esquema y host[:puerto], sin ruta, query, fragmento ni usuario (ej. https://panel.tudominio.cl)"
	u, err := url.Parse(origin)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("el esquema debe ser http o https")
	}
	if u.Host == "" || u.Opaque != "" || u.Path != "" || u.RawQuery != "" || u.ForceQuery ||
		u.Fragment != "" || strings.Contains(origin, "#") || u.User != nil {
		return errors.New(forma)
	}

	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		// La forma canónica de una IPv6 es la de RFC 5952, que es la que da netip.
		a, err := netip.ParseAddr(host)
		if err != nil || a.Zone() != "" || a.Is4In6() {
			return errors.New("la dirección IPv6 no es válida o no está soportada")
		}
		host = "[" + a.String() + "]"
	} else {
		if host == "" {
			return errors.New(forma)
		}
		for _, r := range host {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '.') {
				return errors.New("el host solo admite letras ASCII, dígitos, '-' y '.'")
			}
		}
		// Si la última etiqueta es numérica o hexadecimal, el navegador lo trata
		// como una IP v4 y la normaliza (127.1 → 127.0.0.1), así que solo se
		// acepta la forma decimal con puntos.
		labels := strings.Split(strings.TrimSuffix(host, "."), ".")
		if last := labels[len(labels)-1]; strings.HasPrefix(last, "0x") || strings.Trim(last, "0123456789") == "" {
			if a, err := netip.ParseAddr(host); err != nil || !a.Is4() {
				return errors.New("una IP v4 debe ir en decimal con puntos, sin ceros a la izquierda (ej. 127.0.0.1)")
			}
		}
	}

	canon := u.Scheme + "://" + host
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 || strconv.Itoa(n) != p {
			return errors.New("el puerto debe ser un decimal de 1 a 65535 sin ceros a la izquierda")
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
