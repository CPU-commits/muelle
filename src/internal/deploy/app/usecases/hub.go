package usecases

import "sync"

// Hub avisa a los streams que un deploy cambió: líneas nuevas en su log o un
// estado nuevo (contrato §2.6). Wait devuelve un canal que se cierra con el
// próximo Notify de ese deploy; Notify lo cierra y lo olvida. No se pierde
// ningún aviso si quien espera llama a Wait antes de leer la base: lo que
// cambie después de esa lectura cierra el canal que ya tiene.
//
// La regla del aviso (para todo lo que cambia un deploy): Notify va recién
// después de que el cambio está en la base. Lo que pasa dentro de un InTx se
// avisa después del commit, a cada id que cambió.
type Hub struct {
	mu    sync.Mutex
	chans map[int64]chan struct{}
}

func NewHub() *Hub { return &Hub{chans: map[int64]chan struct{}{}} }

// Wait es el canal que cierra el próximo Notify(id).
func (h *Hub) Wait(id int64) <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch, ok := h.chans[id]
	if !ok {
		ch = make(chan struct{})
		h.chans[id] = ch
	}
	return ch
}

// Notify despierta a todos los que esperan el deploy id.
func (h *Hub) Notify(id int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ch, ok := h.chans[id]; ok {
		close(ch)
		delete(h.chans, id)
	}
}
