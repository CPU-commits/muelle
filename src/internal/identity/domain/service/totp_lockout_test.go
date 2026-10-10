package service

import (
	"fmt"
	"sync"
	"sync/atomic"
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

// Una sesión pendiente valida a lo más MaxPendingCodeAttempts códigos: cada
// intento se reserva antes, el último lo avisa, y después no hay más (la
// cuenta no vuelve a empezar). Forget olvida la cuenta de una sesión canjeada.
func TestPendingCodeAttempts(t *testing.T) {
	c := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	p := NewPendingCodeAttempts(c)
	expires := c.Now().Add(MFAPendingTTL)
	for n := 1; n < MaxPendingCodeAttempts; n++ {
		if last, ok := p.Reserve("s1", expires); !ok || last {
			t.Fatalf("intento %d: ok=%v last=%v", n, ok, last)
		}
	}
	if last, ok := p.Reserve("s1", expires); !ok || !last {
		t.Fatalf("el intento %d es el último: ok=%v last=%v", MaxPendingCodeAttempts, ok, last)
	}
	for range 3 {
		if last, ok := p.Reserve("s1", expires); ok || last {
			t.Fatalf("la sesión agotada no reserva más: ok=%v last=%v", ok, last)
		}
	}
	c.Advance(MFAPendingTTL - time.Second)
	if _, ok := p.Reserve("s1", expires); ok {
		t.Fatal("la sesión agotada sigue así hasta que vence")
	}
	if last, ok := p.Reserve("s2", expires); !ok || last {
		t.Fatal("otra sesión no se ve afectada")
	}
	p.Forget("s2")
	for n := 1; n < MaxPendingCodeAttempts; n++ {
		if last, ok := p.Reserve("s2", expires); !ok || last {
			t.Fatalf("Forget reinicia la cuenta (intento %d)", n)
		}
	}
}

// Muchas reservas a la vez sobre una sesión: exactamente MaxPendingCodeAttempts
// salen bien, y una sola es la última.
func TestPendingCodeAttemptsConcurrent(t *testing.T) {
	c := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	p := NewPendingCodeAttempts(c)
	expires := c.Now().Add(MFAPendingTTL)
	var ok, last atomic.Int32
	var wg sync.WaitGroup
	for range 64 {
		wg.Go(func() {
			l, k := p.Reserve("s1", expires)
			if k {
				ok.Add(1)
			}
			if l {
				last.Add(1)
			}
		})
	}
	wg.Wait()
	if ok.Load() != MaxPendingCodeAttempts || last.Load() != 1 {
		t.Fatalf("reservas=%d últimas=%d", ok.Load(), last.Load())
	}
}

// Con el mapa lleno se desalojan las sesiones que vencen antes.
func TestPendingCodeAttemptsEvictsSoonestExpiring(t *testing.T) {
	c := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	p := NewPendingCodeAttempts(c)
	p.maxKeys = 100
	for i := range 100 {
		p.Reserve(fmt.Sprintf("s%d", i), c.Now().Add(time.Duration(i+1)*time.Second))
	}
	p.Reserve("nueva", c.Now().Add(MFAPendingTTL))
	for i := range 100 {
		_, ok := p.entries[fmt.Sprintf("s%d", i)]
		if want := i >= 25; ok != want {
			t.Fatalf("s%d: en el mapa=%v, esperaba %v", i, ok, want)
		}
	}
}

func TestPendingCodeAttemptsIsBounded(t *testing.T) {
	c := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	p := NewPendingCodeAttempts(c)
	p.maxKeys = 8
	for i := range 100 {
		p.Reserve(fmt.Sprintf("s%d", i), c.Now().Add(MFAPendingTTL))
		if n := len(p.entries); n > p.maxKeys {
			t.Fatalf("el mapa creció a %d entradas", n)
		}
	}
	// Las sesiones vencidas se borran primero.
	c.Advance(MFAPendingTTL)
	p.Reserve("nueva-1", c.Now().Add(MFAPendingTTL))
	p.Reserve("nueva-2", c.Now().Add(MFAPendingTTL))
	if n := len(p.entries); n > 2 {
		t.Fatalf("quedaron %d entradas, esperaba solo las nuevas", n)
	}
}
