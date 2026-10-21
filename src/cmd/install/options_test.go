package install

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// base son los flags obligatorios; cada caso agrega los suyos.
var base = []string{"--panel-domain", "panel.example.com", "--email", "a@example.com"}

func args(extra ...string) []string { return append(append([]string{}, base...), extra...) }

func TestParseArgsDefaults(t *testing.T) {
	o, err := ParseArgs(args(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if o.PanelDomain != "panel.example.com" || o.Email != "a@example.com" {
		t.Fatalf("o=%v", o)
	}
	if o.HTTPPort != 80 || o.HTTPSPort != 443 || o.NamePrefix != "muelle" || o.Reinstall {
		t.Fatalf("o=%v", o)
	}
	if o.CloudflareToken != "" || o.RegistryUser != "" || o.RegistryPassword != "" || o.Image != "" || o.PanelOrigin != "" {
		t.Fatalf("o=%v", o)
	}
}

func TestParseArgsAll(t *testing.T) {
	token := strings.Repeat("t", 40)
	o, err := ParseArgs(args("--public-ip", "203.0.113.24", "--public-ipv6", "2001:db8::1",
		"--http-port", "8080", "--https-port", "8443", "--cloudflare-token", token,
		"--caddy-image", "ghcr.io/x/caddy:1", "--advertise-addr", "10.0.0.5", "--reinstall"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if o.PublicIP != "203.0.113.24" || o.PublicIPv6 != "2001:db8::1" || o.HTTPPort != 8080 || o.HTTPSPort != 8443 ||
		o.CloudflareToken != token || o.CaddyImage != "ghcr.io/x/caddy:1" || o.AdvertiseAddr != "10.0.0.5" || !o.Reinstall {
		t.Fatalf("o=%v", o)
	}
}

func TestParseArgsRequired(t *testing.T) {
	_, err := ParseArgs([]string{"--email", "a@example.com"}, nil)
	if err == nil || !strings.Contains(err.Error(), "--panel-domain") {
		t.Errorf("sin --panel-domain: %v", err)
	}
	_, err = ParseArgs([]string{"--panel-domain", "panel.example.com"}, nil)
	if err == nil || !strings.Contains(err.Error(), "--email") {
		t.Errorf("sin --email: %v", err)
	}
}

func TestParseArgsInvalid(t *testing.T) {
	longToken := strings.Repeat("x", 600)
	for _, tc := range []struct {
		args  []string
		flag  string
		token string // si no está vacío, el error no lo puede citar
	}{
		{[]string{"--panel-domain", "Panel.Example.com", "--email", "a@example.com"}, "--panel-domain", ""},
		{[]string{"--panel-domain", "panel.example.com", "--email", "x"}, "--email", ""},
		{args("--public-ip", "1.2.3"), "--public-ip", ""},
		{args("--public-ipv6", "2001:0db8::1"), "--public-ipv6", ""},
		{args("--http-port", "0"), "--http-port", ""},
		{args("--https-port", "70000"), "--https-port", ""},
		{args("--http-port", "8443", "--https-port", "8443"), "--https-port", ""},
		{args("--cloudflare-token", "corto"), "--cloudflare-token", "corto"},
		{args("--cloudflare-token", "un token con espacios de sobra"), "--cloudflare-token", "un token con espacios de sobra"},
		{args("--cloudflare-token", longToken), "--cloudflare-token", longToken},
		{args("--caddy-image", "-x"), "--caddy-image", ""},
		{args("--image", "a b"), "--image", ""},
		{args("--name-prefix", "It5"), "--name-prefix", ""},
		{args("--name-prefix", "it5_x"), "--name-prefix", ""},
		{args("--advertise-addr", "-x"), "--advertise-addr", ""},
		{args("suelto"), "suelto", ""},
		{args("--registry-user", "bot\x01", "--registry-password-stdin"), "--registry-user", ""},
	} {
		_, err := ParseArgs(tc.args, nil)
		if err == nil {
			t.Errorf("%q: sin error", tc.args)
			continue
		}
		if !strings.Contains(err.Error(), tc.flag) {
			t.Errorf("%q: el error no nombra %s: %v", tc.args, tc.flag, err)
		}
		if tc.token != "" && strings.Contains(err.Error(), tc.token) {
			t.Errorf("%q: el error cita el token: %v", tc.args, err)
		}
	}
}

// { o } en --email o en --cloudflare-token dejarían a Caddy sin
// configuración (out_of_sync, contrato §3.4), y el panel va por Caddy: install
// los rechaza antes de tocar Docker, con la regla de PATCH /settings. El
// error no cita el token.
func TestParseArgsPlaceholders(t *testing.T) {
	const token = "cf-token-{1234567890abcdef}"
	const msg = "no puede tener { ni }: Caddy los toma como placeholders"
	for _, tc := range []struct {
		args  []string
		stdin string
		flag  string
	}{
		{[]string{"--panel-domain", "panel.example.com", "--email", "a{x}@example.com"}, "", "--email"},
		{args("--cloudflare-token", token), "", "--cloudflare-token"},
		{args("--cloudflare-token", "-"), token + "\n", "--cloudflare-token"},
	} {
		_, err := ParseArgs(tc.args, strings.NewReader(tc.stdin))
		if err == nil || err.Error() != tc.flag+": "+msg {
			t.Errorf("%q: %v", tc.args, err)
		}
	}
}

func TestParseArgsTokenFromStdin(t *testing.T) {
	token := strings.Repeat("a", 40)
	o, err := ParseArgs(args("--cloudflare-token", "-"), strings.NewReader("  "+token+" \r\nsobra\n"))
	if err != nil {
		t.Fatal(err)
	}
	if o.CloudflareToken != token {
		t.Fatalf("token=%q", o.CloudflareToken)
	}

	if _, err := ParseArgs(args("--cloudflare-token", "-"), strings.NewReader("")); err == nil || !strings.Contains(err.Error(), "--cloudflare-token") {
		t.Errorf("stdin vacío: %v", err)
	}
	if _, err := ParseArgs(args("--cloudflare-token", "-"), nil); err == nil || !strings.Contains(err.Error(), "--cloudflare-token") {
		t.Errorf("sin stdin: %v", err)
	}
	bad := "aaaaaaaaaaaa bbbbbbbbbbbbbbbbbbbb"
	_, err = ParseArgs(args("--cloudflare-token", "-"), strings.NewReader(bad+"\n"))
	if err == nil || !strings.Contains(err.Error(), "--cloudflare-token") || strings.Contains(err.Error(), bad) ||
		strings.Contains(err.Error(), "bbbbbbbbbbbbbbbbbbbb") {
		t.Errorf("primera línea con un espacio: %v", err)
	}
	// Stdin no se lee si otro flag ya no sirve.
	r := strings.NewReader(token + "\n")
	if _, err := ParseArgs(args("--cloudflare-token", "-", "--http-port", "0"), r); err == nil || r.Len() != len(token)+1 {
		t.Errorf("leyó stdin con un flag inválido: %v (quedan %d bytes)", err, r.Len())
	}
}

// Las imágenes de muelle son privadas: la credencial viene de
// --registry-user y la contraseña de stdin, nunca de un flag.
func TestParseArgsRegistry(t *testing.T) {
	const pass = "ghp_s3cr3t pass con espacio"
	o, err := ParseArgs(args("--registry-user", "bot", "--registry-password-stdin"), strings.NewReader(pass+"\r\nsobra\n"))
	if err != nil {
		t.Fatal(err)
	}
	if o.RegistryUser != "bot" || o.RegistryPassword != pass {
		t.Fatalf("o=%v (contraseña %q)", o, o.RegistryPassword)
	}

	for _, tc := range []struct {
		args  []string
		stdin string
		flag  string
	}{
		{args("--registry-user", "bot"), pass, "--registry-password-stdin"},
		{args("--registry-password-stdin"), pass, "--registry-user"},
		{args("--registry-user", "bot", "--registry-password-stdin"), "", "--registry-password-stdin"},
		{args("--registry-user", "bot", "--registry-password-stdin"), "\n", "--registry-password-stdin"},
		{args("--registry-user", "bot", "--registry-password-stdin"), "ghp_s3cr3t\x07pass\n", "--registry-password-stdin"},
		{args("--registry-user", "bot", "--registry-password-stdin"), strings.Repeat("p", 4097) + "\n", "--registry-password-stdin"},
		{args("--registry-user", strings.Repeat("u", 256), "--registry-password-stdin"), pass, "--registry-user"},
		// Las dos leen stdin: no se pueden juntar.
		{args("--registry-user", "bot", "--registry-password-stdin", "--cloudflare-token", "-"), pass, "--cloudflare-token -"},
	} {
		_, err := ParseArgs(tc.args, strings.NewReader(tc.stdin))
		if err == nil || !strings.Contains(err.Error(), tc.flag) {
			t.Errorf("%q: %v, esperaba un error que nombre %s", tc.args, err, tc.flag)
			continue
		}
		if strings.Contains(err.Error(), "s3cr3t") || strings.Contains(err.Error(), "ppppp") {
			t.Errorf("%q: el error cita la contraseña: %v", tc.args, err)
		}
	}
}

func TestOptionsHidesSecrets(t *testing.T) {
	const token, pass = "cf-token-secreto-0123456789", "registry-pass-secreta"
	o := Options{PanelDomain: "panel.example.com", Email: "a@example.com", CloudflareToken: token,
		RegistryUser: "bot", RegistryPassword: pass, NamePrefix: "muelle"}
	var outs []string
	for _, f := range []string{"%v", "%+v", "%#v", "%s"} {
		outs = append(outs, fmt.Sprintf(f, o), fmt.Sprintf(f, &o))
	}
	outs = append(outs, fmt.Sprint(o), fmt.Sprint(&o), o.String())
	var jsonLog, textLog bytes.Buffer
	slog.New(slog.NewJSONHandler(&jsonLog, nil)).Info("x", "opts", o, "ptr", &o)
	slog.New(slog.NewTextHandler(&textLog, nil)).Info("x", "opts", o, "ptr", &o)
	outs = append(outs, jsonLog.String(), textLog.String())
	for _, s := range outs {
		if strings.Contains(s, token) || strings.Contains(s, pass) {
			t.Errorf("filtra un secreto: %s", s)
		}
	}
	if s := o.String(); !strings.Contains(s, "***") || !strings.Contains(s, "panel.example.com") || !strings.Contains(s, "bot") {
		t.Errorf("String=%s", s)
	}
	// Sin secretos no hay ***.
	if s := (Options{PanelDomain: "p.example.com"}).String(); strings.Contains(s, "***") {
		t.Errorf("String sin secretos=%s", s)
	}
}

func TestParseArgsPanelOrigin(t *testing.T) {
	o, err := ParseArgs([]string{"--panel-origin", "https://127.0.0.1:22443", "--email", "a@example.com"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if o.PanelOrigin != "https://127.0.0.1:22443" || o.PanelDomain != "" {
		t.Fatalf("o=%v", o)
	}
	for _, bad := range []string{"https://x/y", "ftp://x", "x", "https://", "https://x?a=1", "https://u@x"} {
		if _, err := ParseArgs([]string{"--panel-origin", bad, "--email", "a@example.com"}, nil); err == nil || !strings.Contains(err.Error(), "--panel-origin") {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestParseArgsAdvertiseAddr(t *testing.T) {
	for _, ok := range []string{"10.0.0.5", "10.0.0.5:2377", "[2001:db8::1]:2377", "2001:db8::1", "eth0", "enp3s0:2377"} {
		if _, err := ParseArgs(args("--advertise-addr", ok), nil); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"-x", "a b", "eth0;rm", strings.Repeat("e", 20)} {
		if _, err := ParseArgs(args("--advertise-addr", bad), nil); err == nil || !strings.Contains(err.Error(), "--advertise-addr") {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestParseArgsHelp(t *testing.T) {
	for _, h := range []string{"-h", "--help"} {
		if _, err := ParseArgs([]string{h}, nil); !errors.Is(err, flag.ErrHelp) {
			t.Errorf("%s: %v", h, err)
		}
	}
}

func TestUsage(t *testing.T) {
	for _, f := range []string{"--panel-domain", "--email", "--cloudflare-token", "--public-ip", "--public-ipv6",
		"--http-port", "--https-port", "--caddy-image", "--advertise-addr", "--registry-user",
		"--registry-password-stdin", "--reinstall"} {
		if !strings.Contains(Usage, f) {
			t.Errorf("Usage no nombra %s", f)
		}
	}
	for _, hidden := range []string{"name-prefix", "--image ", "panel-origin"} {
		if strings.Contains(Usage, hidden) {
			t.Errorf("Usage nombra el flag oculto %q", hidden)
		}
	}
}

func TestPanelOriginFromFlags(t *testing.T) {
	for _, tc := range []struct {
		o    Options
		want string
	}{
		{Options{PanelDomain: "panel.example.com", HTTPSPort: 443}, "https://panel.example.com"},
		{Options{PanelDomain: "panel.example.com", HTTPSPort: 8443}, "https://panel.example.com:8443"},
		{Options{PanelDomain: "panel.example.com", HTTPSPort: 8443, PanelOrigin: "https://127.0.0.1:22443"}, "https://127.0.0.1:22443"},
	} {
		if got := panelOrigin(tc.o); got != tc.want {
			t.Errorf("panelOrigin(%v) = %q, esperaba %q", tc.o, got, tc.want)
		}
	}
}
