package service

import (
	"sync"
	"sync/atomic"
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

// Muchas goroutines a la vez: con el reloj quieto salen exactamente burst
// tokens, y después de avanzarlo, exactamente los que se recargaron.
func TestTokenBucketConcurrent(t *testing.T) {
	c := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	b := NewTokenBucket(2, 100, c)
	count := func() int64 {
		var ok atomic.Int64
		var wg sync.WaitGroup
		for range 64 {
			wg.Go(func() {
				for range 10 {
					if b.Allow() {
						ok.Add(1)
					}
				}
			})
		}
		wg.Wait()
		return ok.Load()
	}
	if n := count(); n != 100 {
		t.Fatalf("la ráfaga dio %d tokens, esperaba 100", n)
	}
	c.Advance(10 * time.Second)
	if n := count(); n != 20 {
		t.Fatalf("en 10 s a 2/s se recargaron %d tokens, esperaba 20", n)
	}
}

// 2 por segundo son a lo más unos 31 millones de filas en los 180 días de
// retención de la auditoría (con 20 por segundo eran unos 39 GB).
func TestAuditBucketDefaults(t *testing.T) {
	if AuditEventsPerSecond != 2 || AuditBurst != 100 {
		t.Fatalf("rate=%v burst=%v", AuditEventsPerSecond, AuditBurst)
	}
}
