package entity

import (
	"errors"
	"fmt"
	"slices"
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

func TestOwnerCheck(t *testing.T) {
	if err := AppOwner(3).Check(); err != nil {
		t.Errorf("AppOwner(3): %v", err)
	}
	if err := StageOwner(4).Check(); err != nil {
		t.Errorf("StageOwner(4): %v", err)
	}
	for _, o := range []Owner{{}, {Kind: "project", ID: 1}, AppOwner(0)} {
		if err := o.Check(); !errors.Is(err, ErrInvalidOwner) {
			t.Errorf("%+v: got %v", o, err)
		}
	}
}

// bigVars arma 15 variables de 64 KiB, y la que falta para llegar justo a MaxOwnerBytes.
func bigVars(t *testing.T) (vars []EnvVar, rest EnvVar) {
	t.Helper()
	big := []byte(strings.Repeat("x", MaxValueBytes))
	for i := range 15 {
		vars = append(vars, EnvVar{Key: fmt.Sprintf("K%02d", i), Value: big})
	}
	restLen := MaxOwnerBytes - TotalSize(vars) - len("REST")
	if restLen <= 0 || restLen > MaxValueBytes {
		t.Fatalf("restLen=%d", restLen)
	}
	return vars, EnvVar{Key: "REST", Value: []byte(strings.Repeat("x", restLen))}
}

func TestSizeAndCheckLimits(t *testing.T) {
	if got := (EnvVar{Key: "ABC", Value: []byte("12345")}).Size(); got != 8 {
		t.Errorf("Size = %d, esperaba 8 (clave + valor)", got)
	}
	if got := TotalSize(nil); got != 0 {
		t.Errorf("TotalSize(nil) = %d", got)
	}

	vars, rest := bigVars(t)
	full := append(slices.Clone(vars), rest)
	if TotalSize(full) != MaxOwnerBytes {
		t.Fatalf("TotalSize = %d", TotalSize(full))
	}
	if err := CheckLimits("vars", full); err != nil {
		t.Errorf("justo %d bytes deberían caber: %v", MaxOwnerBytes, err)
	}
	rest.Value = append(rest.Value, 'x')
	if err := CheckLimits("set", append(slices.Clone(vars), rest)); validationField(t, err) != "set" {
		t.Errorf("un byte de más: %v", err)
	}

	many := make([]EnvVar, MaxVars+1)
	for i := range many {
		many[i] = EnvVar{Key: fmt.Sprintf("K%d", i)}
	}
	if err := CheckLimits("vars", many); validationField(t, err) != "vars" {
		t.Errorf("superar MaxVars: %v", err)
	}
	if err := CheckLimits("vars", many[:MaxVars]); err != nil {
		t.Errorf("%d variables deberían caber: %v", MaxVars, err)
	}
}

func TestPatchTotals(t *testing.T) {
	v := func(k, val string) EnvVar { return EnvVar{Key: k, Value: []byte(val)} }
	// current es clave → bytes (clave + valor) de lo que el dueño ya tiene.
	current := map[string]int{"A": 2, "B": 2, "C": 2}
	// Queda A (2), B reemplazada (1+2) y D nueva (1+1); C y Z se borran.
	count, size := PatchTotals(current, []EnvVar{v("D", "4"), v("B", "20")}, []string{"C", "Z"})
	if count != 3 || size != 2+3+2 {
		t.Fatalf("count=%d size=%d", count, size)
	}
	if len(current) != 3 || current["B"] != 2 {
		t.Fatalf("PatchTotals no modifica current: %v", current)
	}
	if count, size := PatchTotals(nil, nil, []string{"A"}); count != 0 || size != 0 {
		t.Fatalf("sin variables: %d %d", count, size)
	}
}

func TestCheckTotals(t *testing.T) {
	if err := CheckTotals("set", MaxVars, MaxOwnerBytes); err != nil {
		t.Errorf("justo en los límites: %v", err)
	}
	if err := CheckTotals("set", MaxVars+1, 0); validationField(t, err) != "set" {
		t.Errorf("variables de más: %v", err)
	}
	if err := CheckTotals("vars", 1, MaxOwnerBytes+1); validationField(t, err) != "vars" {
		t.Errorf("bytes de más: %v", err)
	}
}
