package http

import (
	"net/http"
	"strconv"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
)

// streamOffset es desde dónde empieza el stream de un log (contrato §2.6):
// el header Last-Event-ID si viene (y no está vacío: un navegador no lo
// manda vacío); si no, ?offset; si no, 0. Uno inválido es -1 (el caso de uso
// responde los 404 antes del 422).
func streamOffset(r *http.Request) int {
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		return parseOffset(v)
	}
	return ParseOffset(r.URL.Query())
}

// streamLog es GET …/deploys/{id}/log/stream (contrato §2.6): los mismos
// bytes que el polling, en vivo. Los 404 y 422 van como JSON, antes de abrir.
// Después: el status si el deploy no terminó; cada línea como un evento log
// (id = el offset después de ella); un status cada vez que cambia, después
// de las líneas de esa lectura; y el status final, siempre después de la
// última línea, y se cierra. El heartbeat también vuelve a leer: es la red de
// seguridad si se perdiera un aviso del Hub. A la hora (MaxDuration) se
// cierra sin el status final, y el cliente sigue por polling o reconecta con
// Last-Event-ID.
func (h *Handler) streamLog(w http.ResponseWriter, r *http.Request) {
	ref, err := deployRef(r)
	if err != nil {
		h.k.Fail(w, r, err)
		return
	}
	release, ok := h.acquire(w, r)
	if !ok {
		return
	}
	f, err := h.uc.FollowLog.Open(r.Context(), dto.ReadLog{DeployRef: ref, Offset: streamOffset(r)})
	if err != nil {
		release()
		h.k.Fail(w, r, err)
		return
	}
	ls, err := h.open(w, r, release)
	if err != nil {
		return
	}
	defer ls.close()

	sent := f.Status()
	if !sent.Final() {
		if ls.event("status", "", string(sent)) != nil {
			return
		}
	}
	for {
		// Antes de leer: lo que cambie después de la lectura despierta la espera.
		var wait <-chan struct{}
		unsubscribe := func() {}
		if !sent.Final() {
			wait, unsubscribe = f.Subscribe()
		}
		lines, status, err := f.Read(ls.ctx)
		if err != nil {
			unsubscribe()
			return
		}
		for _, l := range lines {
			if ls.event("log", strconv.Itoa(l.Offset), l.Text) != nil {
				unsubscribe()
				return
			}
		}
		if status.Final() {
			// Ya están todas sus líneas: el status final y se cierra.
			unsubscribe()
			_ = ls.event("status", "", string(status))
			return
		}
		if status != sent {
			if ls.event("status", "", string(status)) != nil {
				unsubscribe()
				return
			}
			sent = status
		}
		select {
		case <-wait:
		case <-ls.tick():
			if ls.beat() != nil {
				unsubscribe()
				return
			}
		case <-ls.ctx.Done():
			unsubscribe()
			return
		}
		unsubscribe()
	}
}
