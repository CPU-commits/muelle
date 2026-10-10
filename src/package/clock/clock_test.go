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
	before := time.Now().Truncate(time.Millisecond) // Now() trunca al milisegundo
	got := c.Now()
	if got.Before(before) || got.After(time.Now()) {
		t.Fatalf("System.Now() = %v fuera de rango", got)
	}
}

// inLocal hace que time.Local sea loc mientras dure el test. time.Now() lee
// time.Local, así que esto simula un proceso con TZ=loc (TZ se lee una sola
// vez, al primer uso). Solo sirve en tests que no corren en paralelo.
func inLocal(t *testing.T, loc *time.Location) {
	t.Helper()
	prev := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = prev })
}

// La API devuelve las fechas en UTC con milisegundos (lo que guarda SQLite),
// así que el reloj ya las da así: la respuesta de un create es igual a la del GET.
func TestSystemIsUTCMillis(t *testing.T) {
	inLocal(t, time.FixedZone("-03", -3*60*60))
	if time.Now().Location() == time.UTC {
		t.Fatal("el test necesita un time.Local distinto de UTC")
	}
	for range 20 {
		got := System{}.Now()
		if got.Location() != time.UTC {
			t.Fatalf("System.Now() = %v, esperaba UTC", got)
		}
		if got.Nanosecond()%int(time.Millisecond) != 0 {
			t.Fatalf("System.Now() = %v tiene más precisión que el milisegundo", got)
		}
	}
}

func TestManualNormalizesToUTCMillis(t *testing.T) {
	santiago := time.FixedZone("-03", -3*60*60)
	t0 := time.Date(2026, 10, 10, 16, 36, 53, 770745182, santiago)
	want := time.Date(2026, 10, 10, 19, 36, 53, 770000000, time.UTC)
	check := func(step string, got, want time.Time) {
		t.Helper()
		if got != want { // == compara también la zona, no solo el instante
			t.Fatalf("%s: Now() = %v, esperaba %v", step, got, want)
		}
	}
	m := NewManual(t0)
	check("NewManual", m.Now(), want)
	m.Advance(1500 * time.Microsecond)
	check("Advance", m.Now(), want.Add(time.Millisecond))
	m.Set(t0.Add(time.Hour))
	check("Set", m.Now(), want.Add(time.Hour))
}
