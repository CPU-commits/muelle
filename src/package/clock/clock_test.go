package clock

import (
	"sync"
	"testing"
	"time"
)

func TestManual(t *testing.T) {
	t0 := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	m := NewManual(t0)
	if got := m.Now(); !got.Equal(t0) {
		t.Fatalf("Now() = %v, esperaba %v", got, t0)
	}
	m.Advance(time.Hour)
	if got := m.Now(); !got.Equal(t0.Add(time.Hour)) {
		t.Fatalf("después de Advance: %v", got)
	}
	t1 := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	m.Set(t1)
	if got := m.Now(); !got.Equal(t1) {
		t.Fatalf("después de Set: %v", got)
	}
}

func TestManualIsSafeForConcurrentUse(t *testing.T) {
	t0 := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	m := NewManual(t0)
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			m.Advance(time.Second)
			_ = m.Now()
		})
	}
	wg.Wait()
	if got := m.Now(); !got.Equal(t0.Add(10 * time.Second)) {
		t.Fatalf("Now() = %v", got)
	}
}

func TestSystem(t *testing.T) {
	var c Clock = System{}
	before := time.Now()
	got := c.Now()
	if got.Before(before) || got.After(time.Now()) {
		t.Fatalf("System.Now() = %v fuera de rango", got)
	}
}
