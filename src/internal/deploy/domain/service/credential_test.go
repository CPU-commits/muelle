package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// La credencial de un registry lleva la contraseña en claro (va a Docker),
// pero nunca sale en un %v, en un log de slog ni en un JSON, tampoco dentro
// de otro valor.
func TestCredentialNeverPrintsPassword(t *testing.T) {
	const password = "ghp_contraseña-1234567890abcdef"
	c := Credential{Host: "ghcr.io", Username: "bot", Password: password}
	nested := struct {
		Cred  Credential
		Ptr   *Credential
		Creds []Credential
	}{c, &c, []Credential{c}}
	var out []string
	for _, v := range []any{c, &c, nested, &nested} {
		out = append(out, fmt.Sprintf("%v", v), fmt.Sprintf("%+v", v), fmt.Sprintf("%#v", v), fmt.Sprint(v), fmt.Sprintf("%s", v))
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(b))
	}
	var buf bytes.Buffer
	for _, h := range []slog.Handler{slog.NewJSONHandler(&buf, nil), slog.NewTextHandler(&buf, nil)} {
		slog.New(h).Info("cred", "c", c, "ptr", &c, "nested", nested, slog.Any("any", c))
	}
	out = append(out, buf.String())
	for _, o := range out {
		if strings.Contains(o, password) {
			t.Fatalf("filtra la contraseña: %s", o)
		}
	}
	for _, o := range []string{c.String(), buf.String()} {
		if !strings.Contains(o, "ghcr.io") || !strings.Contains(o, "bot") || !strings.Contains(o, "***") {
			t.Fatalf("debe traer el host, el usuario y ***: %s", o)
		}
	}
	var back map[string]any
	if err := json.Unmarshal([]byte(out[5]), &back); err != nil || back["Host"] != "ghcr.io" || back["Password"] != "***" {
		t.Fatalf("MarshalJSON: %s (%v)", out[5], err)
	}
}
