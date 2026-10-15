package entity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestStatusFinal(t *testing.T) {
	for s, want := range map[Status]bool{
		StatusQueued: false, StatusRunning: false,
		StatusSucceeded: true, StatusFailed: true, StatusSuperseded: true,
	} {
		if got := s.Final(); got != want {
			t.Errorf("%s.Final() = %v", s, got)
		}
		if !s.Valid() {
			t.Errorf("%s.Valid() = false", s)
		}
	}
	if Status("pending").Valid() || Status("").Valid() {
		t.Error("un estado desconocido no es válido")
	}
}

func TestTriggerValid(t *testing.T) {
	for _, tr := range []Trigger{TriggerPanel, TriggerAPI, TriggerRollback} {
		if !tr.Valid() {
			t.Errorf("%s.Valid() = false", tr)
		}
	}
	if Trigger("cron").Valid() || Trigger("").Valid() {
		t.Error("un trigger desconocido no es válido")
	}
}

// La contraseña de un registry va en claro en la entidad (deploy la descifra
// para pasarla a Docker), pero nunca sale en un %v, en un log de slog (main
// usa el JSONHandler, que serializa los structs con encoding/json e ignora
// String) ni en un JSON, ni sola ni dentro de otro valor.
func TestRegistryNeverPrintsPassword(t *testing.T) {
	const password = "ghp_contraseña-1234567890abcdef"
	at := time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC)
	r := Registry{ID: 3, Host: "ghcr.io", Username: "bot", Password: password, CreatedAt: at, UpdatedAt: at}
	nested := struct {
		Reg  Registry
		Ptr  *Registry
		List []Registry
	}{r, &r, []Registry{r}}
	var out []string
	for _, v := range []any{r, &r, nested, &nested} {
		out = append(out, fmt.Sprintf("%v", v), fmt.Sprintf("%+v", v), fmt.Sprintf("%#v", v), fmt.Sprint(v), fmt.Sprintf("%s", v))
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(b))
	}
	var buf bytes.Buffer
	for _, h := range []slog.Handler{slog.NewJSONHandler(&buf, nil), slog.NewTextHandler(&buf, nil)} {
		slog.New(h).Info("registry", "r", r, "ptr", &r, "nested", nested, slog.Group("g", "r", r), slog.Any("any", r))
	}
	out = append(out, buf.String())
	for _, o := range out {
		if strings.Contains(o, password) {
			t.Fatalf("filtra la contraseña: %s", o)
		}
	}
	// Se esconde solo la contraseña.
	for _, o := range []string{r.String(), buf.String()} {
		if !strings.Contains(o, "ghcr.io") || !strings.Contains(o, "bot") || !strings.Contains(o, "***") {
			t.Fatalf("debe traer el host, el usuario y ***: %s", o)
		}
	}
	var back map[string]any
	if err := json.Unmarshal([]byte(out[5]), &back); err != nil || back["Host"] != "ghcr.io" || back["Password"] != "***" {
		t.Fatalf("MarshalJSON: %s (%v)", out[5], err)
	}
	if s := (Registry{Host: "ghcr.io"}).String(); strings.Contains(s, "***") {
		t.Fatalf("sin contraseña no hay nada que esconder: %s", s)
	}
}
