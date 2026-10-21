package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

const testRegistryPassword = "reg-secret-1234567890"

// La contraseña de un registry (la de la imagen de Caddy, que se usa en cada
// recreación) nunca sale en un %v, un log ni un JSON, tampoco dentro de un
// CaddyWant.
func TestCredentialHidesPassword(t *testing.T) {
	c := Credential{Host: "ghcr.io", Username: "bot", Password: testRegistryPassword}
	want := CaddyWant{Image: "ghcr.io/cpu-commits/muelle-caddy:v0.1.0", HTTPPort: 80, HTTPSPort: 443, Auth: &c}

	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("x", "c", c, "ptr", &c, "want", want)
	slog.New(slog.NewTextHandler(&buf, nil)).Info("x", "c", c, "ptr", &c, "want", want)
	j, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	jw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}

	views := map[string]string{
		"Sprint": fmt.Sprint(c), "%+v": fmt.Sprintf("%+v", c), "%#v": fmt.Sprintf("%#v", c), "&": fmt.Sprint(&c),
		"json": string(j), "json del CaddyWant": string(jw),
	}
	for name, s := range views {
		if strings.Contains(s, testRegistryPassword) {
			t.Errorf("%s muestra la contraseña: %s", name, s)
		}
		if !strings.Contains(s, "ghcr.io") || !strings.Contains(s, "bot") || !strings.Contains(s, "***") {
			t.Errorf("%s debería mostrar el host y el usuario, con la contraseña como ***: %s", name, s)
		}
	}
	if strings.Contains(buf.String(), testRegistryPassword) {
		t.Errorf("el log muestra la contraseña: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "bot") {
		t.Errorf("el log debería mostrar el usuario: %s", buf.String())
	}
	if c.Password != testRegistryPassword {
		t.Fatal("ocultarla no cambia el valor")
	}

	// Sin contraseña no hay nada que ocultar.
	if s := fmt.Sprint(Credential{Host: "ghcr.io"}); strings.Contains(s, "***") {
		t.Errorf("sin contraseña: %s", s)
	}
}
