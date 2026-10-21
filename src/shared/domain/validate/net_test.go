package validate

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func fieldOf(t *testing.T, err error) string {
	t.Helper()
	ve, ok := errors.AsType[*Error](err)
	if !ok {
		t.Fatalf("esperaba un *validate.Error, llegó %v", err)
	}
	return ve.Field
}

func TestImageRef(t *testing.T) {
	good := []string{
		"nginx", "nginx:1.27", "ghcr.io/aumi-app/backend-app:sha-" + strings.Repeat("a", 40),
		"registry:5000/a/b@sha256:" + strings.Repeat("0", 64),
		"ghcr.io/x/y:tag@sha256:" + strings.Repeat("f", 64),
	}
	for _, s := range good {
		if err := ImageRef("image", s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	bad := []string{"", "${X}", "--help", "Ghcr.io/Foo/Bar:Latest", "a\x00b", "../../containers/json?", "nginx latest", strings.Repeat("a", 513)}
	for _, s := range bad {
		if err := ImageRef("images.web", s); err == nil {
			t.Errorf("%q: debió fallar", s)
		} else if f := fieldOf(t, err); f != "images.web" {
			t.Errorf("%q: field %q", s, f)
		}
	}
}

func TestHostname(t *testing.T) {
	good := []string{"api.auditfy.cl", "panel.tudominio.cl", "xn--and-6ma2c.cl", "a.b.c.d.e", "a-b.c0", strings.Repeat("a", 63) + ".cl"}
	for _, s := range good {
		if err := Hostname("hostname", s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	bad := []string{
		"", "localhost", "API.auditfy.cl", "api.auditfy.cl.", "*.auditfy.cl", "-a.cl", "a-.cl", "a..cl", "a_b.cl",
		"1.2.3.4", "api.auditfy.123", "ñandú.cl", "a.cl:443", "https://a.cl", strings.Repeat("a", 64) + ".cl",
		strings.Repeat("a.", 127) + "cl", // 256 caracteres
	}
	for _, s := range bad {
		if err := Hostname("hostname", s); err == nil {
			t.Errorf("%q: debió fallar", s)
		} else if f := fieldOf(t, err); f != "hostname" {
			t.Errorf("%q: field %q", s, f)
		}
	}
}

func TestIPs(t *testing.T) {
	if err := IPv4("public_ip", "203.0.113.7"); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"", "2001:db8::1", "203.0.113.07", "::ffff:203.0.113.7", "203.0.113", "x"} {
		if err := IPv4("public_ip", s); err == nil || fieldOf(t, err) != "public_ip" {
			t.Errorf("IPv4(%q) = %v", s, err)
		}
	}
	if err := IPv6("public_ipv6", "2001:db8::1"); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"", "203.0.113.7", "2001:0db8::1", "::ffff:203.0.113.7", "fe80::1%eth0", "2001:DB8::1"} {
		if err := IPv6("public_ipv6", s); err == nil || fieldOf(t, err) != "public_ipv6" {
			t.Errorf("IPv6(%q) = %v", s, err)
		}
	}
	// El mensaje sugiere la forma canónica.
	if err := IPv6("public_ipv6", "2001:0db8::1"); !strings.Contains(err.Error(), "2001:db8::1") {
		t.Errorf("mensaje sin la forma canónica: %v", err)
	}
}

func TestEmail(t *testing.T) {
	for _, s := range []string{"tu@correo.cl", "a.b+c@sub.dominio.com"} {
		if err := Email("acme_email", s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	for _, s := range []string{"", "sin-arroba", "a@b", "Ana <a@b.cl>", "a@b.cl\n", " a@b.cl", strings.Repeat("a", 250) + "@b.cl"} {
		if err := Email("acme_email", s); err == nil || fieldOf(t, err) != "acme_email" {
			t.Errorf("Email(%q) = %v", s, err)
		}
	}
}

// Caddy expande placeholders en el email ACME y en el token de Cloudflare
// (contrato §3.4): { o } no pasan, en PATCH /settings, en install y en
// MUELLE_INIT_*. El error no cita el valor (puede ser un token).
func TestNoPlaceholders(t *testing.T) {
	for _, s := range []string{"", "tu@correo.cl", "cf-token-1234567890abcdef"} {
		if err := NoPlaceholders("acme_email", s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	for _, s := range []string{"a{x}@correo.cl", "a}@correo.cl", "{env.TOKEN}", "cf-token-1234{567890abcdef"} {
		err := NoPlaceholders("cloudflare_token", s)
		if err == nil || fieldOf(t, err) != "cloudflare_token" {
			t.Errorf("NoPlaceholders(%q) = %v", s, err)
			continue
		}
		if want := "cloudflare_token: no puede tener { ni }: Caddy los toma como placeholders"; err.Error() != want {
			t.Errorf("NoPlaceholders(%q) = %q, esperaba %q", s, err, want)
		}
	}
	// Email solo no los rechaza (RFC 5322 los admite): van las dos reglas.
	if err := Email("acme_email", "a{x}@correo.cl"); err != nil {
		t.Fatalf("Email con llaves: %v", err)
	}
}

// La forma de un token de API de Cloudflare (contrato §4.1 y §5.2): de 20 a
// 512 caracteres ASCII visibles, sin espacios ni saltos de línea. La
// comparten PATCH /settings, install y MUELLE_INIT_CLOUDFLARE_TOKEN_FILE.
func TestIsCloudflareToken(t *testing.T) {
	for _, s := range []string{"cf-token-1234567890ab", "cf-token-1234567890abcdef", strings.Repeat("a", 512), "!~" + strings.Repeat("x", 18)} {
		if !IsCloudflareToken(s) {
			t.Errorf("IsCloudflareToken(%q) = false", s)
		}
	}
	for _, s := range []string{"", "corto", strings.Repeat("a", 19), strings.Repeat("a", 513),
		"cf-token 1234567890abcdef", "cf-token-1234567890abcdef\n", "\tcf-token-1234567890abcdef", "cf-token-1234567890abcdeñ"} {
		if IsCloudflareToken(s) {
			t.Errorf("IsCloudflareToken(%q) = true", s)
		}
	}
}

func TestPort(t *testing.T) {
	for _, n := range []int{1, 80, 443, 8443, 65535} {
		if err := Port("proxy_http_port", n); err != nil {
			t.Errorf("%d: %v", n, err)
		}
	}
	for _, n := range []int{0, -1, 65536} {
		if err := Port("proxy_http_port", n); err == nil || fieldOf(t, err) != "proxy_http_port" {
			t.Errorf("Port(%d) = %v", n, err)
		}
	}
}

func TestCIDRs(t *testing.T) {
	got, err := CIDRs("panel_allowed_cidrs", []string{"203.0.113.0/24", "10.1.2.3/8", "198.51.100.7", "2001:db8::/32", "2001:db8::1", "203.0.113.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	// Se normalizan (prefijo enmascarado, una IP sola es /32 o /128), sin repetidos y en el orden en que llegaron.
	want := []string{"203.0.113.0/24", "10.0.0.0/8", "198.51.100.7/32", "2001:db8::/32", "2001:db8::1/128"}
	if !slices.Equal(got, want) {
		t.Fatalf("CIDRs = %v, esperaba %v", got, want)
	}
	if got, err := CIDRs("panel_allowed_cidrs", nil); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("una lista vacía es válida (sin restricción): %v %v", got, err)
	}
	for _, list := range [][]string{{"x"}, {"10.0.0.0/33"}, {""}, {"fe80::1%eth0/64"}, slices.Repeat([]string{"10.0.0.1"}, 65)} {
		if _, err := CIDRs("panel_allowed_cidrs", list); err == nil || fieldOf(t, err) != "panel_allowed_cidrs" {
			t.Errorf("CIDRs(%v) = %v", list, err)
		}
	}
}
