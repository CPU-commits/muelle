package crypto

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testCipher(t *testing.T) *Cipher {
	t.Helper()
	c, err := New(bytes.Repeat([]byte{7}, KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRoundTrip(t *testing.T) {
	c := testCipher(t)
	ct := c.Encrypt([]byte("postgres://secreto"), "env:app:1:DATABASE_URL")
	pt, err := c.Decrypt(ct, "env:app:1:DATABASE_URL")
	if err != nil {
		t.Fatal(err)
	}
	if string(pt) != "postgres://secreto" {
		t.Fatalf("got %q", pt)
	}
}

func TestNonceIsRandom(t *testing.T) {
	c := testCipher(t)
	a := c.Encrypt([]byte("x"), "aad")
	b := c.Encrypt([]byte("x"), "aad")
	if bytes.Equal(a, b) {
		t.Fatal("dos cifrados del mismo valor no deben ser iguales")
	}
}

func TestWrongAADFails(t *testing.T) {
	c := testCipher(t)
	ct := c.Encrypt([]byte("x"), "env:app:1:A")
	if _, err := c.Decrypt(ct, "env:app:2:A"); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("esperaba ErrDecrypt, got %v", err)
	}
}

func TestTamperedFails(t *testing.T) {
	c := testCipher(t)
	ct := c.Encrypt([]byte("x"), "aad")
	ct[len(ct)-1] ^= 0xff
	if _, err := c.Decrypt(ct, "aad"); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("esperaba ErrDecrypt, got %v", err)
	}
	if _, err := c.Decrypt([]byte{1, 2}, "aad"); !errors.Is(err, ErrDecrypt) {
		t.Fatalf("esperaba ErrDecrypt con datos cortos, got %v", err)
	}
}

func TestNewRejectsBadKeySize(t *testing.T) {
	if _, err := New(make([]byte, 16)); err == nil {
		t.Fatal("debió fallar con 16 bytes")
	}
}

func TestLoadKeyFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good")
	os.WriteFile(good, []byte(GenerateKey()+"\n"), 0o600)
	key, err := LoadKeyFile(good)
	if err != nil || len(key) != KeySize {
		t.Fatalf("key=%d err=%v", len(key), err)
	}

	short := filepath.Join(dir, "short")
	os.WriteFile(short, []byte("c2hvcnQ="), 0o600)
	if _, err := LoadKeyFile(short); err == nil {
		t.Fatal("debió fallar con clave corta")
	}

	if _, err := LoadKeyFile(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("debió fallar con archivo inexistente")
	}
}
