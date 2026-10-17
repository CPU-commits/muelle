// Package install es `muelle install` (contrato §5.4 y diseño §3.3): valida
// los flags, revisa Docker y una instalación anterior, prueba los puertos del
// proxy, inicia el Swarm si hace falta, crea la red, los secretos y el stack
// de muelle, imprime la clave maestra y espera el link de setup. Habla con
// Docker solo por docker.Engine (el fake de dockertest en los tests).
package install

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// Options son los flags de muelle install, ya validados.
type Options struct {
	PanelDomain, Email, PublicIP, PublicIPv6, AdvertiseAddr string
	HTTPPort, HTTPSPort                                     int
	CloudflareToken                                         string // ya leído si vino "-"
	CaddyImage                                              string
	Reinstall                                               bool
	// Las credenciales de las imágenes privadas (plan de la Fase 5, "Cambio
	// de requisito del dueño"); la contraseña ya leída de stdin.
	RegistryUser, RegistryPassword string
	// Ocultos, solo para los tests de integración (contrato §5.1).
	NamePrefix, Image, PanelOrigin string
}

// String es Options con el token y la contraseña como "***" (si hay), como
// settings.Init: unas Options en un %+v de un error o de un log de test no
// filtran nada.
func (o Options) String() string {
	if o.CloudflareToken != "" {
		o.CloudflareToken = "***"
	}
	if o.RegistryPassword != "" {
		o.RegistryPassword = "***"
	}
	type plain Options // sin métodos: no vuelve a llamar a String
	return fmt.Sprintf("%+v", plain(o))
}

// GoString es String para %#v, y LogValue para slog (el JSONHandler
// serializaría el struct).
func (o Options) GoString() string     { return o.String() }
func (o Options) LogValue() slog.Value { return slog.StringValue(o.String()) }

// Usage es la ayuda de muelle install. Nombra solo los flags públicos: los
// ocultos (contrato §5.1) son para los tests.
const Usage = `uso: muelle install --panel-domain <dominio> --email <correo> [flags]

Instala muelle en este servidor (Linux amd64, Docker con un Swarm o sin
iniciar). Se corre con la imagen, siempre con --rm y sin -t:

  docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
    -v ~/.docker/config.json:/root/.docker/config.json:ro \
    ghcr.io/cpu-commits/muelle:<versión> install \
    --panel-domain panel.tudominio.cl --email tu@correo.cl > install.txt

flags:
  --panel-domain <dominio>    dominio del panel (obligatorio): https://<dominio>
  --email <correo>            correo para Let's Encrypt (obligatorio)
  --cloudflare-token <token>  token de la API de Cloudflare (DNS-01 y registros DNS);
                              "-" lo lee de la primera línea de stdin (docker run -i)
  --public-ip <ipv4>          IP pública del servidor (si no, la detecta con 1.1.1.1)
  --public-ipv6 <ipv6>        IPv6 pública (opcional)
  --http-port <puerto>        puerto HTTP del proxy (80)
  --https-port <puerto>       puerto HTTPS del proxy (443)
  --caddy-image <imagen>      fija la imagen de muelle-caddy (avanzado)
  --advertise-addr <ip>       la dirección de docker swarm init, si el Swarm no está
                              iniciado y la máquina tiene varias IPs
  --registry-user <usuario>   usuario del registry de las imágenes de muelle, que son
                              privadas (con --registry-password-stdin); sin él, usa
                              el config.json de Docker montado en /root/.docker
  --registry-password-stdin   lee la contraseña o el token del registry de la primera
                              línea de stdin (docker run -i)
  --reinstall                 vuelve a desplegar muelle después de docker stack rm
                              muelle, con la misma clave maestra y la misma base
  -h, --help                  muestra esta ayuda

La clave maestra y el link de setup salen por stdout; el avance y los avisos,
por stderr.`

const (
	defaultPrefix = naming.MuelleStack
	// maxStdinLine es lo más que se lee de stdin para el token o la contraseña.
	maxStdinLine = 8 << 10
	// maxRegistryUser y maxRegistryPassword son los de POST /registries (contrato §2.10).
	maxRegistryUser     = 255
	maxRegistryPassword = 4096
)

var (
	// cloudflareTokenRe es la forma de un token de Cloudflare, la misma que
	// acepta PATCH /settings (contrato §4.1).
	cloudflareTokenRe = regexp.MustCompile(`^[\x21-\x7e]{20,512}$`)
	prefixRe          = regexp.MustCompile(`^[a-z][a-z0-9]{0,15}$`)
	// ifaceRe es un nombre de interfaz de red (IFNAMSIZ: hasta 15), con el
	// puerto opcional, como lo acepta docker swarm init --advertise-addr.
	ifaceRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,14}(:[0-9]{1,5})?$`)
)

// Los errores de los secretos son fijos: nunca citan el valor.
var (
	errTokenFormat    = errors.New("--cloudflare-token: tiene que tener de 20 a 512 caracteres ASCII visibles, sin espacios")
	errPasswordFormat = fmt.Errorf("--registry-password-stdin: la contraseña tiene que tener de 1 a %d bytes de UTF-8, sin caracteres de control", maxRegistryPassword)
)

// ParseArgs lee los flags de `muelle install` (stdin solo para
// --cloudflare-token - y --registry-password-stdin) y los valida sin tocar
// Docker. Un error es de uso (código 2); con -h devuelve flag.ErrHelp.
func ParseArgs(args []string, stdin io.Reader) (Options, error) {
	var o Options
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.PanelDomain, "panel-domain", "", "")
	fs.StringVar(&o.Email, "email", "", "")
	fs.StringVar(&o.PublicIP, "public-ip", "", "")
	fs.StringVar(&o.PublicIPv6, "public-ipv6", "", "")
	fs.StringVar(&o.AdvertiseAddr, "advertise-addr", "", "")
	fs.IntVar(&o.HTTPPort, "http-port", 80, "")
	fs.IntVar(&o.HTTPSPort, "https-port", 443, "")
	fs.StringVar(&o.CloudflareToken, "cloudflare-token", "", "")
	fs.StringVar(&o.CaddyImage, "caddy-image", "", "")
	fs.BoolVar(&o.Reinstall, "reinstall", false, "")
	fs.StringVar(&o.RegistryUser, "registry-user", "", "")
	passwordStdin := fs.Bool("registry-password-stdin", false, "")
	fs.StringVar(&o.NamePrefix, "name-prefix", defaultPrefix, "")
	fs.StringVar(&o.Image, "image", "", "")
	fs.StringVar(&o.PanelOrigin, "panel-origin", "", "")
	if err := fs.Parse(args); err != nil {
		return Options{}, err // flag.ErrHelp con -h
	}
	if fs.NArg() > 0 {
		return Options{}, fmt.Errorf("no recibe argumentos sueltos (sobra %q)", fs.Arg(0))
	}

	tokenStdin := o.CloudflareToken == "-"
	if tokenStdin {
		o.CloudflareToken = ""
	}
	switch {
	case o.RegistryUser != "" && !*passwordStdin:
		return Options{}, errors.New("--registry-user: va con --registry-password-stdin (la contraseña nunca va en un flag)")
	case o.RegistryUser == "" && *passwordStdin:
		return Options{}, errors.New("--registry-password-stdin: falta --registry-user")
	case tokenStdin && *passwordStdin:
		return Options{}, errors.New("--cloudflare-token - y --registry-password-stdin leen los dos de stdin: pasa el token con --cloudflare-token <token>, o la credencial del registry con el config.json de Docker")
	}
	if err := o.validate(); err != nil {
		return Options{}, err
	}

	// Stdin se lee al final, cuando el resto de los flags ya sirve.
	switch {
	case tokenStdin:
		line, err := firstLine(stdin, "--cloudflare-token")
		if err != nil {
			return Options{}, err
		}
		o.CloudflareToken = strings.TrimSpace(line)
		if o.CloudflareToken == "" {
			return Options{}, errors.New("--cloudflare-token: la primera línea de stdin está vacía")
		}
		if !cloudflareTokenRe.MatchString(o.CloudflareToken) {
			return Options{}, errTokenFormat
		}
	case *passwordStdin:
		line, err := firstLine(stdin, "--registry-password-stdin")
		if err != nil {
			return Options{}, err
		}
		if err := checkRegistryPassword(line); err != nil {
			return Options{}, err
		}
		o.RegistryPassword = line
	}
	return o, nil
}

// firstLine lee la primera línea de r, sin el "\n" ni el "\r" final.
func firstLine(r io.Reader, flagName string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("%s: no hay stdin (usa docker run -i)", flagName)
	}
	br := bufio.NewReader(io.LimitReader(r, maxStdinLine+1))
	line, err := br.ReadString('\n')
	switch {
	case err == nil:
	case errors.Is(err, io.EOF):
		if line == "" {
			return "", fmt.Errorf("%s: stdin está vacío (usa docker run -i y pasa el valor por stdin)", flagName)
		}
		if len(line) > maxStdinLine {
			return "", fmt.Errorf("%s: la primera línea de stdin es demasiado larga", flagName)
		}
	default:
		return "", fmt.Errorf("%s: leyendo stdin: %w", flagName, err)
	}
	line = strings.TrimSuffix(line, "\n")
	return strings.TrimSuffix(line, "\r"), nil
}

// validate revisa todo lo que no viene de stdin, con las mismas reglas que
// PATCH /settings (contrato §5.2). Run la vuelve a llamar con lo que recibe.
func (o Options) validate() error {
	if o.PanelOrigin != "" {
		if err := checkPanelOrigin(o.PanelOrigin); err != nil {
			return err
		}
	} else if o.PanelDomain == "" {
		return errors.New("--panel-domain: es obligatorio (el dominio del panel, ej. panel.tudominio.cl)")
	}
	if o.PanelDomain != "" {
		if err := validate.Hostname("--panel-domain", o.PanelDomain); err != nil {
			return err
		}
	}
	if o.Email == "" {
		return errors.New("--email: es obligatorio (el correo de Let's Encrypt)")
	}
	checks := []struct {
		flag, value string
		check       func(field, s string) error
	}{
		{"--email", o.Email, validate.Email},
		{"--public-ip", o.PublicIP, validate.IPv4},
		{"--public-ipv6", o.PublicIPv6, validate.IPv6},
		{"--caddy-image", o.CaddyImage, validate.ImageRef},
		{"--image", o.Image, validate.ImageRef},
	}
	for _, c := range checks {
		if c.value == "" {
			continue
		}
		if err := c.check(c.flag, c.value); err != nil {
			return err
		}
	}
	if err := validate.Port("--http-port", o.HTTPPort); err != nil {
		return err
	}
	if err := validate.Port("--https-port", o.HTTPSPort); err != nil {
		return err
	}
	if o.HTTPPort == o.HTTPSPort {
		return fmt.Errorf("--https-port: no puede ser el mismo que --http-port (%d)", o.HTTPPort)
	}
	if o.CloudflareToken != "" && !cloudflareTokenRe.MatchString(o.CloudflareToken) {
		return errTokenFormat
	}
	if o.RegistryUser != "" {
		if err := checkRegistryUser(o.RegistryUser); err != nil {
			return err
		}
	}
	if o.RegistryPassword != "" {
		if err := checkRegistryPassword(o.RegistryPassword); err != nil {
			return err
		}
	}
	if !prefixRe.MatchString(o.NamePrefix) {
		return fmt.Errorf("--name-prefix: %q no sirve: de 1 a 16 caracteres a-z y 0-9, empieza con letra", o.NamePrefix)
	}
	if o.AdvertiseAddr != "" && !validAdvertiseAddr(o.AdvertiseAddr) {
		return fmt.Errorf("--advertise-addr: %q no es una IP, una IP:puerto ni un nombre de interfaz", o.AdvertiseAddr)
	}
	return nil
}

// checkRegistryUser y checkRegistryPassword son las reglas de POST
// /registries (contrato §2.10), con las que muelle valida
// MUELLE_INIT_REGISTRY_FILE al arrancar. La contraseña además sin ningún
// caracter de control (§2.10 rechaza los saltos de línea). Los errores no
// citan el valor.
func checkRegistryUser(s string) error {
	if !utf8.ValidString(s) || s == "" || utf8.RuneCountInString(s) > maxRegistryUser || strings.ContainsFunc(s, unicode.IsControl) {
		return fmt.Errorf("--registry-user: tiene que tener de 1 a %d caracteres, sin caracteres de control", maxRegistryUser)
	}
	return nil
}

func checkRegistryPassword(s string) error {
	if !utf8.ValidString(s) || s == "" || len(s) > maxRegistryPassword || strings.ContainsFunc(s, unicode.IsControl) {
		return errPasswordFormat
	}
	return nil
}

// checkPanelOrigin revisa la forma de --panel-origin (esquema http o https y
// host, sin ruta, query ni usuario); muelle lo valida entero al arrancar.
func checkPanelOrigin(s string) error {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Opaque != "" ||
		u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.User != nil {
		return fmt.Errorf("--panel-origin: %q no es un origen (http(s)://host[:puerto])", s)
	}
	return nil
}

func validAdvertiseAddr(s string) bool {
	if _, err := netip.ParseAddr(s); err == nil {
		return true
	}
	if _, err := netip.ParseAddrPort(s); err == nil {
		return true
	}
	return ifaceRe.MatchString(s)
}

// panelOrigin es el origen del panel recién instalado, para el mensaje final
// cuando muelle no dice el suyo en listening: --panel-origin si vino; si no,
// https://<dominio>, con el puerto si no es 443.
func panelOrigin(o Options) string {
	if o.PanelOrigin != "" {
		return o.PanelOrigin
	}
	if o.HTTPSPort == 443 || o.HTTPSPort == 0 {
		return "https://" + o.PanelDomain
	}
	return "https://" + o.PanelDomain + ":" + strconv.Itoa(o.HTTPSPort)
}
