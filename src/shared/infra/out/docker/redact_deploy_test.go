package docker

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// RegistryAuth y StackDeploy llevan secretos (la contraseña, y el compose
// final con los valores de las variables): ningún %v, slog ni JSON los
// imprime (contrato §2.10). Lo demás sí sale, para que sirvan en un log.
func TestSecretsNeverPrinted(t *testing.T) {
	const password, compose = "s3cr3t-pass", "MARCADOR-DEL-COMPOSE"
	a := RegistryAuth{Host: "ghcr.io", Username: "bot", Password: password}
	d := StackDeploy{Name: "p_s_a", Compose: []byte(compose), Registries: []RegistryAuth{a}}

	var out []string
	for _, v := range []any{a, &a, []RegistryAuth{a}, d, &d} {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
			out = append(out, fmt.Sprintf(verb, v))
		}
	}
	out = append(out, fmt.Errorf("%v", d).Error())

	var buf bytes.Buffer
	for _, h := range []slog.Handler{slog.NewJSONHandler(&buf, nil), slog.NewTextHandler(&buf, nil)} {
		slog.New(h).Info("deploy", slog.Any("x", a), slog.Any("y", d), slog.Any("p", &a), slog.Any("q", &d))
	}
	out = append(out, buf.String())

	for _, v := range []any{a, &a, d, &d} {
		b1, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		b2, err := jsonv2.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(b1), string(b2))
	}

	for _, o := range out {
		if strings.Contains(o, password) || strings.Contains(o, compose) {
			t.Fatalf("filtra un secreto: %s", o)
		}
	}
	all := strings.Join(out, "\n")
	for _, want := range []string{"ghcr.io", "bot", "p_s_a", fmt.Sprint(len(compose))} {
		if !strings.Contains(all, want) {
			t.Errorf("falta %q en las salidas:\n%s", want, all)
		}
	}
	// Cada forma por separado dice lo que importa.
	if s := a.String(); !strings.Contains(s, "ghcr.io") || !strings.Contains(s, "bot") || !strings.Contains(s, "[oculta]") {
		t.Errorf("String de RegistryAuth: %s", s)
	}
	if s := d.String(); !strings.Contains(s, "p_s_a") || !strings.Contains(s, "(20 bytes)") || !strings.Contains(s, "ghcr.io") {
		t.Errorf("String de StackDeploy: %s", s)
	}
	if s := fmt.Sprintf("%#v", a); !strings.HasPrefix(s, "docker.RegistryAuth{") {
		t.Errorf("GoString de RegistryAuth: %s", s)
	}
	if s := fmt.Sprintf("%#v", d); !strings.HasPrefix(s, "docker.StackDeploy{") {
		t.Errorf("GoString de StackDeploy: %s", s)
	}
}

// Una contraseña vacía no se confunde con una oculta: sale "".
func TestRegistryAuthEmptyPassword(t *testing.T) {
	a := RegistryAuth{Host: "docker.io", Username: "bot"}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != `{"Host":"docker.io","Username":"bot","Password":""}` {
		t.Fatalf("JSON: %s", got)
	}
	if s := a.String(); strings.Contains(s, "oculta") {
		t.Fatalf("String: %s", s)
	}
}
