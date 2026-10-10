package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/package/clock"
)

// Los códigos TOTP malos cuentan por usuario, desde cualquier IP: solo llega
// a este paso quien ya sabe la contraseña, así que el bloqueo por IP no sirve.
func TestTOTPLockoutPerUser(t *testing.T) {
	c := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	l := NewTOTPLockout(c)
	for n := 1; n <= 4; n++ {
		if wasLocked, locked := l.Fail(7); wasLocked || locked {
			t.Fatalf("fallo %d: wasLocked=%v locked=%v", n, wasLocked, locked)
		}
	}
	if wasLocked, locked := l.Fail(7); wasLocked || !locked {
		t.Fatalf("el 5º fallo bloquea: wasLocked=%v locked=%v", wasLocked, locked)
	}
	if !l.Locked(7) || l.Locked(8) {
		t.Fatal("el bloqueo es del usuario 7 y de nadie más")
	}
	if wasLocked, _ := l.Fail(7); !wasLocked {
		t.Fatal("con el usuario bloqueado, Fail devuelve wasLocked")
	}
	if l.Succeed(7) {
		t.Fatal("con el usuario bloqueado, el código correcto tampoco pasa")
	}
	c.Advance(LockoutFor(5))
	if !l.Succeed(7) {
		t.Fatal("pasado el bloqueo, el código correcto pasa")
	}
	for range 4 {
		l.Fail(7)
	}
	if l.Locked(7) {
		t.Fatal("el código correcto reinició los fallos")
	}
}

// Una sesión pendiente aguanta MaxPendingCodeAttempts códigos malos; después
// hay que volver a poner la contraseña.
func TestPendingCodeAttempts(t *testing.T) {
	c := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	p := NewPendingCodeAttempts(c)
	expires := c.Now().Add(MFAPendingTTL)
	for n := 1; n < MaxPendingCodeAttempts; n++ {
		if p.Fail("s1", expires) {
			t.Fatalf("fallo %d: todavía no se agota", n)
		}
	}
	if !p.Fail("s1", expires) {
		t.Fatalf("el fallo %d agota la sesión", MaxPendingCodeAttempts)
	}
	if p.Fail("s2", expires) {
		t.Fatal("otra sesión no se ve afectada")
	}
	p.Forget("s2")
	for n := 1; n < MaxPendingCodeAttempts; n++ {
		if p.Fail("s2", expires) {
			t.Fatalf("Forget reinicia la cuenta (fallo %d)", n)
		}
	}
}

func TestPendingCodeAttemptsIsBounded(t *testing.T) {
	c := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	p := NewPendingCodeAttempts(c)
	p.maxKeys = 8
	for i := range 100 {
		p.Fail(fmt.Sprintf("s%d", i), c.Now().Add(MFAPendingTTL))
		if n := len(p.entries); n > p.maxKeys {
			t.Fatalf("el mapa creció a %d entradas", n)
		}
	}
	// Las sesiones vencidas se borran primero.
	c.Advance(MFAPendingTTL)
	p.Fail("nueva-1", c.Now().Add(MFAPendingTTL))
	p.Fail("nueva-2", c.Now().Add(MFAPendingTTL))
	if n := len(p.entries); n > 2 {
		t.Fatalf("quedaron %d entradas, esperaba solo las nuevas", n)
	}
}
