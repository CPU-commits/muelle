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
