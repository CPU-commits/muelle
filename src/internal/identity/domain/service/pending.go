package service

import (
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/package/clock"
)

const (
	// MaxPendingCodeAttempts es cuántos códigos TOTP malos aguanta una sesión
	// pendiente: después se borra y hay que volver a poner la contraseña.
	MaxPendingCodeAttempts = 5
	// maxPendingKeys acota el mapa de intentos por sesión pendiente.
	maxPendingKeys = 10000
)

// PendingCodeAttempts cuenta en memoria los códigos TOTP malos de cada sesión
// pendiente de MFA (por el hash de la sesión). Es seguro usarlo desde varias
// goroutines.
type PendingCodeAttempts struct {
	mu      sync.Mutex
	clock   clock.Clock
	maxKeys int
	entries map[string]pendingEntry
}

type pendingEntry struct {
	failures int
	expires  time.Time // el vencimiento de la sesión: después la entrada no sirve
}

func NewPendingCodeAttempts(c clock.Clock) *PendingCodeAttempts {
	return &PendingCodeAttempts{clock: c, maxKeys: maxPendingKeys, entries: map[string]pendingEntry{}}
}

// Fail registra un código malo de la sesión sessionHash (que vence en
// expires) y devuelve true cuando la sesión agotó sus intentos: quien llama
// la borra. La cuenta de esa sesión se olvida.
func (p *PendingCodeAttempts) Fail(sessionHash string, expires time.Time) (exhausted bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, known := p.entries[sessionHash]
	if !known && len(p.entries) >= p.maxKeys {
		p.makeRoom(p.clock.Now())
	}
	e.failures++
	e.expires = expires
	if e.failures >= MaxPendingCodeAttempts {
		delete(p.entries, sessionHash)
		return true
	}
	p.entries[sessionHash] = e
	return false
}

// Forget olvida la cuenta de una sesión (se canjeó o se borró).
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
