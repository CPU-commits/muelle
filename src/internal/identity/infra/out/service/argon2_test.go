package service

import (
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"
)

func TestPasswordHashAndVerify(t *testing.T) {
	h := NewArgon2Hasher()
	hash, err := h.Hash("una-contraseña-larga")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("formato: %s", hash)
	}
	parts := strings.Split(hash, "$")
	salt, _ := base64.RawStdEncoding.DecodeString(parts[4])
	key, _ := base64.RawStdEncoding.DecodeString(parts[5])
	if len(salt) != 16 || len(key) != 32 {
		t.Fatalf("salt de %d bytes y clave de %d bytes", len(salt), len(key))
	}

	ok, err := h.Verify(hash, "una-contraseña-larga")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	ok, err = h.Verify(hash, "otra-contraseña-larga")
	if err != nil || ok {
		t.Fatalf("contraseña incorrecta aceptada: ok=%v err=%v", ok, err)
	}
	hash2, _ := h.Hash("una-contraseña-larga")
	if hash == hash2 {
		t.Fatal("el salt debe ser aleatorio")
	}
	if _, err := h.Verify("$bcrypt$x", "x"); err == nil {
		t.Fatal("formato inválido debió fallar")
	}
}

// Verify usa los parámetros guardados en el hash, así se pueden subir más adelante
// sin invalidar las contraseñas existentes.
func TestVerifyReadsParamsFromHash(t *testing.T) {
	salt := []byte("0123456789abcdef")
	key := argon2.IDKey([]byte("contraseña-vieja"), salt, 3, 8*1024, 2, 24)
	old := fmt.Sprintf("$argon2id$v=19$m=8192,t=3,p=2$%s$%s",
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))

	h := NewArgon2Hasher()
	if ok, err := h.Verify(old, "contraseña-vieja"); err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if ok, _ := h.Verify(old, "otra"); ok {
		t.Fatal("contraseña incorrecta aceptada")
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	h := NewArgon2Hasher()
	good, _ := h.Hash("x")
	parts := strings.Split(good, "$")
	bad := []string{
		"",
		"$argon2i$v=19$m=19456,t=2,p=1$" + parts[4] + "$" + parts[5], // otra variante
		"$argon2id$v=18$m=19456,t=2,p=1$" + parts[4] + "$" + parts[5], // otra versión
		"$argon2id$v=19$m=19456,t=0,p=1$" + parts[4] + "$" + parts[5], // t=0 haría panic en argon2
		"$argon2id$v=19$m=19456,t=2,p=0$" + parts[4] + "$" + parts[5], // p=0 haría panic en argon2
		"$argon2id$v=19$m=19456,t=2,p=1$no*base64$" + parts[5],
		"$argon2id$v=19$m=19456,t=2,p=1$" + parts[4] + "$",
		"$argon2id$v=19$m=19456,t=2,p=1$" + parts[4],
	}
	for _, b := range bad {
		if ok, err := h.Verify(b, "x"); err == nil || ok {
			t.Errorf("%q: esperaba error, got ok=%v err=%v", b, ok, err)
		}
	}
}

// TestHasherAllowsAtMostTwoAtOnce: Hash y Verify comparten un semáforo de
// capacidad 2, para que un pico de logins no rompa el presupuesto de RAM (§10.1).
func TestHasherAllowsAtMostTwoAtOnce(t *testing.T) {
	h := NewArgon2Hasher()
	stored, err := h.Hash("contraseña")
	if err != nil {
		t.Fatal(err)
	}

	var inside, peak, calls atomic.Int32
	release := make(chan struct{})
	h.key = func(password, salt []byte, time, memory uint32, threads uint8, keyLen uint32) []byte {
		n := inside.Add(1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		calls.Add(1)
		<-release
		inside.Add(-1)
		return make([]byte, keyLen)
	}

	var wg sync.WaitGroup
	for i := range 4 {
		wg.Go(func() {
			if i%2 == 0 {
				h.Hash("contraseña")
			} else {
				h.Verify(stored, "contraseña")
			}
		})
	}

	waitFor(t, func() bool { return inside.Load() >= 2 })
	// Las otras dos esperan en el semáforo: no entran aunque pase el tiempo.
	time.Sleep(50 * time.Millisecond)
	if n := inside.Load(); n != 2 {
		t.Fatalf("%d cálculos a la vez, esperaba 2", n)
	}
	close(release)
	wg.Wait()

	if p := peak.Load(); p != 2 {
		t.Fatalf("pico de %d cálculos a la vez, esperaba 2", p)
	}
	if c := calls.Load(); c != 4 {
		t.Fatalf("%d cálculos, esperaba 4", c)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("la condición no se cumplió a tiempo")
		}
		time.Sleep(time.Millisecond)
	}
}
