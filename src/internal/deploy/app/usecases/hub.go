package usecases

import "sync"

// Hub avisa a los streams que un deploy cambió: líneas nuevas en su log o un
// estado nuevo (contrato §2.6). Wait y Subscribe devuelven un canal que se
// cierra con el próximo Notify de ese deploy; Notify lo cierra y lo olvida.
// No se pierde ningún aviso si quien espera toma el canal antes de leer la
// base: lo que cambie después de esa lectura cierra el canal que ya tiene.
//
// La regla del aviso (para todo lo que cambia un deploy): Notify va recién
// después de que el cambio está en la base. Lo que pasa dentro de un InTx se
// avisa después del commit, a cada id que cambió.
type Hub struct {
	mu    sync.Mutex
	chans map[int64]*hubWait
}

// hubWait es el canal de un deploy y cuántos lo esperan.
type hubWait struct {
	ch   chan struct{}
	refs int
}

func NewHub() *Hub { return &Hub{chans: map[int64]*hubWait{}} }

// Wait es el canal que cierra el próximo Notify(id). Queda en el Hub hasta
// ese Notify; un stream usa Subscribe, que se da de baja.
func (h *Hub) Wait(id int64) <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.entry(id).ch
}

// Subscribe es Wait con su baja: cancel (se puede llamar varias veces) deja
// de esperar y, si nadie más espera ese canal, lo borra. Sin la baja, el
// stream de un deploy que terminó dejaría su canal en el Hub para siempre:
// nadie vuelve a avisar de un deploy cerrado.
func (h *Hub) Subscribe(id int64) (<-chan struct{}, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	w := h.entry(id)
	var once sync.Once
	return w.ch, func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			// Si Notify ya lo cerró, el del mapa es otro (o no hay): no se toca.
			if cur, ok := h.chans[id]; ok && cur == w {
				if w.refs--; w.refs == 0 {
					delete(h.chans, id)
				}
			}
		})
	}
}

// entry es el canal de id, con uno más esperándolo. Se llama con mu tomado.
func (h *Hub) entry(id int64) *hubWait {
	w, ok := h.chans[id]
	if !ok {
		w = &hubWait{ch: make(chan struct{})}
		h.chans[id] = w
	}
	w.refs++
	return w
}

// Notify despierta a todos los que esperan el deploy id.
func (h *Hub) Notify(id int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if w, ok := h.chans[id]; ok {
		close(w.ch)
		delete(h.chans, id)
	}
}

// Pending es cuántos deploys tienen un canal en el Hub: los tests verifican
// que un stream que se fue no dejó el suyo.
func (h *Hub) Pending() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.chans)
}
