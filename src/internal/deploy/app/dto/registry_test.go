package dto

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// Las credenciales que llegan a un caso de uso (y el registry inicial de
// install) nunca imprimen la contraseña: ni en un %v, ni en slog, ni en un
// JSON, ni solas ni dentro de otro valor.
func TestRegistryInputNeverPrintsPassword(t *testing.T) {
	const password = "s3cr3t-pass-de-install"
	r := InitRegistry{Host: "ghcr.io", Username: "bot", Password: password}
	nested := struct {
		In  RegistryInput
		Ptr *RegistryInput
	}{r, &r}
	var out []string
	for _, v := range []any{r, &r, nested, &nested} {
		out = append(out, fmt.Sprintf("%v", v), fmt.Sprintf("%+v", v), fmt.Sprintf("%#v", v), fmt.Sprint(v), fmt.Sprintf("%s", v))
		for _, marshal := range []func(any) ([]byte, error){json.Marshal, func(v any) ([]byte, error) { return jsonv2.Marshal(v) }} {
			b, err := marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, string(b))
		}
	}
	var buf bytes.Buffer
	for _, h := range []slog.Handler{slog.NewJSONHandler(&buf, nil), slog.NewTextHandler(&buf, nil)} {
		slog.New(h).Info("registry", "r", r, "ptr", &r, "nested", nested)
	}
	out = append(out, buf.String())
	for _, o := range out {
		if strings.Contains(o, password) {
			t.Fatalf("filtra la contraseña: %s", o)
		}
	}
	if s := r.String(); !strings.Contains(s, "ghcr.io") || !strings.Contains(s, "bot") || !strings.Contains(s, "***") {
		t.Fatalf("String: %s", s)
	}
	if b, _ := json.Marshal(RegistryInput{Host: "docker.io"}); !strings.Contains(string(b), `"Password":""`) {
		t.Fatalf("una contraseña vacía sale \"\": %s", b)
	}
}
