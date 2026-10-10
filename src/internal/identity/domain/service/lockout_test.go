package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/package/clock"
)

func newLockout() (*LoginLockout, *clock.Manual) {
	c := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	return NewLoginLockout(c), c
}

// El calendario es el de §10.1: nada hasta 4 fallos, 1 min con 5, y se
// duplica con cada fallo hasta 1 h.
func TestLockoutSchedule(t *testing.T) {
	l, c := newLockout()
	for n := 1; n <= 12; n++ {
		if l.Fail("felipe", "1.1.1.1") {
			t.Fatalf("fallo %d: el par no estaba bloqueado", n)
		}
		d := LockoutFor(n)
		if d == 0 {
			if l.Locked("felipe", "1.1.1.1") {
				t.Fatalf("con %d fallos no se bloquea", n)
			}
			continue
		}
		if !l.Locked("felipe", "1.1.1.1") {
			t.Fatalf("con %d fallos se bloquea %v", n, d)
		}
		c.Advance(d - time.Millisecond)
		if !l.Locked("felipe", "1.1.1.1") {
			t.Fatalf("con %d fallos sigue bloqueado hasta %v", n, d)
		}
		c.Advance(time.Millisecond)
		if l.Locked("felipe", "1.1.1.1") {
			t.Fatalf("con %d fallos el bloqueo dura %v", n, d)
		}
	}
}

// Mientras el par está bloqueado, ni un fallo ni un éxito cambian nada: así
// un intento que esperó al hasher no revela si la contraseña era correcta.
func TestLockoutNoChangeWhileLocked(t *testing.T) {
	l, c := newLockout()
	for range 5 {
		l.Fail("felipe", "1.1.1.1")
	}
	if !l.Fail("felipe", "1.1.1.1") {
		t.Fatal("Fail con el par bloqueado devuelve true")
	}
	if l.Succeed("felipe", "1.1.1.1", true) {
		t.Fatal("Succeed con el par bloqueado devuelve false")
	}
	// El bloqueo sigue siendo de 1 min (el fallo de arriba no lo extendió)...
	c.Advance(time.Minute)
	if l.Locked("felipe", "1.1.1.1") {
		t.Fatal("el fallo con el par bloqueado no debió extender el bloqueo")
	}
	// ...y el éxito de arriba no borró los fallos: el siguiente bloquea 2 min.
	l.Fail("felipe", "1.1.1.1")
	c.Advance(2*time.Minute - time.Millisecond)
	if !l.Locked("felipe", "1.1.1.1") {
		t.Fatal("el éxito con el par bloqueado no debió borrar los fallos")
	}
}

func TestLockoutSucceedResets(t *testing.T) {
	l, c := newLockout()
	for range 5 {
		l.Fail("felipe", "1.1.1.1")
	}
	c.Advance(time.Minute)
	// Sin reset (login con 2FA, falta el código) los fallos siguen.
	if !l.Succeed("felipe", "1.1.1.1", false) {
		t.Fatal("sin bloqueo vigente, Succeed devuelve true")
	}
	l.Fail("felipe", "1.1.1.1")
	if !l.Locked("felipe", "1.1.1.1") {
		t.Fatal("sin reset, el 6º fallo bloquea")
	}
	c.Advance(2 * time.Minute)
	if !l.Succeed("felipe", "1.1.1.1", true) {
		t.Fatal("Succeed devuelve true")
	}
	for range 4 {
		l.Fail("felipe", "1.1.1.1")
	}
	if l.Locked("felipe", "1.1.1.1") {
		t.Fatal("tras el reset, 4 fallos no bloquean")
	}
}

// La clave es el usuario (en minúsculas) más la IP: un atacante solo se
// bloquea a sí mismo, y una IPv6 cuenta por su /64.
func TestLockoutKey(t *testing.T) {
	l, _ := newLockout()
	for range 5 {
		l.Fail("Admin", "203.0.113.9")
	}
	if !l.Locked("admin", "203.0.113.9") || !l.Locked("ADMIN", "203.0.113.9") {
		t.Fatal("el usuario se compara en minúsculas")
	}
	if l.Locked("admin", "198.51.100.1") {
		t.Fatal("el admin desde otra IP no queda bloqueado")
	}
	if l.Locked("ana", "203.0.113.9") {
		t.Fatal("otro usuario desde la misma IP no queda bloqueado")
	}

	for range 5 {
		l.Fail("admin", "2001:db8:1:2::1")
	}
	if !l.Locked("admin", "2001:db8:1:2:ffff::9") {
		t.Fatal("otra dirección del mismo /64 comparte el bloqueo")
	}
	if l.Locked("admin", "2001:db8:1:3::1") {
		t.Fatal("otro /64 no comparte el bloqueo")
	}

	// Un usuario larguísimo se recorta: no ocupa memoria sin límite.
	long := strings.Repeat("a", 10000)
	l.Fail(long+"x", "1.1.1.1")
	for k := range l.entries {
		if len(k) > maxIPKey+1+maxUsernameKey {
			t.Fatalf("clave de %d bytes", len(k))
		}
	}
}

// El mapa tiene un tamaño máximo. Con el mapa lleno se olvidan primero los
// pares viejos, después los que no están bloqueados; un bloqueo vigente es
// lo último que se desaloja.
func TestLockoutIsBounded(t *testing.T) {
	l, c := newLockout()
	l.maxKeys = 8
	for range 5 {
		l.Fail("admin", "203.0.113.9") // bloqueado
	}
	for i := range 100 {
		l.Fail("nadie", fmt.Sprintf("10.0.0.%d", i))
		if n := len(l.entries); n > l.maxKeys {
			t.Fatalf("el mapa creció a %d entradas (máximo %d)", n, l.maxKeys)
		}
	}
	if !l.Locked("admin", "203.0.113.9") {
		t.Fatal("un bloqueo vigente no se desaloja mientras haya pares sin bloquear")
	}

	// Pasado el tiempo de olvido, la próxima clave nueva con el mapa lleno barre
	// los pares viejos (con dos claves nuevas el mapa seguro se llenó una vez).
	c.Advance(lockoutForget)
	l.Fail("nuevo", "192.0.2.1")
	l.Fail("nuevo", "192.0.2.2")
	if _, ok := l.entries[lockoutKey("admin", "203.0.113.9")]; ok || len(l.entries) > 2 {
		t.Fatalf("quedaron %d entradas (admin: %v), esperaba solo las nuevas", len(l.entries), ok)
	}
}
