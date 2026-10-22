// Package settings lee la configuración de muelle desde variables de entorno.
package settings

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

type Settings struct {
	Listen        string // MUELLE_LISTEN, por defecto ":8080"
	DataDir       string // MUELLE_DATA_DIR, por defecto "/data"
	MasterKeyFile string // MUELLE_MASTER_KEY_FILE, por defecto "/run/secrets/muelle_master_key"
	PanelOrigin   string // MUELLE_PANEL_ORIGIN, ej. "https://panel.tudominio.cl"; obligatorio sin MUELLE_PANEL_DOMAIN
	SecureCookies bool   // MUELLE_SECURE_COOKIES, por defecto true
	// Domain es MUELLE_PANEL_DOMAIN, el dominio del panel (el que enruta
	// muelle-caddy), que escribe install. Con él, el origen del panel sale de
	// los ajustes (https://<dominio>[:<proxy_https_port>]) y sigue al puerto,
	// y PanelOrigin queda como un origen permitido más (detrás de otro proxy).
	Domain string

	// Init es la configuración inicial que install le pasa al primer arranque
	// (MUELLE_INIT_*, Fase 5). admin la aplica solo en el primer arranque de
	// la base: después mandan los cambios del panel, aunque las variables sigan.
	Init Init

	// Version es la de este binario (main.version, por -ldflags al compilar
	// la imagen). No viene del entorno: la pone main después de Load.
	Version string
}

// Init son las variables MUELLE_INIT_* que escribe install en el servicio de
// muelle. Lo que no está queda en cero. El token y la contraseña del registry
// van en claro: String, GoString y LogValue los ocultan, también dentro de
// Settings.
type Init struct {
	PublicIP        string // MUELLE_INIT_PUBLIC_IP (IPv4)
	PublicIPv6      string // MUELLE_INIT_PUBLIC_IPV6
	ACMEEmail       string // MUELLE_INIT_ACME_EMAIL
	ProxyHTTPPort   int    // MUELLE_INIT_PROXY_HTTP_PORT (el proxy usa 80 si no está)
	ProxyHTTPSPort  int    // MUELLE_INIT_PROXY_HTTPS_PORT (443 si no está)
	CloudflareToken string // el contenido de MUELLE_INIT_CLOUDFLARE_TOKEN_FILE (un secreto Swarm)
	CaddyImage      string // MUELLE_INIT_CADDY_IMAGE: fija la imagen de muelle-caddy
	// Registry es el registry inicial de install: el contenido de
	// MUELLE_INIT_REGISTRY_FILE (un secreto Swarm). nil si no está. deploy lo
	// guarda solo en el primer arranque de la base (SeedInitRegistry).
	Registry *InitRegistry
}

// InitRegistry es la credencial de un registry que install le pasa a muelle
// (la de un registry privado, si la usó; contrato §2.10). La contraseña va en
// claro: String, GoString, LogValue y MarshalJSON la ocultan, también dentro
// de Init y de Settings (fmt usa el String de un campo exportado, también de
// un puntero).
type InitRegistry struct{ Host, Username, Password string }

// hidden es r con la contraseña como "***" (si hay).
func (r InitRegistry) hidden() InitRegistry {
	if r.Password != "" {
		r.Password = "***"
	}
	return r
}

// String es {Host:ghcr.io Username:bot Password:***}.
func (r InitRegistry) String() string {
	type plain InitRegistry // sin métodos: no vuelve a llamar a String
	return fmt.Sprintf("%+v", plain(r.hidden()))
}

// GoString es String para %#v.
func (r InitRegistry) GoString() string { return r.String() }

// LogValue es String para slog: el JSONHandler serializaría el struct.
func (r InitRegistry) LogValue() slog.Value { return slog.StringValue(r.String()) }

// MarshalJSON es el JSON del archivo con la contraseña como "***" ("" si
// está vacía): un json.Marshal de Settings no la filtra.
func (r InitRegistry) MarshalJSON() ([]byte, error) {
	h := r.hidden()
	return json.Marshal(registryFile{Host: h.Host, Username: h.Username, Password: h.Password})
}

// registryFile es el JSON de MUELLE_INIT_REGISTRY_FILE.
type registryFile struct {
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// maxRegistryFile es el tope del archivo del registry inicial.
const maxRegistryFile = 16 << 10

// String es Init con el token como "***" (si hay).
func (i Init) String() string {
	if i.CloudflareToken != "" {
		i.CloudflareToken = "***"
	}
	type plain Init // sin métodos: no vuelve a llamar a String
	return fmt.Sprintf("%+v", plain(i))
}

// GoString es String para %#v.
func (i Init) GoString() string { return i.String() }

// LogValue es String para slog: el JSONHandler serializaría el struct.
func (i Init) LogValue() slog.Value { return slog.StringValue(i.String()) }

// LogValue es Settings para slog, con el token de Init oculto (fmt usa
// Init.String en el campo).
func (s Settings) LogValue() slog.Value {
	type plain Settings
	return slog.StringValue(fmt.Sprintf("%+v", plain(s)))
}

// PanelDomain es el dominio del panel (el que enruta Caddy, settings
// panel_domain): Domain si está; si no, el host de PanelOrigin si es un
// dominio, y "" si es una IP o localhost (desarrollo).
func (s Settings) PanelDomain() string {
	if s.Domain != "" {
		return s.Domain
	}
	u, err := url.Parse(s.PanelOrigin)
	if err != nil {
		return ""
	}
	if host := u.Hostname(); validate.Hostname("", host) == nil {
		return host
	}
	return ""
}

func Load() (Settings, error) {
	c := Settings{
		Listen:        getenv("MUELLE_LISTEN", ":8080"),
		DataDir:       getenv("MUELLE_DATA_DIR", "/data"),
		MasterKeyFile: getenv("MUELLE_MASTER_KEY_FILE", "/run/secrets/muelle_master_key"),
		PanelOrigin:   os.Getenv("MUELLE_PANEL_ORIGIN"),
		Domain:        os.Getenv("MUELLE_PANEL_DOMAIN"),
		SecureCookies: true,
	}
	if v := os.Getenv("MUELLE_SECURE_COOKIES"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Settings{}, fmt.Errorf("MUELLE_SECURE_COOKIES: %w", err)
		}
		c.SecureCookies = b
	}
	if c.Domain != "" {
		if err := validate.Hostname("MUELLE_PANEL_DOMAIN", c.Domain); err != nil {
			return Settings{}, err
		}
	}
	if c.PanelOrigin != "" || c.Domain == "" {
		if err := validateOrigin(c.PanelOrigin); err != nil {
			return Settings{}, fmt.Errorf("MUELLE_PANEL_ORIGIN: %w", err)
		}
	}
	if strings.HasPrefix(c.PanelOrigin, "https://") && !c.SecureCookies {
		return Settings{}, errors.New("MUELLE_SECURE_COOKIES: no puede ser false con un MUELLE_PANEL_ORIGIN https")
	}
	if c.Domain != "" && !c.SecureCookies {
		return Settings{}, errors.New("MUELLE_SECURE_COOKIES: no puede ser false con MUELLE_PANEL_DOMAIN (el panel va por https)")
	}
	init, err := loadInit()
	if err != nil {
		return Settings{}, err
	}
	c.Init = init
	return c, nil
}

// loadInit lee y valida MUELLE_INIT_*. Los errores empiezan con el nombre de
// la variable (validate.Error usa el nombre como campo).
func loadInit() (Init, error) {
	var in Init
	text := []struct {
		key   string
		dst   *string
		check func(field, s string) error
	}{
		{"MUELLE_INIT_PUBLIC_IP", &in.PublicIP, validate.IPv4},
		{"MUELLE_INIT_PUBLIC_IPV6", &in.PublicIPv6, validate.IPv6},
		{"MUELLE_INIT_ACME_EMAIL", &in.ACMEEmail, acmeEmail},
		{"MUELLE_INIT_CADDY_IMAGE", &in.CaddyImage, validate.ImageRef},
	}
	for _, v := range text {
		s := os.Getenv(v.key)
		if s == "" {
			continue
		}
		if err := v.check(v.key, s); err != nil {
			return Init{}, err
		}
		*v.dst = s
	}
	for key, dst := range map[string]*int{"MUELLE_INIT_PROXY_HTTP_PORT": &in.ProxyHTTPPort, "MUELLE_INIT_PROXY_HTTPS_PORT": &in.ProxyHTTPSPort} {
		s := os.Getenv(key)
		if s == "" {
			continue
		}
		n, err := strconv.Atoi(s)
		if err != nil || strconv.Itoa(n) != s {
			return Init{}, fmt.Errorf("%s: %q no es un puerto", key, s)
		}
		if err := validate.Port(key, n); err != nil {
			return Init{}, err
		}
		*dst = n
	}
	http, https := in.ProxyHTTPPort, in.ProxyHTTPSPort
	if http == 0 {
		http = 80
	}
	if https == 0 {
		https = 443
	}
	if http == https {
		return Init{}, fmt.Errorf("MUELLE_INIT_PROXY_HTTPS_PORT: no puede ser el mismo que el puerto HTTP del proxy (%d)", http)
	}
	if path := os.Getenv("MUELLE_INIT_CLOUDFLARE_TOKEN_FILE"); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return Init{}, fmt.Errorf("MUELLE_INIT_CLOUDFLARE_TOKEN_FILE: %w", err)
		}
		// La forma de --cloudflare-token y de PATCH /settings (contrato
		// §5.2), con los espacios de los extremos recortados. El error no
		// cita el token.
		token := strings.TrimSpace(string(b))
		if !validate.IsCloudflareToken(token) {
			return Init{}, fmt.Errorf("MUELLE_INIT_CLOUDFLARE_TOKEN_FILE: %s no tiene un token de Cloudflare: de 20 a 512 caracteres ASCII visibles, en una sola línea y sin espacios", path)
		}
		if err := validate.NoPlaceholders("MUELLE_INIT_CLOUDFLARE_TOKEN_FILE", token); err != nil {
			return Init{}, err
		}
		in.CloudflareToken = token
	}
	if path := os.Getenv("MUELLE_INIT_REGISTRY_FILE"); path != "" {
		r, err := loadInitRegistry(path)
		if err != nil {
			return Init{}, err
		}
		in.Registry = r
	}
	return in, nil
}

// acmeEmail es MUELLE_INIT_ACME_EMAIL: un email, y sin { ni }, como en PATCH
// /settings. Con ellos Caddy nunca recibiría la configuración (contrato
// §3.4), y el panel va por Caddy: no se podría corregir desde Ajustes.
func acmeEmail(field, s string) error {
	if err := validate.Email(field, s); err != nil {
		return err
	}
	return validate.NoPlaceholders(field, s)
}

// loadInitRegistry lee y valida el registry inicial (contrato §2.10): JSON
// estricto {"host", "username", "password"} y las reglas de
// shared/domain/validate. Se lee en cada arranque, aunque deploy ya no lo
// use: un archivo roto no deja arrancar. Ningún error trae la contraseña ni el
// contenido del archivo: el de decodificación no incluye el de json/v2, que
// puede citar un pedazo.
func loadInitRegistry(path string) (*InitRegistry, error) {
	const key = "MUELLE_INIT_REGISTRY_FILE"
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxRegistryFile+1))
	if err != nil {
		return nil, fmt.Errorf("%s: leyendo %s: %w", key, path, err)
	}
	if len(b) > maxRegistryFile {
		return nil, fmt.Errorf("%s: %s pasa de 16 KiB", key, path)
	}
	// json/v2 rechaza los campos desconocidos y repetidos y lo que sigue al
	// objeto; null deja los campos vacíos, que rechazan las reglas.
	var v registryFile
	if err := jsonv2.Unmarshal(b, &v, jsonv2.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf(`%s: %s no es un JSON {"host", "username", "password"} (sin otros campos)`, key, path)
	}
	if err := validate.RegistryHost(key+".host", v.Host); err != nil {
		return nil, err
	}
	if err := validate.RegistryUsername(key+".username", v.Username); err != nil {
		return nil, err
	}
	if err := validate.RegistryPassword(key+".password", v.Password); err != nil {
		return nil, err
	}
	return &InitRegistry{Host: v.Host, Username: v.Username, Password: v.Password}, nil
}

// validateOrigin exige el origen canónico (esquema + host[:puerto]), tal como
// lo envía un navegador, porque se compara literalmente contra el header Origin.
// Rechaza en vez de normalizar: un valor no canónico nunca coincidiría.
func validateOrigin(origin string) error {
	if origin == "" {
		return errors.New("es obligatorio sin MUELLE_PANEL_DOMAIN (ej. https://panel.tudominio.cl)")
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
