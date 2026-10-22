package dto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// El token de la configuración inicial nunca sale en un %v ni en un log, ni
// solo ni dentro de StartupSettings.
func TestInitialSettingsDoesNotPrintToken(t *testing.T) {
	const token = "cf-token-1234567890abcdef"
	s := StartupSettings{PanelDomain: "panel.tudominio.cl", Init: InitialSettings{PublicIP: "203.0.113.7", CloudflareToken: token}}
	var out []string
	for _, v := range []any{s.Init, &s.Init, s, &s} {
		out = append(out, fmt.Sprintf("%v", v), fmt.Sprintf("%+v", v), fmt.Sprintf("%#v", v), fmt.Sprint(v))
	}
	var buf bytes.Buffer
	for _, h := range []slog.Handler{slog.NewJSONHandler(&buf, nil), slog.NewTextHandler(&buf, nil)} {
		slog.New(h).Info("arranque", "init", s.Init, "startup", s)
	}
	out = append(out, buf.String())
	for _, o := range out {
		if strings.Contains(o, token) {
			t.Fatalf("filtra el token: %s", o)
		}
	}
	if !strings.Contains(s.Init.String(), "203.0.113.7") || !strings.Contains(s.Init.String(), "***") {
		t.Fatalf("String esconde solo el token: %s", s.Init.String())
	}
}

// GET /settings trae siempre todas las claves, también cloudflare_origin_ca
// (contrato §4): el panel las trata como obligatorias.
func TestSettingsOutputAlwaysHasOriginCA(t *testing.T) {
	b, err := json.Marshal(SettingsOutput{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"cloudflare_origin_ca":false`) {
		t.Fatalf("falta cloudflare_origin_ca: %s", b)
	}
}
