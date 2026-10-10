package service

import (
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/package/clock"
)

// TOTPPeriod es el período de los códigos TOTP: cada paso dura 30 s.
const TOTPPeriod = 30 * time.Second

// maxReplayUsers acota el mapa de pasos usados.
const maxReplayUsers = 10000

// TOTPStep es el paso TOTP (contador de RFC 6238) del instante at.
func TOTPStep(at time.Time) int64 { return at.Unix() / int64(TOTPPeriod/time.Second) }

// TOTPReplayGuard impide usar un código TOTP dos veces (RFC 6238 §5.2):
// guarda en memoria el último paso aceptado de cada usuario y rechaza
// cualquier paso igual o anterior. Es seguro usarlo desde varias goroutines.
type TOTPReplayGuard struct {
	mu       sync.Mutex
	clock    clock.Clock
	maxUsers int
	last     map[int64]int64
}

func NewTOTPReplayGuard(c clock.Clock) *TOTPReplayGuard {
	return &TOTPReplayGuard{clock: c, maxUsers: maxReplayUsers, last: map[int64]int64{}}
}

// Use acepta el paso step de userID si es posterior al último usado, y lo
// registra en la misma operación. Devuelve false si ya se usó ese paso o uno
// posterior.
func (g *TOTPReplayGuard) Use(userID, step int64) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if last, ok := g.last[userID]; ok && step <= last {
		return false
	}
	g.record(userID, step)
	return true
}

// Set registra step como el último paso usado de userID, reemplazando el
// historial: al activar un secreto nuevo, el código con que se activó no
// sirve para entrar, y los pasos del secreto anterior ya no importan.
func (g *TOTPReplayGuard) Set(userID, step int64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.record(userID, step)
}

func (g *TOTPReplayGuard) record(userID, step int64) {
	if _, ok := g.last[userID]; !ok && len(g.last) >= g.maxUsers {
		g.makeRoom()
	}
	g.last[userID] = step
}

// makeRoom se llama con el mapa lleno. Un paso más viejo que el desfase
// tolerado (un período) ya no puede volver a validarse, así que se olvida.
// Si no alcanza, se desaloja cualquiera hasta dejar un cuarto libre.
func (g *TOTPReplayGuard) makeRoom() {
	oldest := TOTPStep(g.clock.Now()) - 1
	for id, step := range g.last {
		if step < oldest {
			delete(g.last, id)
		}
	}
	target := g.maxUsers * 3 / 4
	for id := range g.last {
		if len(g.last) <= target {
			return
		}
		delete(g.last, id)
	}
}
