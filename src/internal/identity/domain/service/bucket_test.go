package service

import (
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/package/clock"
)

func TestTokenBucket(t *testing.T) {
	c := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	b := NewTokenBucket(20, 200, c)
	for i := range 200 {
		if !b.Allow() {
			t.Fatalf("la ráfaga permite 200: rechazó el %d", i+1)
		}
	}
	if b.Allow() {
		t.Fatal("pasada la ráfaga, sin tiempo no hay más")
	}
	c.Advance(50 * time.Millisecond) // 20/s: un token cada 50 ms
	if !b.Allow() || b.Allow() {
		t.Fatal("a los 50 ms hay exactamente un token")
	}
	c.Advance(time.Hour)
	n := 0
	for b.Allow() {
		n++
	}
	if n != 200 {
		t.Fatalf("tras mucho tiempo vuelve a la ráfaga completa: %d", n)
	}
	// Un reloj que retrocede no regala tokens.
	c.Set(c.Now().Add(-time.Hour))
	if b.Allow() {
		t.Fatal("un reloj que retrocede no regala tokens")
	}
}

func TestAuditBucketDefaults(t *testing.T) {
	if AuditEventsPerSecond != 20 || AuditBurst != 200 {
		t.Fatalf("rate=%v burst=%v", AuditEventsPerSecond, AuditBurst)
	}
}
