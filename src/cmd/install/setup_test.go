package install

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
)

const (
	svcName  = "muelle_muelle"
	testPoll = 10 * time.Millisecond
	// errorLine es lo que escribe main cuando muelle no arranca.
	errorLine     = `{"time":"2026-10-15T12:00:00Z","level":"ERROR","msg":"muelle terminó con error","err":"MUELLE_INIT_ACME_EMAIL: \"x\" no es un email válido"}`
	errorShown    = `MUELLE_INIT_ACME_EMAIL: "x" no es un email válido`
	listeningLine = `{"time":"2026-10-15T12:00:01Z","level":"INFO","msg":"muelle escuchando","event":"listening","addr":"[::]:8080"}`
)

func setupLine(url string) string {
	return `{"time":"2026-10-15T12:00:00Z","level":"INFO","msg":"todavía no hay usuarios","event":"setup_url","url":"` + url + `"}`
}

func lines(texts ...string) []docker.LogLine {
	out := make([]docker.LogLine, len(texts))
	for i, s := range texts {
		out[i] = docker.LogLine{Time: time.Now(), Stream: docker.StreamStdout, Text: s}
	}
	return out
}

// t0 es la hora de creación de la primera tarea; las siguientes son más nuevas.
var t0 = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

func task(id, desired, state string, age int) docker.Task {
	return docker.Task{ID: id, DesiredState: desired, State: state, CreatedAt: t0.Add(time.Duration(age) * time.Second)}
}

func running(id string, age int) docker.Task {
	return task(id, docker.TaskRunning, docker.TaskRunning, age)
}

func newSetupFake(tasks ...docker.Task) *dockertest.Fake {
	f := dockertest.New()
	f.PutStackService("muelle", docker.Service{Name: svcName}, tasks...)
	return f
}

// later corre fn después de d, en otra goroutine (como Swarm que cambia la tarea).
func later(t *testing.T, d time.Duration, fn func()) {
	t.Helper()
	timer := time.AfterFunc(d, fn)
	t.Cleanup(func() { timer.Stop() })
}

func wait(t *testing.T, e docker.Services, timeout time.Duration) (string, bool, string, error) {
	t.Helper()
	var stderr bytes.Buffer
	url, users, err := waitSetup(context.Background(), e, svcName, timeout, testPoll, &stderr)
	return url, users, stderr.String(), err
}

func TestWaitSetupURL(t *testing.T) {
	f := newSetupFake(running("t1", 0))
	f.AppendTaskLogs("t1", lines("texto suelto", "{no es json", setupLine("https://p/setup?token=A"), listeningLine)...)
	url, users, _, err := wait(t, f, 2*time.Second)
	if err != nil || users || url != "https://p/setup?token=A" {
		t.Fatalf("url=%q users=%v err=%v", url, users, err)
	}
}

// Solo vale la tarea que corre: la shutdown (con su link viejo) y la que
// todavía no corre no cuentan.
func TestWaitSetupURLCurrentTask(t *testing.T) {
	f := newSetupFake(
		task("t1", docker.TaskShutdown, docker.TaskShutdown, 0),
		running("t2", 1),
		task("t3", docker.TaskRunning, docker.TaskStarting, 2),
	)
	f.AppendTaskLogs("t1", lines(setupLine("https://p/setup?token=A"), listeningLine)...)
	f.AppendTaskLogs("t2", lines(setupLine("https://p/setup?token=B"), listeningLine)...)
	url, _, _, err := wait(t, f, 2*time.Second)
	if err != nil || url != "https://p/setup?token=B" {
		t.Fatalf("url=%q err=%v", url, err)
	}
}

func TestWaitSetupURLTaskReplaced(t *testing.T) {
	f := newSetupFake(running("t1", 0))
	f.AppendTaskLogs("t1", lines(setupLine("https://p/setup?token=A"))...)
	later(t, 100*time.Millisecond, func() {
		f.SetTasks(svcName, task("t1", docker.TaskShutdown, docker.TaskShutdown, 0), running("t2", 1))
		f.AppendTaskLogs("t2", lines(setupLine("https://p/setup?token=B"), listeningLine)...)
	})
	url, _, _, err := wait(t, f, 2*time.Second)
	if err != nil || url != "https://p/setup?token=B" {
		t.Fatalf("url=%q err=%v", url, err)
	}
}

func TestWaitSetupURLNoTaskYet(t *testing.T) {
	f := newSetupFake()
	later(t, 100*time.Millisecond, func() {
		f.SetTasks(svcName, running("t1", 0))
		f.AppendTaskLogs("t1", lines(setupLine("https://p/setup?token=A"), listeningLine)...)
	})
	url, _, _, err := wait(t, f, 2*time.Second)
	if err != nil || url != "https://p/setup?token=A" {
		t.Fatalf("url=%q err=%v", url, err)
	}
}

func TestWaitSetupURLHasUsers(t *testing.T) {
	f := newSetupFake(running("t1", 0))
	f.AppendTaskLogs("t1", lines(listeningLine)...)
	url, users, _, err := wait(t, f, 2*time.Second)
	if err != nil || !users || url != "" {
		t.Fatalf("url=%q users=%v err=%v", url, users, err)
	}
}

func TestWaitSetupURLShowsErrors(t *testing.T) {
	f := newSetupFake(running("t1", 0))
	f.AppendTaskLogs("t1", lines("texto", errorLine)...)
	_, _, stderr, err := wait(t, f, 300*time.Millisecond)
	if !errors.Is(err, ErrSetupTimeout) {
		t.Fatalf("err=%v", err)
	}
	if strings.Count(stderr, errorShown) != 1 || !strings.Contains(stderr, "t1") {
		t.Fatalf("stderr=%q", stderr)
	}
}

// muelle que sale enseguida (una clave equivocada) deja la tarea failed en
// milisegundos: casi nunca se la ve corriendo. install muestra su Err y las
// líneas ERROR de su log, una sola vez.
func TestWaitSetupURLFailedTask(t *testing.T) {
	failed := task("t1", docker.TaskShutdown, docker.TaskFailed, 0)
	failed.ContainerID, failed.Err = "c1", "task: non-zero exit (1)"
	f := newSetupFake(failed)
	f.AppendTaskLogs("t1", lines(errorLine)...)
	_, _, stderr, err := wait(t, f, 300*time.Millisecond)
	if !errors.Is(err, ErrSetupTimeout) {
		t.Fatalf("err=%v", err)
	}
	warn := "aviso: la tarea t1 de " + svcName + " está failed: task: non-zero exit (1)"
	if strings.Count(stderr, warn) != 1 || strings.Count(stderr, errorShown) != 1 {
		t.Fatalf("stderr=%q", stderr)
	}
	if n := countCalls(f, "TaskLogs t1"); n != 1 {
		t.Fatalf("leyó el log de la tarea terminada %d veces", n)
	}
}

// Una tarea rejected (el bind de docker.sock no existe, con Docker rootless)
// nunca tuvo contenedor: el motivo está solo en su Err.
func TestWaitSetupURLRejectedTask(t *testing.T) {
	rejected := task("t1", docker.TaskShutdown, docker.TaskRejected, 0)
	rejected.Err = `invalid mount config for type "bind": bind source path does not exist: /var/run/docker.sock`
	f := newSetupFake(rejected)
	_, _, stderr, err := wait(t, f, 300*time.Millisecond)
	if !errors.Is(err, ErrSetupTimeout) {
		t.Fatalf("err=%v", err)
	}
	if strings.Count(stderr, "rejected: "+rejected.Err) != 1 {
		t.Fatalf("stderr=%q", stderr)
	}
	if n := countCalls(f, "TaskLogs"); n != 0 {
		t.Fatalf("pidió el log de una tarea sin contenedor (%d veces)", n)
	}
}

// Una tarea pending sin Err no dice nada; cuando aparece el Err (no hay un
// nodo posible), sale.
func TestWaitSetupURLPendingErrLater(t *testing.T) {
	f := newSetupFake(task("t1", docker.TaskRunning, docker.TaskPending, 0))
	later(t, 100*time.Millisecond, func() {
		p := task("t1", docker.TaskRunning, docker.TaskPending, 0)
		p.Err = "no suitable node (scheduling constraints not satisfied on 1 node)"
		f.SetTasks(svcName, p)
	})
	_, _, stderr, err := wait(t, f, 300*time.Millisecond)
	if !errors.Is(err, ErrSetupTimeout) {
		t.Fatalf("err=%v", err)
	}
	if strings.Count(stderr, "está pending: no suitable node") != 1 || strings.Count(stderr, "aviso:") != 1 {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestWaitSetupURLFailedThenRunning(t *testing.T) {
	failed := task("t1", docker.TaskShutdown, docker.TaskFailed, 0)
	failed.ContainerID, failed.Err = "c1", "task: non-zero exit (1)"
	f := newSetupFake(failed)
	f.AppendTaskLogs("t1", lines(errorLine)...)
	later(t, 100*time.Millisecond, func() {
		f.SetTasks(svcName, failed, running("t2", 1))
		f.AppendTaskLogs("t2", lines(setupLine("https://p/setup?token=B"), listeningLine)...)
	})
	url, _, stderr, err := wait(t, f, 2*time.Second)
	if err != nil || url != "https://p/setup?token=B" {
		t.Fatalf("url=%q err=%v", url, err)
	}
	if strings.Count(stderr, errorShown) != 1 {
		t.Fatalf("stderr=%q", stderr)
	}
}

// endsOnce corta el primer stream de la tarea: le pide al fake el mismo log
// sin Follow, que termina con io.EOF después de las líneas que ya hay, y
// avisa por ended cuando se cierra (como el daemon que corta el stream).
type endsOnce struct {
	*dockertest.Fake
	task  string
	once  sync.Once
	ended chan struct{}
}

func (e *endsOnce) TaskLogs(ctx context.Context, id string, o docker.LogOptions) (docker.LogStream, error) {
	first := false
	if id == e.task && o.Follow {
		e.once.Do(func() { first = true })
	}
	if !first {
		return e.Fake.TaskLogs(ctx, id, o)
	}
	o.Follow = false
	s, err := e.Fake.TaskLogs(ctx, id, o)
	if err != nil {
		return nil, err
	}
	return &signalOnClose{LogStream: s, ended: e.ended}, nil
}

type signalOnClose struct {
	docker.LogStream
	once  sync.Once
	ended chan struct{}
}

func (s *signalOnClose) Close() error {
	s.once.Do(func() { close(s.ended) })
	return s.LogStream.Close()
}

// Si el stream de la tarea actual se corta antes de listening, se vuelve a
// leer entero: el link es el de la lectura que llega a listening, y una línea
// ERROR no se repite.
func TestWaitSetupURLStreamEnds(t *testing.T) {
	f := newSetupFake(running("t1", 0))
	f.AppendTaskLogs("t1", lines(setupLine("https://p/setup?token=A"), errorLine)...)
	e := &endsOnce{Fake: f, task: "t1", ended: make(chan struct{})}
	go func() {
		<-e.ended
		f.AppendTaskLogs("t1", lines(listeningLine)...)
	}()
	url, users, stderr, err := wait(t, e, 2*time.Second)
	if err != nil || users || url != "https://p/setup?token=A" {
		t.Fatalf("url=%q users=%v err=%v", url, users, err)
	}
	if n := countCalls(f, "TaskLogs t1"); n != 2 {
		t.Fatalf("TaskLogs t1 %d veces, esperaba 2: %q", n, f.Calls())
	}
	if strings.Count(stderr, errorShown) != 1 {
		t.Fatalf("stderr=%q", stderr)
	}
}

func TestWaitSetupURLTimeout(t *testing.T) {
	f := newSetupFake(running("t1", 0))
	start := time.Now()
	_, _, _, err := wait(t, f, 200*time.Millisecond)
	if !errors.Is(err, ErrSetupTimeout) {
		t.Fatalf("err=%v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("tardó %s", d)
	}
	for _, want := range []string{"docker service logs " + svcName, "docker service ps --no-trunc " + svcName} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("el error no sugiere %q: %v", want, err)
		}
	}
}

// Un Ctrl-C mientras espera no es el plazo: devuelve el error del ctx.
func TestWaitSetupURLCanceled(t *testing.T) {
	f := newSetupFake(running("t1", 0))
	ctx, cancel := context.WithCancel(context.Background())
	later(t, 50*time.Millisecond, cancel)
	_, _, err := waitSetup(ctx, f, svcName, 2*time.Second, testPoll, io.Discard)
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrSetupTimeout) {
		t.Fatalf("err=%v", err)
	}
}

func countCalls(f *dockertest.Fake, prefix string) int {
	n := 0
	for _, c := range f.Calls() {
		if c == prefix || strings.HasPrefix(c, prefix+" ") {
			n++
		}
	}
	return n
}
