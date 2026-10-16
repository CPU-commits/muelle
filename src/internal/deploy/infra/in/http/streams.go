package http

import (
	jsonv2 "encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

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

// appLogRes es el data de un evento log de GET …/logs (contrato §2.9), en
// JSON compacto.
type appLogRes struct {
	Time   *time.Time `json:"time"`
	Task   *string    `json:"task"`
	Stream string     `json:"stream"`
	Line   string     `json:"line"`
}

// optional es el valor de key en la query, o nil si no viene.
func optional(q url.Values, key string) *string {
	if !q.Has(key) {
		return nil
	}
	v := q.Get(key)
	return &v
}

// appLogs es GET …/apps/{a}/logs (contrato §2.9): los logs de un servicio en
// vivo. Los 404, 422 y 503 van como JSON, antes de abrir. Después: un evento
// log por línea; end con data eof cuando el log termina (sin follow, o el
// servicio se borró) y end con data timeout a la hora (MaxDuration), y se
// cierra. Si el log de Docker se corta por otra cosa, se cierra sin end: el
// panel reconecta con since.
func (h *Handler) appLogs(w http.ResponseWriter, r *http.Request) {
	release, ok := h.acquire(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	in := dto.AppLogsRequest{
		AppRef: appRef(r), Service: q.Get("service"),
		Tail: optional(q, "tail"), Since: optional(q, "since"), Follow: optional(q, "follow"),
	}
	logs, err := h.uc.AppLogs.Open(r.Context(), in)
	if err != nil {
		release()
		h.k.Fail(w, r, err)
		return
	}
	defer logs.Close()
	ls, err := h.open(w, r, release)
	if err != nil {
		return
	}
	defer ls.close()
	for {
		select {
		case l, ok := <-logs.Lines():
			if !ok {
				if errors.Is(logs.Err(), io.EOF) {
					_ = ls.event("end", "", "eof")
				}
				return
			}
			data, err := jsonv2.Marshal(appLogRes{Time: l.Time, Task: l.Task, Stream: l.Stream, Line: l.Line})
			if err != nil || ls.event("log", "", string(data)) != nil {
				return
			}
		case <-ls.tick():
			if ls.beat() != nil {
				return
			}
		case <-ls.ctx.Done():
			if ls.timedOut() {
				_ = ls.event("end", "", "timeout")
			}
			return
		}
	}
}
