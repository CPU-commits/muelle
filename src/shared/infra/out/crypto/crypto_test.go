package crypto

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/CPU-commits/muelle/src/package/cipher"
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

func TestRoundTripEmptyPlaintext(t *testing.T) {
	c := testCipher(t)
	ct := c.Encrypt(nil, "env:app:1:VACIA")
	pt, err := c.Decrypt(ct, "env:app:1:VACIA")
	if err != nil {
		t.Fatal(err)
	}
	if len(pt) != 0 {
		t.Fatalf("got %q, esperaba vacío", pt)
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
	if _, err := c.Decrypt(ct, "env:app:2:A"); !errors.Is(err, cipher.ErrDecrypt) {
		t.Fatalf("esperaba ErrDecrypt, got %v", err)
	}
}

func TestEmptyAAD(t *testing.T) {
	c := testCipher(t)
	func() {
		defer func() {
			if r := recover(); r != "crypto: AAD vacía" {
				t.Fatalf("Encrypt con AAD vacía debió hacer panic, recover() = %v", r)
			}
		}()
		c.Encrypt([]byte("x"), "")
	}()

	ct := c.Encrypt([]byte("x"), "aad")
	if _, err := c.Decrypt(ct, ""); !errors.Is(err, cipher.ErrDecrypt) {
		t.Fatalf("Decrypt con AAD vacía: esperaba ErrDecrypt, got %v", err)
	}
}

func TestOtherKeyFails(t *testing.T) {
	c := testCipher(t)
	other, err := New(bytes.Repeat([]byte{8}, KeySize))
	if err != nil {
		t.Fatal(err)
	}
	ct := c.Encrypt([]byte("x"), "aad")
	if _, err := other.Decrypt(ct, "aad"); !errors.Is(err, cipher.ErrDecrypt) {
		t.Fatalf("esperaba ErrDecrypt con otra clave, got %v", err)
	}
}

func TestTamperedFails(t *testing.T) {
	c := testCipher(t)
	ct := c.Encrypt([]byte("x"), "aad")
	ct[len(ct)-1] ^= 0xff
	if _, err := c.Decrypt(ct, "aad"); !errors.Is(err, cipher.ErrDecrypt) {
		t.Fatalf("esperaba ErrDecrypt, got %v", err)
	}
	if _, err := c.Decrypt([]byte{1, 2}, "aad"); !errors.Is(err, cipher.ErrDecrypt) {
		t.Fatalf("esperaba ErrDecrypt con datos cortos, got %v", err)
	}
}

func TestNewRejectsBadKeySize(t *testing.T) {
	if _, err := New(make([]byte, 16)); err == nil {
		t.Fatal("debió fallar con 16 bytes")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadKeyFile(t *testing.T) {
	dir := t.TempDir()
	key := GenerateKey()

	good := filepath.Join(dir, "good")
	writeFile(t, good, key+"\n")
	got, err := LoadKeyFile(good)
	if err != nil || len(got) != KeySize {
		t.Fatalf("key=%d err=%v", len(got), err)
	}

	// Saltos de línea en cualquier lado y espacios en los extremos.
	wrapped := filepath.Join(dir, "wrapped")
	writeFile(t, wrapped, "  "+key[:20]+"\n"+key[20:]+"\r\n  ")
	if again, err := LoadKeyFile(wrapped); err != nil || !bytes.Equal(again, got) {
		t.Fatalf("clave con saltos de línea: err=%v", err)
	}

	inner := filepath.Join(dir, "inner-space")
	writeFile(t, inner, key[:20]+" "+key[20:])
	if _, err := LoadKeyFile(inner); err == nil {
		t.Fatal("debió fallar con un espacio en medio")
	}

	short := filepath.Join(dir, "short")
	writeFile(t, short, "c2hvcnQ=")
	if _, err := LoadKeyFile(short); err == nil {
		t.Fatal("debió fallar con clave corta")
	}

	invalid := filepath.Join(dir, "invalid")
	writeFile(t, invalid, "esto no es base64!")
	if _, err := LoadKeyFile(invalid); err == nil {
		t.Fatal("debió fallar con base64 inválido")
	}

	if _, err := LoadKeyFile(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("debió fallar con archivo inexistente")
	}
}
