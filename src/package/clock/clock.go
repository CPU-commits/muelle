// Package clock abstrae el reloj para que los tests controlen el tiempo.
package clock

import (
	"sync"
	"time"
)

type Clock interface {
	Now() time.Time
}

// System es el reloj real.
type System struct{}

func (System) Now() time.Time { return time.Now() }

// Manual es un reloj controlado para tests. Es seguro usarlo desde varias goroutines.
type Manual struct {
	mu  sync.Mutex
	now time.Time
}

func NewManual(t time.Time) *Manual { return &Manual{now: t} }

func (m *Manual) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now
}

func (m *Manual) Set(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = t
}

func (m *Manual) Advance(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = m.now.Add(d)
}
