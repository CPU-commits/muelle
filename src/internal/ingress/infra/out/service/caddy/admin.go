package caddy

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
)

const (
	// loadURL: el host no importa, la conexión la abre dial. Caddy 2.10 o más
	// nuevo no revisa el Host en un socket unix (caddy#6832).
	loadURL = "http://caddy/load"
	// adminTimeout es el plazo de cada POST /load: Caddy responde cuando
	// terminó de aplicar la configuración.
	adminTimeout = 30 * time.Second
	// maxResponse es lo máximo que se lee de una respuesta de la admin API.
	maxResponse = 64 << 10
	// maxMessage es el largo máximo, en runas, del mensaje de un rechazo.
	maxMessage = 500
)

// Admin es el cliente de la admin API de muelle-caddy, por el socket unix
// que comparten los dos contenedores (volumen muelle_caddy_admin).
type Admin struct{ http *http.Client }

var _ service.CaddyAdmin = (*Admin)(nil)

// NewAdmin arma el cliente. dial abre la conexión con la admin API; nil es
// el socket de producción (naming.CaddyAdminSocket). Los tests pasan el Dial
// de caddytest, y la integración un puente TCP hacia el socket. Sin proxy
// HTTP ni conexiones reusadas: cada POST /load abre la suya.
func NewAdmin(dial func(ctx context.Context) (net.Conn, error)) *Admin {
	if dial == nil {
		dial = func(ctx context.Context) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", naming.CaddyAdminSocket)
		}
	}
	return &Admin{http: &http.Client{
		Timeout: adminTimeout,
		Transport: &http.Transport{
			DialContext:       func(ctx context.Context, _, _ string) (net.Conn, error) { return dial(ctx) },
			DisableKeepAlives: true,
		},
	}}
}

// Load arma el JSON (Render) y lo manda con POST /load. Errores:
// service.ErrPlaceholderInSettings sin mandar nada; service.ErrCaddyUnreachable
// si no hubo respuesta; un *service.CaddyRejected si Caddy respondió con
// error (y conserva la configuración anterior). Ninguno trae el token ni las
// claves de Origin CA (secrets), ni escapados.
func (a *Admin) Load(ctx context.Context, cfg service.ProxyConfig, force bool) error {
	body, err := Render(cfg)
	if err != nil {
		return err
	}
	secrets := secretsOf(cfg)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loadURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: %s", service.ErrCaddyUnreachable, redact(err.Error(), secrets))
	}
	req.Header.Set("Content-Type", "application/json")
	if force {
		req.Header.Set("Cache-Control", "must-revalidate")
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %s", service.ErrCaddyUnreachable, redact(err.Error(), secrets))
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return fmt.Errorf("%w: leyendo la respuesta: %s", service.ErrCaddyUnreachable, redact(err.Error(), secrets))
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	// Caddy responde {"error": "…"}; si no viene así, el cuerpo tal cual.
	var e struct {
		Error string `json:"error"`
	}
	msg := strings.TrimSpace(string(raw))
	if json.Unmarshal(raw, &e) == nil && e.Error != "" {
		msg = e.Error
	}
	if msg == "" {
		msg = "respondió " + strconv.Itoa(resp.StatusCode)
	}
	return &service.CaddyRejected{Message: redact(strings.ToValidUTF8(msg, "�"), secrets)}
}

// minSecretLine es el largo mínimo de una línea base64 de una clave que se
// oculta sola: una más corta podría ser parte de cualquier texto.
const minSecretLine = 16

// secretsOf son lo que nunca sale en un error de Load: el token de
// Cloudflare y cada clave de Origin CA, entera y cada una de sus líneas
// base64 de 16 caracteres o más (contrato §3.4).
func secretsOf(cfg service.ProxyConfig) []string {
	secrets := []string{cfg.CloudflareToken}
	addKey := func(p *service.OriginPEM) {
		if p == nil || p.KeyPEM == "" {
			return
		}
		secrets = append(secrets, p.KeyPEM)
		for line := range strings.Lines(p.KeyPEM) {
			line = strings.TrimSpace(line)
			if len(line) >= minSecretLine && !strings.HasPrefix(line, "-----") {
				secrets = append(secrets, line)
			}
		}
	}
	if cfg.Panel != nil {
		addKey(cfg.Panel.OriginCA)
	}
	for _, r := range cfg.Routes {
		addKey(r.OriginCA)
	}
	return secrets
}

// redact reemplaza por "***" cada secreto y sus formas escapadas (sin las
// comillas de afuera): strconv.Quote, y JSON con y sin el escape de HTML,
// también una dentro de otra. Las reemplaza de la más larga a la más corta,
// y corta a 500 runas. Un secreto "" no reemplaza nada (un ReplaceAll con ""
// metería *** entre cada letra).
func redact(msg string, secrets []string) string {
	var forms []string
	for _, s := range secrets {
		if s != "" {
			forms = append(forms, tokenForms(s)...)
		}
	}
	slices.SortFunc(forms, func(a, b string) int { return cmp.Or(cmp.Compare(len(b), len(a)), strings.Compare(a, b)) })
	for _, form := range forms {
		msg = strings.ReplaceAll(msg, form, "***")
	}
	return truncate(msg, maxMessage)
}

// tokenForms son el token y cómo puede aparecer escapado en un mensaje de
// Caddy: un %q de Go o un pedazo de JSON, hasta dos niveles (un %q de un
// JSON), de la más larga a la más corta.
func tokenForms(token string) []string {
	escapes := []func(string) string{
		func(s string) string { return unquote(strconv.Quote(s)) },
		func(s string) string { return jsonInner(s, true) },
		func(s string) string { return jsonInner(s, false) },
	}
	seen := map[string]bool{token: true}
	forms, level := []string{token}, []string{token}
	for range 2 {
		var next []string
		for _, s := range level {
			for _, esc := range escapes {
				if e := esc(s); e != "" && !seen[e] {
					seen[e] = true
					next = append(next, e)
				}
			}
		}
		forms = append(forms, next...)
		level = next
	}
	slices.SortFunc(forms, func(a, b string) int { return cmp.Or(cmp.Compare(len(b), len(a)), strings.Compare(a, b)) })
	return forms
}

// jsonInner es s como texto JSON, sin las comillas; html escapa <, > y &
// como <, > y & (lo que hace encoding/json por defecto).
func jsonInner(s string, html bool) string {
	b, err := json.Marshal(s, jsontext.EscapeForHTML(html), jsontext.EscapeForJS(html))
	if err != nil {
		return ""
	}
	return unquote(string(b))
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// truncate corta s a n runas, con "…" si sobra.
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	i := 0
	for range n {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return s[:i] + "…"
}
