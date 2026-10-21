package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

// ErrSetupTimeout: muelle no llegó a escribir el link de setup (ni a
// escuchar) dentro del plazo. El stack queda desplegado.
var ErrSetupTimeout = errors.New("muelle no mostró el link de setup a tiempo")

// Los eventos del log de muelle que lee install (main.go: logSetupURL y
// logListening). El mensaje de esas líneas puede cambiar; event, url y panel
// no.
const (
	eventSetupURL  = "setup_url"
	eventListening = "listening"
	levelError     = "ERROR"
)

// finalStates son los estados de una tarea que ya no va a correr.
var finalStates = map[string]bool{
	docker.TaskFailed: true, docker.TaskRejected: true, docker.TaskShutdown: true,
	docker.TaskComplete: true, docker.TaskOrphaned: true,
}

// setupWait guarda lo que ya se mostró, para que nada salga dos veces
// aunque el bucle pase muchas veces por la misma tarea.
type setupWait struct {
	e       docker.Services
	service string
	poll    time.Duration
	stderr  io.Writer

	warned   map[string]bool // los aviso:, por id de tarea
	logRead  map[string]bool // el log de una tarea que ya terminó, leído entero
	errLines map[string]bool // las líneas ERROR escritas, por tarea y texto
}

// setupResult es lo que install lee del arranque de muelle: el último
// setup_url de la tarea actual (URL), o HasUsers si no hubo ninguno, y el
// origen del panel que dice listening (Panel; "" con un muelle que no lo
// escribe).
type setupResult struct {
	URL, Panel string
	HasUsers   bool
}

// waitSetup sigue el log de la tarea actual del servicio (la running más
// nueva) hasta "event":"listening". Devuelve el último setup_url de esa
// tarea, o HasUsers si no hubo ninguno, y el origen del panel. Si la tarea actual cambia, cierra el
// stream y lee la nueva desde el principio; si el stream se corta antes de
// listening, vuelve a leer la misma desde el principio. Mientras no hay una tarea
// corriendo, muestra en stderr por qué: el estado y el Err de la tarea más
// nueva, y las líneas ERROR de su log si ya terminó. Las líneas de nivel
// ERROR de cualquier tarea van a stderr, una vez cada una (contrato §5.4,
// "El link de setup").
func waitSetup(ctx context.Context, e docker.Services, service string, timeout, poll time.Duration, stderr io.Writer) (setupResult, error) {
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	w := &setupWait{e: e, service: service, poll: poll, stderr: stderr,
		warned: map[string]bool{}, logRead: map[string]bool{}, errLines: map[string]bool{}}
	for {
		// Todas las tareas, de la más nueva a la más vieja (sin DesiredState:
		// también las que fallaron). Un error (el servicio que Swarm todavía
		// no muestra, Docker que no responde un momento) se reintenta.
		if tasks, err := e.ListTasks(ctx, docker.TaskFilter{Service: service}); err == nil {
			if cur, ok := currentTask(tasks); ok {
				if res, done := w.follow(ctx, cur.ID); done {
					return res, nil
				}
			} else if len(tasks) > 0 {
				w.explain(ctx, tasks[0])
			}
		}
		select {
		case <-ctx.Done():
		case <-time.After(poll):
		}
		if ctx.Err() != nil {
			break
		}
	}
	if err := parent.Err(); err != nil {
		return setupResult{}, err // Ctrl-C: no es el plazo
	}
	return setupResult{}, fmt.Errorf("%w: revisa docker service logs %s y docker service ps --no-trunc %s (muestra el error entero de cada tarea)",
		ErrSetupTimeout, service, service)
}

// currentTask es la tarea que corre ahora: la más nueva con desired running
// y state running (tasks viene de la más nueva a la más vieja).
func currentTask(tasks []docker.Task) (docker.Task, bool) {
	for _, t := range tasks {
		if t.DesiredState == docker.TaskRunning && t.State == docker.TaskRunning {
			return t, true
		}
	}
	return docker.Task{}, false
}

// follow lee el log de la tarea desde el principio, con Follow, hasta
// listening (done). Una goroutine cierra el stream si la tarea actual
// cambia; si el stream termina antes de listening, done es false y el bucle
// de waitSetup decide qué leer.
func (w *setupWait) follow(ctx context.Context, taskID string) (res setupResult, done bool) {
	ctx, cancel := context.WithCancel(ctx)
	stream, err := w.e.TaskLogs(ctx, taskID, docker.LogOptions{Tail: -1, Follow: true})
	if err != nil {
		cancel()
		return setupResult{}, false
	}
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		w.watch(ctx, taskID, stream)
	}()
	defer func() {
		cancel()
		<-watched // la goroutine no llama a Docker después de que follow vuelve
		stream.Close()
	}()
	for {
		line, err := stream.Next()
		if err != nil {
			return setupResult{}, false
		}
		ev, ok := decodeLogLine(line.Text)
		if !ok {
			continue
		}
		if ev.level == levelError {
			w.showError(taskID, line.Text, ev)
		}
		switch ev.event {
		case eventSetupURL:
			if ev.url != "" {
				res.URL = ev.url
			}
		case eventListening:
			res.Panel, res.HasUsers = ev.panel, res.URL == ""
			return res, true
		}
	}
}

// watch cierra el stream si la tarea actual deja de ser taskID, o si se
// cancela ctx (el plazo, o follow que terminó).
func (w *setupWait) watch(ctx context.Context, taskID string, stream docker.LogStream) {
	tick := time.NewTicker(w.poll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			stream.Close()
			return
		case <-tick.C:
		}
		tasks, err := w.e.ListTasks(ctx, docker.TaskFilter{Service: w.service})
		if err != nil {
			continue
		}
		if cur, ok := currentTask(tasks); !ok || cur.ID != taskID {
			stream.Close()
			return
		}
	}
}

// explain dice por qué no corre la tarea más nueva: su estado y su Err (una
// vez por tarea, cuando hay algo que decir) y, si tuvo contenedor y terminó,
// las líneas ERROR de su log (leído una sola vez: ya no cambia).
func (w *setupWait) explain(ctx context.Context, t docker.Task) {
	final := finalStates[t.State]
	if (t.Err != "" || final) && !w.warned[t.ID] {
		w.warned[t.ID] = true
		msg := fmt.Sprintf("aviso: la tarea %s de %s está %s", t.ID, w.service, t.State)
		if t.Err != "" {
			msg += ": " + t.Err
		}
		fmt.Fprintln(w.stderr, msg)
	}
	if t.ContainerID == "" || !final || w.logRead[t.ID] {
		return
	}
	stream, err := w.e.TaskLogs(ctx, t.ID, docker.LogOptions{Tail: -1})
	if err != nil {
		return // se reintenta en la próxima vuelta
	}
	defer stream.Close()
	for {
		line, err := stream.Next()
		if errors.Is(err, io.EOF) {
			w.logRead[t.ID] = true
			return
		}
		if err != nil {
			return
		}
		if ev, ok := decodeLogLine(line.Text); ok && ev.level == levelError {
			w.showError(t.ID, line.Text, ev)
		}
	}
}

// showError escribe una línea ERROR de muelle en stderr, una sola vez por
// tarea y texto.
func (w *setupWait) showError(taskID, raw string, ev logEvent) {
	key := taskID + "\x00" + raw
	if w.errLines[key] {
		return
	}
	w.errLines[key] = true
	msg := ev.msg
	if ev.err != "" {
		msg += ": " + ev.err
	}
	fmt.Fprintf(w.stderr, "error de muelle (tarea %s): %s\n", taskID, msg)
}

// logEvent son los campos que install lee de una línea JSON del log de muelle.
type logEvent struct {
	level, msg, err, event, url, panel string
}

func decodeLogLine(text string) (logEvent, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "{") {
		return logEvent{}, false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		return logEvent{}, false
	}
	str := func(k string) string {
		s, _ := m[k].(string)
		return s
	}
	return logEvent{level: str("level"), msg: str("msg"), err: str("err"), event: str("event"), url: str("url"),
		panel: str("panel")}, true
}
