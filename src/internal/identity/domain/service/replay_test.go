package service

import (
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/package/clock"
)

func TestTOTPStep(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if s := TOTPStep(at); s != at.Unix()/30 {
		t.Fatalf("step=%d", s)
	}
	if TOTPStep(at.Add(29*time.Second)) != TOTPStep(at) || TOTPStep(at.Add(30*time.Second)) != TOTPStep(at)+1 {
		t.Fatal("un paso dura 30 s")
	}
}

// Forget olvida los pasos usados de un usuario (al reiniciarle el 2FA) y no
// toca a los demás.
func TestTOTPReplayGuardForget(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	g := NewTOTPReplayGuard(clock.NewManual(at))
	step := TOTPStep(at)
	g.Use(1, step)
	g.Use(2, step)
	g.Forget(1)
	if !g.Use(1, step) {
		t.Fatal("después de Forget, el paso del usuario 1 se acepta de nuevo")
	}
	if g.Use(2, step) {
		t.Fatal("el usuario 2 conserva su paso usado")
	}
}

// Un código TOTP no se acepta dos veces (RFC 6238 §5.2): un paso ya usado,
// o uno anterior, se rechaza.
func TestTOTPReplayGuard(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	g := NewTOTPReplayGuard(clock.NewManual(at))
	step := TOTPStep(at)

	if !g.Use(1, step) {
		t.Fatal("el primer uso se acepta")
	}
	if g.Use(1, step) {
		t.Fatal("el mismo paso no se acepta dos veces")
	}
	if g.Use(1, step-1) {
		t.Fatal("un paso anterior al último usado no se acepta")
	}
	if !g.Use(2, step) {
		t.Fatal("otro usuario no se ve afectado")
	}
	if !g.Use(1, step+1) {
		t.Fatal("el paso siguiente se acepta")
	}

	// Al activar un secreto nuevo, el código de la activación queda usado.
	g.Set(3, step)
	if g.Use(3, step) {
		t.Fatal("el código de la activación no sirve para entrar")
	}
	// Set reemplaza el historial (es de otro secreto), aunque sea un paso anterior.
	g.Set(1, step-5)
	if !g.Use(1, step) {
		t.Fatal("tras Set, un paso posterior se acepta")
	}
}

// Con el mapa lleno solo se olvidan los pasos que ya no pueden volver a
// validarse: un paso reciente (el actual o el anterior) sobrevive, y su
// código sigue sin poder reutilizarse.
func TestTOTPReplayGuardKeepsRecentSteps(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := clock.NewManual(at)
	g := NewTOTPReplayGuard(c)
	g.maxUsers = 4
	now := TOTPStep(at)
	g.Set(1, now-5) // viejo
	g.Set(2, now-5) // viejo
	g.Set(3, now)   // reciente
	g.Set(4, now-1) // reciente (dentro del desfase)
	g.Use(5, now)   // mapa lleno: se olvidan solo los viejos
	if _, ok := g.last[1]; ok {
		t.Fatal("un paso viejo se olvida")
	}
	if g.Use(3, now) || g.Use(4, now-1) {
		t.Fatal("un paso reciente no se olvida: su código sigue usado")
	}

	// Si todos son recientes, se desaloja primero el más viejo.
	g = NewTOTPReplayGuard(c)
	g.maxUsers = 4
	g.Set(1, now-1)
	g.Set(2, now)
	g.Set(3, now)
	g.Set(4, now)
	g.Set(5, now)
	if _, ok := g.last[1]; ok {
		t.Fatal("el paso más viejo es el primero en irse")
	}
	for id := range int64(4) {
		if g.Use(id+2, now) {
			t.Fatalf("usuario %d: el paso actual no debió olvidarse", id+2)
		}
	}
}

// El mapa está acotado: un paso que ya no puede volver a validarse (más
// viejo que el período de desfase) se puede olvidar.
func TestTOTPReplayGuardIsBounded(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := clock.NewManual(at)
	g := NewTOTPReplayGuard(c)
	g.maxUsers = 4
	for id := range int64(4) {
		g.Use(id, TOTPStep(at))
	}
	c.Advance(2 * TOTPPeriod)
	g.Use(99, TOTPStep(c.Now()))
	if n := len(g.last); n != 1 {
		t.Fatalf("quedaron %d entradas, esperaba solo la nueva", n)
	}
	// Con el mapa lleno de pasos vigentes igual no crece.
	for id := range int64(10) {
		g.Use(100+id, TOTPStep(c.Now()))
		if n := len(g.last); n > g.maxUsers {
			t.Fatalf("el mapa creció a %d entradas", n)
		}
	}
}
