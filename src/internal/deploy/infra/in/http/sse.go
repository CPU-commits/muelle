package http

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/deploy/app/usecases"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// SSEConfig son los límites de los streams (contrato §0.3); los tests los
// acortan.
type SSEConfig struct {
	Heartbeat       time.Duration // cada cuánto va ": ping" (15 s)
	RevalidateEvery int           // cada cuántos heartbeats se revisa la credencial (4: 60 s)
	MaxDuration     time.Duration // lo más que dura un stream (1 h)
	WriteTimeout    time.Duration // el plazo de cada escritura (30 s)
	MaxPerPrincipal int           // los abiertos a la vez por principal, de los dos tipos (8)
}

// DefaultSSEConfig son los límites del contrato §0.3.
func DefaultSSEConfig() SSEConfig {
	return SSEConfig{
		Heartbeat: 15 * time.Second, RevalidateEvery: 4, MaxDuration: time.Hour,
		WriteTimeout: 30 * time.Second, MaxPerPrincipal: 8,
	}
}

// Streams es lo que necesitan los streams además de los casos de uso: el ctx
// de Module.Run (al cancelarse, con la señal de apagado, se cierran todos, así
// srv.Shutdown no los espera), los límites y el contador de streams por
// principal, que se comparte con las herramientas MCP (contrato §8.5). Sin
// Slots, uno nuevo de Config.MaxPerPrincipal.
type Streams struct {
	Ctx    context.Context
	Config SSEConfig
	Slots  *usecases.StreamSlots
}

// sse escribe los eventos de un stream ya abierto (contrato §0.3).
type sse struct {
	w       http.ResponseWriter
	rc      *http.ResponseController
	timeout time.Duration
}

// openSSE abre el stream: quita el plazo de lectura de la conexión (el
// ReadTimeout del servidor, al vencer, cancelaría el ctx de la petición:
// decisión 11 del plan), pone Content-Type text/event-stream, Cache-Control:
// no-cache (pisa el no-store de cmd/http) y X-Accel-Buffering: no, responde
// 200 y hace flush. Después de esto un error ya no se puede responder: solo
// se cierra el stream.
func openSSE(w http.ResponseWriter, cfg SSEConfig) (*sse, error) {
	rc := http.NewResponseController(w)
	if err := rc.SetReadDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return nil, err
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	s := &sse{w: w, rc: rc, timeout: cfg.WriteTimeout}
	if err := s.deadline(); err != nil {
		return nil, err
	}
	w.WriteHeader(http.StatusOK)
	if err := rc.Flush(); err != nil {
		return nil, err
	}
	return s, nil
}

// deadline pone el plazo de la próxima escritura: un cliente que no lee no
// retiene la goroutine. Un ResponseWriter sin plazos (un test) lo ignora.
func (s *sse) deadline() error {
	err := s.rc.SetWriteDeadline(time.Now().Add(s.timeout))
	if errors.Is(err, http.ErrNotSupported) {
		return nil
	}
	return err
}

// singleLine deja s en una línea: un \r o un \n cortarían el evento. Las
// líneas del log ya vienen sin ellos (service.Sanitize); el JSON los escapa.
// Se reemplazan byte por byte, así no cambia el largo.
func singleLine(s string) string {
	if !strings.ContainsAny(s, "\r\n") {
		return s
	}
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}

// event manda un evento con su nombre, su id (si no es "") y un data de una
// sola línea.
func (s *sse) event(name, id, data string) error {
	var b strings.Builder
	b.WriteString("event: ")
	b.WriteString(name)
	b.WriteByte('\n')
	if id != "" {
		b.WriteString("id: ")
		b.WriteString(singleLine(id))
		b.WriteByte('\n')
	}
	b.WriteString("data: ")
	b.WriteString(singleLine(data))
	b.WriteString("\n\n")
	return s.write(b.String())
}

// comment manda un comentario (": ping").
func (s *sse) comment(text string) error { return s.write(": " + singleLine(text) + "\n\n") }

func (s *sse) write(text string) error {
	if err := s.deadline(); err != nil {
		return err
	}
	if _, err := s.w.Write([]byte(text)); err != nil {
		return err
	}
	return s.rc.Flush()
}

// finish quita el plazo de escritura: la conexión puede seguir con otra
// petición (keep-alive, Caddy reusa las conexiones), y sin WriteTimeout el
// servidor no lo vuelve a poner; un plazo viejo cortaría la respuesta siguiente.
func (s *sse) finish() {
	_ = s.rc.SetWriteDeadline(time.Time{})
}

// Las causas con que termina el ctx de un stream, además de la petición.
var (
	errStreamTimeout = errors.New("el stream llegó a su duración máxima")
	errModuleStopped = errors.New("muelle se está apagando")
)

// liveStream es un stream abierto: el SSE, su ctx y el heartbeat que revisa
// la credencial (contrato §0.3).
type liveStream struct {
	*sse
	ctx     context.Context
	cancel  context.CancelCauseFunc
	stopRun func() bool
	timer   *time.Timer
	ticker  *time.Ticker
	release func()
	beats   int

	k      *utils.Kit
	r      *http.Request
	access utils.Access
	cfg    SSEConfig
}

// acquire reserva el lugar del principal de r antes de validar nada: si ya
// tiene MaxPerPrincipal abiertos, responde 429 y devuelve false.
func (h *Handler) acquire(w http.ResponseWriter, r *http.Request) (func(), bool) {
	p, _ := entity.PrincipalFrom(r.Context())
	release, ok := h.limit.Acquire(p)
	if !ok {
		h.k.Fail(w, r, usecases.ErrTooManyStreams)
		return nil, false
	}
	return release, true
}

// open abre el SSE con el lugar ya reservado (release). El ctx del stream se
// cancela con el de la petición, con el de Module.Run y a los MaxDuration.
// Si no pudo abrir, devuelve el lugar.
func (h *Handler) open(w http.ResponseWriter, r *http.Request, release func()) (*liveStream, error) {
	s, err := openSSE(w, h.cfg)
	if err != nil {
		release()
		return nil, err
	}
	ctx, cancel := context.WithCancelCause(r.Context())
	ls := &liveStream{
		sse: s, ctx: ctx, cancel: cancel, release: release,
		stopRun: context.AfterFunc(h.base, func() { cancel(errModuleStopped) }),
		timer:   time.AfterFunc(h.cfg.MaxDuration, func() { cancel(errStreamTimeout) }),
		ticker:  time.NewTicker(h.cfg.Heartbeat),
		k:       h.k, r: r, access: utils.AccessToken, cfg: h.cfg,
	}
	return ls, nil
}

// close termina el stream y devuelve su lugar.
func (ls *liveStream) close() {
	ls.ticker.Stop()
	ls.timer.Stop()
	ls.stopRun()
	ls.cancel(nil)
	ls.finish()
	ls.release()
}

// tick es el canal del heartbeat.
func (ls *liveStream) tick() <-chan time.Time { return ls.ticker.C }

// beat es el heartbeat: cada RevalidateEvery vuelve a validar la credencial
// con que se abrió (Kit.Revalidate), y después manda ": ping". Un error es
// que hay que cerrar: la credencial ya no vale (la sesión se cerró o venció,
// el token se revocó o ya no alcanza), y entonces no se manda nada más; o no
// se pudo escribir.
func (ls *liveStream) beat() error {
	ls.beats++
	if ls.cfg.RevalidateEvery > 0 && ls.beats%ls.cfg.RevalidateEvery == 0 {
		if err := ls.k.Revalidate(ls.r, ls.access); err != nil {
			return err
		}
	}
	return ls.comment("ping")
}

// timedOut dice si el ctx terminó por MaxDuration.
func (ls *liveStream) timedOut() bool { return errors.Is(context.Cause(ls.ctx), errStreamTimeout) }
