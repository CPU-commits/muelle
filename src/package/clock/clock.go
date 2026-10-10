// Package clock abstrae el reloj para que los tests controlen el tiempo.
//
// Todos los relojes dan la hora en UTC y truncada al milisegundo, que es lo
// que guarda SQLite (milisegundos Unix). Así una respuesta armada con la hora
// recién tomada es idéntica a la que se lee después de la base, sea cual sea
// el TZ del proceso. Como el resultado no lleva la lectura monotónica, una
// diferencia entre dos horas sigue al reloj de pared, con resolución de 1 ms.
package clock

import (
	"sync"
	"time"
)

type Clock interface {
	// Now devuelve la hora actual en UTC, truncada al milisegundo.
	Now() time.Time
}

// normalize lleva t a UTC y la trunca al milisegundo.
func normalize(t time.Time) time.Time { return t.UTC().Truncate(time.Millisecond) }

// System es el reloj real.
type System struct{}

func (System) Now() time.Time { return normalize(time.Now()) }

// Manual es un reloj controlado para tests. Es seguro usarlo desde varias
// goroutines. Normaliza como System: guarda la hora en UTC truncada al
// milisegundo, así que Advance con menos de 1 ms puede no moverla.
type Manual struct {
	mu  sync.Mutex
	now time.Time
}

func NewManual(t time.Time) *Manual { return &Manual{now: normalize(t)} }

func (m *Manual) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now
}

func (m *Manual) Set(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = normalize(t)
}

func (m *Manual) Advance(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = normalize(m.now.Add(d))
}
