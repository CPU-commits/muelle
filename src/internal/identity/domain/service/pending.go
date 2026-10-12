package service

import (
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/package/clock"
)

const (
	// MaxPendingCodeAttempts es cuántos códigos TOTP se validan como máximo
	// con una sesión pendiente: si ninguno es correcto, se borra y hay que
	// volver a poner la contraseña.
	MaxPendingCodeAttempts = 5
	// maxPendingKeys acota el mapa de intentos por sesión pendiente.
	maxPendingKeys = 10000
)

// PendingCodeAttempts cuenta en memoria los códigos TOTP que se validaron con
// cada sesión pendiente de MFA (por el hash de la sesión). Es seguro usarlo
// desde varias goroutines.
type PendingCodeAttempts struct {
	mu      sync.Mutex
	clock   clock.Clock
	maxKeys int
	entries map[string]pendingEntry
}

type pendingEntry struct {
	attempts int
	expires  time.Time // el vencimiento de la sesión: después la entrada no sirve
}

func NewPendingCodeAttempts(c clock.Clock) *PendingCodeAttempts {
	return &PendingCodeAttempts{clock: c, maxKeys: maxPendingKeys, entries: map[string]pendingEntry{}}
}

// Reserve reserva un intento de la sesión sessionHash (que vence en expires)
// antes de validar su código, en la misma operación que lo cuenta: así los
// pedidos que llegan a la vez no pasan del límite. ok es false si la sesión
// ya usó sus MaxPendingCodeAttempts intentos, y entonces el código no se
// valida. last dice si es el último: si el código es malo, quien llama borra
// la sesión. La cuenta de una sesión agotada se guarda hasta que vence, así
// no vuelve a empezar de cero aunque falle su borrado de la base.
func (p *PendingCodeAttempts) Reserve(sessionHash string, expires time.Time) (last, ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, known := p.entries[sessionHash]
	if !known && len(p.entries) >= p.maxKeys {
		p.makeRoom(p.clock.Now())
	}
	if e.attempts >= MaxPendingCodeAttempts {
		return false, false
	}
	e.attempts++
	e.expires = expires
	p.entries[sessionHash] = e
	return e.attempts == MaxPendingCodeAttempts, true
}

// Forget olvida la cuenta de una sesión que se canjeó (ya no existe).
func (p *PendingCodeAttempts) Forget(sessionHash string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.entries, sessionHash)
}

// makeRoom se llama con el mapa lleno: borra las sesiones vencidas y, si no
// alcanza, las que vencen antes hasta dejar un cuarto libre.
func (p *PendingCodeAttempts) makeRoom(now time.Time) {
	for k, e := range p.entries {
		if !now.Before(e.expires) {
			delete(p.entries, k)
		}
	}
	evictSoonest(p.entries, p.maxKeys*3/4, func(e pendingEntry) time.Time { return e.expires })
}
