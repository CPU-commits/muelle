package entity

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// Settings lleva el token de Cloudflare en claro (Proxy.CloudflareToken):
// nunca sale en un %v ni en un log de slog, con el JSONHandler de main incluido.
func TestSettingsDoesNotPrintToken(t *testing.T) {
	const token = "cf-token-1234567890abcdef"
	p := shared.DefaultProxySettings()
	p.CloudflareToken, p.PublicIP = token, "203.0.113.7"
	s := Settings{Require2FA: true, Proxy: p}
	var out []string
	for _, v := range []any{s, &s} {
		out = append(out, fmt.Sprintf("%v", v), fmt.Sprintf("%+v", v), fmt.Sprintf("%#v", v))
	}
	var buf bytes.Buffer
	for _, h := range []slog.Handler{slog.NewJSONHandler(&buf, nil), slog.NewTextHandler(&buf, nil)} {
		slog.New(h).Info("ajustes", "settings", s, "ptr", &s)
	}
	out = append(out, buf.String())
	for _, o := range out {
		if strings.Contains(o, token) {
			t.Fatalf("filtra el token: %s", o)
		}
	}
	if !strings.Contains(buf.String(), "203.0.113.7") {
		t.Fatalf("el log no trae el resto de los ajustes: %s", buf.String())
	}
}
