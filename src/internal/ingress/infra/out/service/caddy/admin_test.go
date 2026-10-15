package caddy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/caddy/caddytest"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
)

func loadConfig(token string) service.ProxyConfig {
	return service.ProxyConfig{
		Panel:  testPanel(entity.TLSCloudflare),
		Routes: threeRoutes(), ACMEEmail: "ops@auditfy.cl", CloudflareToken: token,
	}
}

type received struct {
	method, path, contentType, cacheControl, host string
	body                                         []byte
}

// unixServer es un http.Server en un socket unix, como la admin API de
// muelle-caddy. La ruta es corta: un socket unix tiene a lo más 108 bytes.
func unixServer(t *testing.T, h func(w http.ResponseWriter, r *http.Request)) (dial func(ctx context.Context) (net.Conn, error)) {
	t.Helper()
	dir, err := os.MkdirTemp("", "it3")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "admin.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(h)}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	return func(ctx context.Context) (net.Conn, error) { return (&net.Dialer{}).DialContext(ctx, "unix", sock) }
}

func TestLoadOverUnixSocket(t *testing.T) {
	got := make(chan received, 1)
	dial := unixServer(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got <- received{r.Method, r.URL.Path, r.Header.Get("Content-Type"), r.Header.Get("Cache-Control"), r.Host, b}
	})
	cfg := loadConfig(testToken)
	if err := NewAdmin(dial).Load(context.Background(), cfg, false); err != nil {
		t.Fatal(err)
	}
	r := <-got
	want, _ := Render(cfg)
	if r.method != "POST" || r.path != "/load" || r.contentType != "application/json" || string(r.body) != string(want) {
		t.Fatalf("recibió %s %s (%s): %s", r.method, r.path, r.contentType, r.body)
	}
	if r.cacheControl != "" {
		t.Errorf("sin force no va Cache-Control: %q", r.cacheControl)
	}
}

// En producción, sin dial, va al socket de muelle-caddy, que en la máquina
// de test no existe.
func TestLoadDefaultDialsCaddySocket(t *testing.T) {
	if _, err := os.Stat(naming.CaddyAdminSocket); err == nil {
		t.Skipf("%s existe en esta máquina", naming.CaddyAdminSocket)
	}
	err := NewAdmin(nil).Load(context.Background(), loadConfig(testToken), false)
	if !errors.Is(err, service.ErrCaddyUnreachable) || !strings.Contains(err.Error(), naming.CaddyAdminSocket) {
		t.Fatalf("sin socket: %v", err)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("el error trae el token: %v", err)
	}
}

func TestLoadForce(t *testing.T) {
	fake := caddytest.New(t)
	a := NewAdmin(fake.Dial)
	ctx := context.Background()
	if err := a.Load(ctx, loadConfig(""), true); err != nil {
		t.Fatal(err)
	}
	if err := a.Load(ctx, loadConfig(""), false); err != nil {
		t.Fatal(err)
	}
	if got := fake.Forced(); len(got) != 2 || !got[0] || got[1] {
		t.Fatalf("Forced: %v", got)
	}
}

func TestLoadRejected(t *testing.T) {
	const token = "tok-123"
	fake := caddytest.New(t)
	a := NewAdmin(fake.Dial)
	fake.Reject(`loading config: provision tls: loading module 'cloudflare': api_token "tok-123" rechazado`)
	err := a.Load(context.Background(), loadConfig(token), false)
	var rej *service.CaddyRejected
	if !errors.As(err, &rej) {
		t.Fatalf("rechazo: %v", err)
	}
	if strings.Contains(err.Error(), token) || !strings.Contains(rej.Message, `api_token "***" rechazado`) {
		t.Fatalf("mensaje: %q", rej.Message)
	}

	fake.Reject(strings.Repeat("é", 2000))
	err = a.Load(context.Background(), loadConfig(token), false)
	if !errors.As(err, &rej) {
		t.Fatal(err)
	}
	if n := utf8.RuneCountInString(rej.Message); n != 501 || !strings.HasSuffix(rej.Message, "…") || !utf8.ValidString(rej.Message) {
		t.Fatalf("cortado: %d runas, %q…", n, rej.Message[:20])
	}
}

// Un 4xx o 5xx que no trae el JSON de Caddy también es un rechazo, con el
// cuerpo (o el status) como mensaje.
func TestLoadRejectedPlainBody(t *testing.T) {
	dial := unixServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "algo salió mal con "+testToken, http.StatusInternalServerError)
	})
	err := NewAdmin(dial).Load(context.Background(), loadConfig(testToken), false)
	var rej *service.CaddyRejected
	if !errors.As(err, &rej) || rej.Message != "algo salió mal con ***" {
		t.Fatalf("rechazo: %v", err)
	}
}

// El token de admin puede traer cualquier ASCII visible, así que el mensaje
// de Caddy lo puede traer escapado de varias formas.
func TestRedactEscapedToken(t *testing.T) {
	const token = `cf"tok\en<1234567890&x`
	jsonHTML, _ := json.Marshal(token) // encoding/json escapa < y & (<, &)
	var plain strings.Builder
	enc := json.NewEncoder(&plain)
	enc.SetEscapeHTML(false)
	enc.Encode(token)
	inner := func(s string) string { return strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(s), `"`), `"`) }
	forms := map[string]string{
		"tal cual":         token,
		"%q":               inner(strconv.Quote(token)),
		"JSON con HTML":    inner(string(jsonHTML)),
		"JSON sin HTML":    inner(plain.String()),
		"%q del JSON":      inner(strconv.Quote(inner(string(jsonHTML)))),
		"Sprintf %q":       inner(fmt.Sprintf("%q", token)),
		"JSON de JSON":     inner(mustJSON(t, inner(string(jsonHTML)))),
		"JSON de %q":       inner(mustJSON(t, inner(strconv.Quote(token)))),
		"%q en un mensaje": fmt.Sprintf("api_token %q no sirve", token),
	}
	for name, form := range forms {
		msg := "loading config: " + form + " fin"
		got := redact(msg, token)
		if !strings.Contains(got, "***") || !strings.HasSuffix(got, " fin") {
			t.Errorf("%s: %q", name, got)
		}
		for other, f := range forms {
			if strings.Contains(got, f) && other != "%q en un mensaje" {
				t.Errorf("%s: queda la forma %s en %q", name, other, got)
			}
		}
	}
	if got := redact("sin token", ""); got != "sin token" {
		t.Errorf("con token vacío no reemplaza nada: %q", got)
	}
}

func mustJSON(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLoadUnreachable(t *testing.T) {
	fake := caddytest.New(t)
	fake.SetDown(true)
	err := NewAdmin(fake.Dial).Load(context.Background(), loadConfig(testToken), false)
	if !errors.Is(err, service.ErrCaddyUnreachable) || strings.Contains(err.Error(), testToken) {
		t.Fatalf("caída: %v", err)
	}
	if len(fake.Loads()) != 0 {
		t.Fatal("no debió llegar nada")
	}
}

// Con { o } en el email o el token no se manda nada.
func TestLoadPlaceholder(t *testing.T) {
	fake := caddytest.New(t)
	cfg := loadConfig("{file./etc/passwd}")
	err := NewAdmin(fake.Dial).Load(context.Background(), cfg, false)
	if !errors.Is(err, service.ErrPlaceholderInSettings) || strings.Contains(err.Error(), cfg.CloudflareToken) {
		t.Fatalf("placeholder: %v", err)
	}
	if len(fake.Loads()) != 0 {
		t.Fatal("no debió mandar nada")
	}
}
