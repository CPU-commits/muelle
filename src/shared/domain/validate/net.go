package validate

import (
	"net/mail"
	"net/netip"
	"regexp"
	"strings"

	"github.com/distribution/reference"
)

// ImageRef valida una referencia de imagen Docker: registro/repositorio, tag
// y digest opcionales, en minúsculas (como la imagen del spec de una app).
// Lo usan el cuerpo de un deploy (Fase 2) y caddy_image.
func ImageRef(field, s string) error {
	if s == "" || len(s) > 512 {
		return Errorf(field, "es obligatoria y de hasta 512 caracteres")
	}
	// Defensa extra: una imagen que empieza con "-" o lleva "$" nunca debe
	// llegar a un comando aunque la librería la aceptara.
	if strings.HasPrefix(s, "-") || strings.Contains(s, "$") {
		return Errorf(field, "no es una referencia de imagen válida")
	}
	if _, err := reference.ParseNormalizedNamed(s); err != nil { // el texto de err está en inglés: no se muestra
		return Errorf(field, "no es una referencia válida de Docker (registro/repositorio[:tag][@sha256:…], en minúsculas)")
	}
	return nil
}

var labelRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// Hostname valida un nombre de dominio completo en su forma canónica: en
// minúsculas, sin punto final, con al menos dos etiquetas de letras ASCII,
// dígitos y '-' (los dominios con tildes van en punycode, xn--…), de hasta 253
// caracteres. Rechaza las IP y los comodines (*.dominio), que la v1 no soporta.
func Hostname(field, s string) error {
	if s == "" || len(s) > 253 {
		return Errorf(field, "es obligatorio y de hasta 253 caracteres")
	}
	labels := strings.Split(s, ".")
	if len(labels) < 2 {
		return Errorf(field, "%q no es un dominio completo (ej. api.tudominio.cl)", s)
	}
	for _, l := range labels {
		if !labelRe.MatchString(l) {
			return Errorf(field, "%q no es válido: en minúsculas, cada parte con letras, dígitos o '-' (hasta 63), sin '*' ni punto final; un dominio con tildes va en punycode (xn--…)", s)
		}
	}
	// Una última etiqueta numérica es una IP (o lo parece): los dominios de
	// primer nivel nunca son numéricos.
	if strings.Trim(labels[len(labels)-1], "0123456789") == "" {
		return Errorf(field, "%q parece una IP; usa un nombre de dominio", s)
	}
	return nil
}

// IPv4 valida una IPv4 en su forma canónica (decimal con puntos, sin ceros a la izquierda).
func IPv4(field, s string) error {
	a, err := netip.ParseAddr(s)
	if err != nil || !a.Is4() {
		return Errorf(field, "%q no es una IPv4 válida", s)
	}
	if a.String() != s {
		return Errorf(field, "usa la forma canónica %q", a.String())
	}
	return nil
}

// IPv6 valida una IPv6 en su forma canónica (RFC 5952: minúsculas y ceros
// comprimidos), sin zona ni IPv4 mapeada.
func IPv6(field, s string) error {
	a, err := netip.ParseAddr(s)
	if err != nil || !a.Is6() || a.Is4In6() || a.Zone() != "" {
		return Errorf(field, "%q no es una IPv6 válida (sin zona ni IPv4 mapeada)", s)
	}
	if a.String() != s {
		return Errorf(field, "usa la forma canónica %q", a.String())
	}
	return nil
}

// Email valida una dirección simple (usuario@dominio.tld), sin nombre ni
// espacios, de hasta 254 caracteres.
func Email(field, s string) error {
	if len(s) > 254 {
		return Errorf(field, "no puede superar 254 caracteres")
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s || a.Name != "" {
		return Errorf(field, "%q no es un email válido (usuario@dominio)", s)
	}
	_, domain, _ := strings.Cut(s, "@")
	if !strings.Contains(domain, ".") {
		return Errorf(field, "%q no es un email válido (usuario@dominio)", s)
	}
	return nil
}

// NoPlaceholders rechaza { y } en un valor que va tal cual a la
// configuración de Caddy (el email ACME y el token de Cloudflare): Caddy los
// expande como placeholders, y con uno el POST /load entero falla (contrato
// §3.4). Lo usan PATCH /settings, install y MUELLE_INIT_*. El error no cita
// el valor, que puede ser un token.
func NoPlaceholders(field, s string) error {
	if strings.ContainsAny(s, "{}") {
		return Errorf(field, "no puede tener { ni }: Caddy los toma como placeholders")
	}
	return nil
}

// cloudflareTokenRe es la forma de un token de API de Cloudflare.
var cloudflareTokenRe = regexp.MustCompile(`^[\x21-\x7e]{20,512}$`)

// IsCloudflareToken dice si s tiene la forma de un token de API de
// Cloudflare: de 20 a 512 caracteres ASCII visibles, sin espacios ni saltos
// de línea (contrato §4.1). La comparten PATCH /settings, install
// (--cloudflare-token) y MUELLE_INIT_CLOUDFLARE_TOKEN_FILE, cada uno con su
// mensaje, que nunca cita el valor.
func IsCloudflareToken(s string) bool { return cloudflareTokenRe.MatchString(s) }

// Port valida un puerto TCP o UDP (1-65535).
func Port(field string, n int) error {
	if n < 1 || n > 65535 {
		return Errorf(field, "debe estar entre 1 y 65535")
	}
	return nil
}

// MaxCIDRs es el máximo de rangos de panel_allowed_cidrs.
const MaxCIDRs = 64

// CIDRs valida una lista de rangos IPv4 o IPv6 y la devuelve normalizada: el
// prefijo enmascarado (10.1.2.3/8 → 10.0.0.0/8), una IP sola como /32 o
// /128, sin repetidos y en el orden en que llegaron. Una lista vacía (o nil)
// es válida y devuelve una lista vacía, no nil.
func CIDRs(field string, list []string) ([]string, error) {
	if len(list) > MaxCIDRs {
		return nil, Errorf(field, "a lo más %d rangos", MaxCIDRs)
	}
	out := make([]string, 0, len(list))
	seen := map[string]bool{}
	for _, s := range list {
		var p netip.Prefix
		if a, err := netip.ParseAddr(s); err == nil && a.Zone() == "" {
			p = netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen())
		} else if p, err = netip.ParsePrefix(s); err != nil {
			return nil, Errorf(field, "%q no es un rango CIDR (ej. 203.0.113.0/24) ni una IP", s)
		}
		c := p.Masked().String()
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out, nil
}
