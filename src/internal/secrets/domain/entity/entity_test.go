package entity

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

func validationField(t *testing.T, err error) string {
	t.Helper()
	ve, ok := errors.AsType[*validate.Error](err)
	if !ok {
		t.Fatalf("esperaba un *validate.Error, llegó %v", err)
	}
	return ve.Field
}

func TestOwners(t *testing.T) {
	if o := StageOwner(4); o.Kind != OwnerStage || o.ID != 4 || !o.Valid() {
		t.Errorf("StageOwner(4) = %+v", o)
	}
	if o := AppOwner(3); o.Kind != OwnerApp || o.ID != 3 || !o.Valid() {
		t.Errorf("AppOwner(3) = %+v", o)
	}
	for _, o := range []Owner{{}, {Kind: OwnerApp}, {Kind: "project", ID: 1}, StageOwner(-1)} {
		if o.Valid() {
			t.Errorf("%+v no debería ser válido", o)
		}
	}
}

func TestNewEnvVar(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	v, err := NewEnvVar("vars", "DATABASE_URL", []byte("postgres://x"), true, at)
	if err != nil {
		t.Fatal(err)
	}
	if v.Key != "DATABASE_URL" || string(v.Value) != "postgres://x" || !v.Secret || !v.UpdatedAt.Equal(at) {
		t.Fatalf("v=%+v", v)
	}

	// El error nombra el campo del cuerpo donde vino la clave.
	if _, err := NewEnvVar("vars", "1BAD", nil, true, at); validationField(t, err) != "vars" {
		t.Errorf("clave inválida en vars: %v", err)
	}
	if _, err := NewEnvVar("set", "bad-key", nil, true, at); validationField(t, err) != "set" {
		t.Errorf("clave inválida en set: %v", err)
	}
	if _, err := NewEnvVar("vars", "COMPOSE_PROJECT_NAME", nil, true, at); validationField(t, err) != "vars" {
		t.Errorf("prefijo COMPOSE_: %v", err)
	}

	if _, err := NewEnvVar("vars", "BIG", []byte(strings.Repeat("x", MaxValueBytes)), true, at); err != nil {
		t.Errorf("un valor de %d bytes debería caber: %v", MaxValueBytes, err)
	}
	if _, err := NewEnvVar("set", "BIG", []byte(strings.Repeat("x", MaxValueBytes+1)), true, at); validationField(t, err) != "set" {
		t.Errorf("valor grande: %v", err)
	}

	// Texto UTF-8 válido y sin NUL, sea secreta o no.
	for _, value := range []string{"\xff", "ok\xc3", "\xed\xa0\x80", "a\x00b"} {
		for _, secret := range []bool{true, false} {
			if _, err := NewEnvVar("vars", "BAD", []byte(value), secret, at); validationField(t, err) != "vars" {
				t.Errorf("valor %q (secret=%v): %v", value, secret, err)
			}
		}
	}
	if _, err := NewEnvVar("vars", "UNICODE", []byte("ñandú 🚀\t\n"), false, at); err != nil {
		t.Errorf("UTF-8 válido con tab y salto de línea: %v", err)
	}
}

func TestNewEnvVarsSortsAndValidates(t *testing.T) {
	at := time.UnixMilli(1_700_000_000_000).UTC()
	vars, err := NewEnvVars("vars", map[string]string{"B": "2", "A": "1", "C": ""}, false, at)
	if err != nil {
		t.Fatal(err)
	}
	if len(vars) != 3 || vars[0].Key != "A" || vars[1].Key != "B" || vars[2].Key != "C" {
		t.Fatalf("vars=%+v", vars)
	}
	if string(vars[0].Value) != "1" || vars[0].Secret || !vars[2].UpdatedAt.Equal(at) {
		t.Fatalf("vars[0]=%+v vars[2]=%+v", vars[0], vars[2])
	}

	if vars, err := NewEnvVars("set", nil, true, at); err != nil || len(vars) != 0 {
		t.Fatalf("un map nil da un set vacío: %v %v", vars, err)
	}
	if _, err := NewEnvVars("set", map[string]string{"OK": "x", "bad-key": "y"}, true, at); validationField(t, err) != "set" {
		t.Errorf("clave inválida: %v", err)
	}
	if _, err := NewEnvVars("set", map[string]string{"OK": "x", "BAD": "\xff"}, true, at); validationField(t, err) != "set" {
		t.Errorf("valor inválido: %v", err)
	}
}

func TestCheckCount(t *testing.T) {
	if err := CheckCount("vars", MaxVars); err != nil {
		t.Errorf("%d variables deberían caber: %v", MaxVars, err)
	}
	if err := CheckCount("set", MaxVars+1); validationField(t, err) != "set" {
		t.Errorf("superar MaxVars: %v", err)
	}
}
