package service

import (
	"bytes"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/secrets/domain/entity"
	"github.com/CPU-commits/muelle/src/package/cipher"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
)

func newCrypto(t *testing.T) *EnvCrypto {
	t.Helper()
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return NewEnvCrypto(c)
}

func envVars(t *testing.T, vars map[string]string, secret bool) []entity.EnvVar {
	t.Helper()
	out, err := entity.NewEnvVars("vars", vars, secret, time.UnixMilli(1_700_000_000_000).UTC())
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAAD(t *testing.T) {
	if got := AAD(entity.AppOwner(3), "K"); got != "env:app:3:K" {
		t.Errorf("got %q", got)
	}
	if got := AAD(entity.StageOwner(4), "K"); got != "env:stage:4:K" {
		t.Errorf("got %q", got)
	}
}

func TestSealAndOpen(t *testing.T) {
	e := newCrypto(t)
	o := entity.AppOwner(3)
	vars := envVars(t, map[string]string{"TOKEN": "super-secreto", "LEVEL": "debug"}, true)

	sealed, err := e.Seal(o, vars)
	if err != nil {
		t.Fatal(err)
	}
	if len(sealed) != 2 || sealed[0].Key != "LEVEL" || sealed[1].Key != "TOKEN" || !sealed[1].Secret ||
		!sealed[1].UpdatedAt.Equal(vars[1].UpdatedAt) {
		t.Fatalf("sealed=%+v", sealed)
	}
	if bytes.Contains(sealed[1].Ciphertext, []byte("super-secreto")) {
		t.Fatalf("el valor quedó en claro: %q", sealed[1].Ciphertext)
	}

	opened, err := e.Open(o, sealed)
	if err != nil {
		t.Fatal(err)
	}
	if len(opened) != 2 || string(opened[0].Value) != "debug" || string(opened[1].Value) != "super-secreto" ||
		!opened[1].Secret || !opened[1].UpdatedAt.Equal(vars[1].UpdatedAt) {
		t.Fatalf("opened=%+v", opened)
	}
}

// Un valor cifrado copiado a otro dueño o a otra clave no se puede descifrar (§12 del spec).
func TestSealBindsOwnerAndKey(t *testing.T) {
	e := newCrypto(t)
	sealed, err := e.Seal(entity.AppOwner(3), envVars(t, map[string]string{"K": "v"}, true))
	if err != nil {
		t.Fatal(err)
	}
	moved := sealed[0]
	moved.Key = "K2"
	cases := []struct {
		name  string
		owner entity.Owner
		row   entity.EncryptedEnvVar
	}{
		{"otra app", entity.AppOwner(4), sealed[0]},
		{"el stage con el mismo id", entity.StageOwner(3), sealed[0]},
		{"otra clave", entity.AppOwner(3), moved},
	}
	for _, c := range cases {
		if _, err := e.Open(c.owner, []entity.EncryptedEnvVar{c.row}); !errors.Is(err, cipher.ErrDecrypt) {
			t.Errorf("%s: got %v", c.name, err)
		}
	}
}

func TestSealAndOpenRejectInvalidOwner(t *testing.T) {
	e := newCrypto(t)
	if _, err := e.Seal(entity.Owner{}, envVars(t, map[string]string{"K": "v"}, true)); err == nil {
		t.Error("Seal con un dueño vacío debería fallar")
	}
	if _, err := e.Open(entity.Owner{}, []entity.EncryptedEnvVar{{Key: "K"}}); err == nil {
		t.Error("Open con un dueño vacío debería fallar")
	}
	if _, err := e.Mask(entity.Owner{}, nil); err == nil {
		t.Error("Mask con un dueño vacío debería fallar")
	}
}

func TestMaskHidesSecrets(t *testing.T) {
	e := newCrypto(t)
	o := entity.AppOwner(3)
	secret, _ := e.Seal(o, envVars(t, map[string]string{"DATABASE_URL": "postgres://x"}, true))
	plain, _ := e.Seal(o, envVars(t, map[string]string{"LOG_LEVEL": "debug", "EMPTY": ""}, false))
	rows := append(secret, plain...)

	masked, err := e.Mask(o, rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(masked) != 3 {
		t.Fatalf("masked=%+v", masked)
	}
	if masked[0].Key != "DATABASE_URL" || masked[0].Value != nil || !masked[0].Secret {
		t.Fatalf("masked[0]=%+v", masked[0])
	}
	if masked[1].Key != "EMPTY" || masked[1].Value == nil || *masked[1].Value != "" || masked[1].Secret {
		t.Fatalf("masked[1]=%+v", masked[1])
	}
	if masked[2].Value == nil || *masked[2].Value != "debug" || !masked[2].UpdatedAt.Equal(rows[2].UpdatedAt) {
		t.Fatalf("masked[2]=%+v", masked[2])
	}
}

// Mask ni siquiera descifra los secretos: un secreto que no descifra no hace fallar la lectura.
func TestMaskNeverDecryptsSecrets(t *testing.T) {
	e := newCrypto(t)
	rows := []entity.EncryptedEnvVar{{Key: "TOKEN", Ciphertext: []byte("basura"), Secret: true}}
	masked, err := e.Mask(entity.AppOwner(3), rows)
	if err != nil || len(masked) != 1 || masked[0].Value != nil {
		t.Fatalf("masked=%+v err=%v", masked, err)
	}

	rows[0].Secret = false
	if _, err := e.Mask(entity.AppOwner(3), rows); !errors.Is(err, cipher.ErrDecrypt) {
		t.Fatalf("un valor no secreto que no descifra es un error: %v", err)
	}
}

func TestMerge(t *testing.T) {
	stage := envVars(t, map[string]string{"NATS": "nats:4222", "LEVEL": "info"}, false)
	app := envVars(t, map[string]string{"LEVEL": "debug", "DB": "pg"}, true)
	got := Merge(stage, app)
	want := map[string]string{"NATS": "nats:4222", "LEVEL": "debug", "DB": "pg"}
	if !maps.Equal(got, want) {
		t.Fatalf("got=%v", got)
	}
	if got := Merge(nil, nil); got == nil || len(got) != 0 {
		t.Fatalf("sin variables da un map vacío, no nil: %v", got)
	}
}
