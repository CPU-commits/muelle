package usecases

import (
	"errors"
	"strconv"
	"sync"

	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// ErrTooManyStreams: el principal ya tiene todos sus lugares de streams
// ocupados (contrato §0.3 y §8.5): 429 rate_limited en REST, antes de abrir;
// rate_limited en MCP.
var ErrTooManyStreams = errors.New("demasiados streams abiertos")

// StreamSlots cuenta los streams abiertos por principal ("s:"+SessionHash o
// "t:"+id del token): los SSE de deploys y de logs, y por MCP get_app_logs y
// get_deploy con wait_s, todos con el mismo contador (contrato §8.5).
type StreamSlots struct {
	mu   sync.Mutex
	max  int
	open map[string]int
}

// NewStreamSlots arma el contador con max lugares por principal.
func NewStreamSlots(max int) *StreamSlots { return &StreamSlots{max: max, open: map[string]int{}} }

func principalKey(p entity.Principal) string {
	switch {
	case p.Token != nil:
		return "t:" + strconv.FormatInt(p.Token.ID, 10)
	case p.SessionHash != "":
		return "s:" + p.SessionHash
	}
	// Una sesión sin hash no la produce identity; por las dudas, el usuario.
	return "u:" + strconv.FormatInt(p.UserID, 10)
}

// Acquire reserva un lugar para p; release lo devuelve (se puede llamar
// varias veces). ok es false si p ya tiene max abiertos.
func (s *StreamSlots) Acquire(p entity.Principal) (release func(), ok bool) {
	key := principalKey(p)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.open[key] >= s.max {
		return nil, false
	}
	s.open[key]++
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.open[key]--; s.open[key] <= 0 {
				delete(s.open, key)
			}
		})
	}, true
}
