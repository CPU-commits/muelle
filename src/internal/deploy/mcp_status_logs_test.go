package deploy

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/mcp/tools"
	"github.com/CPU-commits/muelle/src/cmd/mcp/tools/toolstest"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	depmcp "github.com/CPU-commits/muelle/src/internal/deploy/infra/in/mcp"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// mcpTest es el módulo con sus herramientas en un tools.Kit (con los errores
// de la API) y el servidor HTTP de los tests de streams.
type mcpTest struct {
	*streamTest
	kit *tools.Kit
}

// newMCPTest arma el módulo con env, deja que configure cambie los límites
// de las herramientas y las registra.
func newMCPTest(t *testing.T, env service.EnvResolver, configure func(*depmcp.Config)) *mcpTest {
	t.Helper()
	h := newModuleTest(t, env)
	if configure != nil {
		configure(&h.m.mcp)
	}
	st := newStreamTestOn(t, h, testSSE())
	k := toolstest.NewKit(t, h.m.RegisterRoutes)
	h.m.RegisterTools(k)
	return &mcpTest{streamTest: st, kit: k}
}

// tp es el token "tp" de los tests de HTTP (id 4, del proyecto auditfy).
var tp = entity.Principal{Token: &entity.TokenScope{ID: 4, Name: "github-actions", ProjectSlug: "auditfy"}}

func (h *mcpTest) session(t *testing.T) *toolstest.Session { return toolstest.Connect(t, h.kit, tp) }

var apiArgs = map[string]any{"project": "auditfy", "stage": "staging", "app": "api"}

func args(extra map[string]any) map[string]any {
	out := map[string]any{"project": "auditfy", "stage": "staging", "app": "api"}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

type statusResult struct {
	Services []struct {
		Name  string `json:"name"`
		Tasks []struct {
			ID           string  `json:"id"`
			DesiredState string  `json:"desired_state"`
			Since        *string `json:"since"`
			Error        *string `json:"error"`
		} `json:"tasks"`
		TasksOmitted int `json:"tasks_omitted"`
	} `json:"services"`
	Warning string `json:"warning"`
}

// get_app_status: la forma de §2.8 con el id a 12, las vigentes y las
// history históricas más nuevas (tasks_omitted), y el error enmascarado y
// cortado a 300 caracteres.
func TestMCPGetAppStatus(t *testing.T) {
	const secret = "s3cr3t-del-command-1234"
	h := newMCPTest(t, secretEnv{secret}, nil)
	h.seed(t)
	at := time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC)
	tasks := []docker.Task{{ID: "abcdefghijklmnopqrst", Slot: 1, State: "running", DesiredState: "running",
		NodeID: "node-1", UpdatedAt: at.Add(time.Hour)}}
	for i := range 5 {
		tasks = append(tasks, docker.Task{ID: fmt.Sprintf("old%d", i), Slot: 1, State: "failed", DesiredState: "shutdown",
			Err: "falló con " + secret + strings.Repeat(" ñ", 200), UpdatedAt: at.Add(time.Duration(i) * time.Minute)})
	}
	one := uint64(1)
	h.f.PutStackService(naming.Stack("auditfy", "staging", "api"), docker.Service{
		Name: appService, Labels: naming.Labels("auditfy", "staging", "api", 1), Image: "ghcr.io/x/api:1", Replicas: &one,
	}, tasks...)
	s := h.session(t)

	text, isErr := s.Call("get_app_status", apiArgs)
	if isErr || strings.Contains(text, secret) {
		t.Fatalf("get_app_status: %s", text)
	}
	out := toolstest.Decode[statusResult](t, text)
	svc := out.Services[0]
	if len(out.Services) != 1 || svc.Name != "api" || len(svc.Tasks) != 4 || svc.TasksOmitted != 2 || out.Warning == "" {
		t.Fatalf("history 3: %s", text)
	}
	if svc.Tasks[0].ID != "abcdefghijkl" || svc.Tasks[0].DesiredState != "running" {
		t.Fatalf("la vigente: %+v", svc.Tasks[0])
	}
	// Las históricas que quedan son las tres más nuevas (old4, old3, old2).
	got := []string{svc.Tasks[1].ID, svc.Tasks[2].ID, svc.Tasks[3].ID}
	if strings.Join(got, ",") != "old4,old3,old2" {
		t.Fatalf("históricas: %v", got)
	}
	e := *svc.Tasks[1].Error
	if !strings.HasPrefix(e, "falló con *** ñ") || len([]rune(e)) != depmcp.MaxErrorChars || !strings.HasSuffix(e, "…") {
		t.Fatalf("error: %q (%d)", e, len([]rune(e)))
	}

	out = toolstest.OK[statusResult](s, "get_app_status", args(map[string]any{"history": 0}))
	if len(out.Services[0].Tasks) != 1 || out.Services[0].TasksOmitted != 5 {
		t.Fatalf("history 0: %+v", out)
	}
	if code, _ := toolstest.Fail(s, "get_app_status", args(map[string]any{"app": "nada"})); code != "not_found" {
		t.Fatalf("app que no existe: %s", code)
	}
}

// Peor caso: 32 servicios de 50 tareas con errores de 300 caracteres llenos
// de ESC entran en el tope sin las históricas y después sin parte de las
// vigentes; los servicios salen todos.
func TestMCPGetAppStatusWorstCase(t *testing.T) {
	h := newMCPTest(t, fakeEnv{}, nil)
	h.seed(t)
	stack := naming.Stack("auditfy", "staging", "api")
	at := time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC)
	for svc := range 32 {
		var tasks []docker.Task
		for i := range 50 {
			desired := "running"
			if i >= 20 {
				desired = "shutdown"
			}
			tasks = append(tasks, docker.Task{ID: fmt.Sprintf("t%02d-%02d-%s", svc, i, strings.Repeat("x", 20)), Slot: i + 1,
				State: "failed", DesiredState: desired, Err: strings.Repeat("\x1b[31m", 60), UpdatedAt: at.Add(time.Duration(i) * time.Second)})
		}
		replicas := uint64(20)
		h.f.PutStackService(stack, docker.Service{
			Name: naming.Service("auditfy", "staging", "api", fmt.Sprintf("s%02d", svc)), Labels: naming.AppLabels("auditfy", "staging", "api"),
			Image: "ghcr.io/x/" + strings.Repeat("y", 400) + ":1", Replicas: &replicas,
		}, tasks...)
	}
	text, isErr := h.session(t).Call("get_app_status", args(map[string]any{"history": 10}))
	if isErr || len(text) > tools.MaxResultBytes {
		t.Fatalf("isErr=%v, %d bytes: %.300s", isErr, len(text), text)
	}
	out := toolstest.Decode[statusResult](t, text)
	tasks, omitted := 0, 0
	for _, s := range out.Services {
		tasks += len(s.Tasks)
		omitted += s.TasksOmitted
		for _, task := range s.Tasks {
			if task.DesiredState != "running" {
				t.Fatalf("quedó una histórica: %+v", task)
			}
		}
	}
	if len(out.Services) != 32 || tasks == 0 || tasks+omitted != 32*50 || tasks >= 32*20 {
		t.Fatalf("%d servicios, %d tareas, %d omitidas", len(out.Services), tasks, omitted)
	}
	// Las vigentes se van de a una por servicio en cada vuelta, desde el
	// último servicio y, en cada uno, desde el final del orden de §2.8 (acá,
	// por slot): cada servicio conserva sus primeras k o k+1, y las k+1 son
	// de los primeros servicios.
	first, last := len(out.Services[0].Tasks), len(out.Services[31].Tasks)
	for svc, s := range out.Services {
		for i, task := range s.Tasks {
			if want := fmt.Sprintf("t%02d-%02d-", svc, i); !strings.HasPrefix(task.ID, want) {
				t.Fatalf("servicio %d, tarea %d: %s, esperaba %s…", svc, i, task.ID, want)
			}
		}
		if s.TasksOmitted != 50-len(s.Tasks) {
			t.Fatalf("servicio %d: %d tareas y %d omitidas", svc, len(s.Tasks), s.TasksOmitted)
		}
		if svc > 0 && len(s.Tasks) > len(out.Services[svc-1].Tasks) {
			t.Fatalf("el servicio %d tiene más tareas (%d) que el anterior (%d)", svc, len(s.Tasks), len(out.Services[svc-1].Tasks))
		}
	}
	if first == 0 || first-last > 1 {
		t.Fatalf("de %d a %d tareas por servicio", first, last)
	}
}

func (h *mcpTest) putAPIService() {
	one := uint64(1)
	h.f.PutStackService(naming.Stack("auditfy", "staging", "api"), docker.Service{
		Name: appService, Labels: naming.Labels("auditfy", "staging", "api", 1), Replicas: &one,
	}, docker.Task{ID: "t1aaaaaaaaaa", Slot: 1, State: "running", DesiredState: "running"})
}

type logsResult struct {
	Service      string   `json:"service"`
	Lines        []string `json:"lines"`
	Returned     int      `json:"returned"`
	OmittedOlder int      `json:"omitted_older"`
	Incomplete   bool     `json:"incomplete"`
	Warning      string   `json:"warning"`
}

// rawLines es lo que mide lines en el JSON que salió.
func rawLines(t *testing.T, text string) int {
	t.Helper()
	return len(toolstest.Decode[struct {
		Lines jsontext.Value `json:"lines"`
	}](t, text).Lines)
}

// get_app_logs: tail, since (RFC 3339 y una duración), stream y contains
// después del enmascarado.
func TestMCPGetAppLogs(t *testing.T) {
	const secret = "s3cr3t-de-la-app-1234"
	h := newMCPTest(t, secretEnv{secret}, nil)
	h.seed(t)
	h.putAPIService()
	at := h.clock.Now().Add(-95 * time.Minute)
	var lines []docker.LogLine
	for i := range 10 {
		stream := "stdout"
		if i%2 == 1 {
			stream = "stderr"
		}
		lines = append(lines, docker.LogLine{Time: at.Add(time.Duration(i) * 10 * time.Minute), Stream: stream, TaskID: "t1aaaaaaaaaa",
			Text: fmt.Sprintf("línea %d token=%s", i, secret)})
	}
	h.f.AppendServiceLogs(appService, lines...)
	s := h.session(t)

	text, isErr := s.Call("get_app_logs", args(map[string]any{"tail": 3}))
	out := toolstest.Decode[logsResult](t, text)
	if isErr || strings.Contains(text, secret) || out.Service != "api" || out.Returned != 3 || out.OmittedOlder != 0 || out.Incomplete ||
		out.Warning == "" || out.Lines[0] != at.Add(70*time.Minute).Format(time.RFC3339Nano)+" api.1 stderr línea 7 token=***" {
		t.Fatalf("tail 3: %s", text)
	}
	if !strings.HasSuffix(out.Lines[2], " api.1 stderr línea 9 token=***") {
		t.Fatalf("la más nueva: %q", out.Lines[2])
	}
	// since con una duración, desde la hora del reloj: las de los últimos 25
	// minutos (la última es de hace 5). since reemplaza a tail.
	out = toolstest.OK[logsResult](s, "get_app_logs", args(map[string]any{"since": "25m", "tail": 1}))
	if out.Returned != 3 {
		t.Fatalf("since 25m: %+v", out)
	}
	since := at.Add(80 * time.Minute).Format(time.RFC3339Nano)
	if out = toolstest.OK[logsResult](s, "get_app_logs", args(map[string]any{"since": since})); out.Returned != 2 {
		t.Fatalf("since %s: %+v", since, out)
	}
	if out = toolstest.OK[logsResult](s, "get_app_logs", args(map[string]any{"stream": "stderr"})); out.Returned != 5 {
		t.Fatalf("stream stderr: %+v", out)
	}
	if out = toolstest.OK[logsResult](s, "get_app_logs", args(map[string]any{"contains": "LÍNEA 3"})); out.Returned != 1 {
		t.Fatalf("contains: %+v", out)
	}
	// Buscar el secreto da 0 líneas: el filtro va después del enmascarado.
	if out = toolstest.OK[logsResult](s, "get_app_logs", args(map[string]any{"contains": "s3cr3t"})); out.Returned != 0 {
		t.Fatalf("contains del secreto: %+v", out)
	}
	if out = toolstest.OK[logsResult](s, "get_app_logs", args(map[string]any{"contains": "***"})); out.Returned != 10 {
		t.Fatalf("contains ***: %+v", out)
	}

	for _, c := range []struct {
		args map[string]any
		msg  string
	}{
		{map[string]any{"since": "ayer"}, "since: "},
		{map[string]any{"since": "-5m"}, "since: "},
		{map[string]any{"since": "169h"}, "since: "},
		{map[string]any{"contains": strings.Repeat("ñ", 201)}, "contains: es de hasta 200 caracteres"},
		{map[string]any{"stream": "stdin"}, "stream: debe ser stdout o stderr"},
	} {
		code, msg := toolstest.Fail(s, "get_app_logs", args(c.args))
		if code != "invalid" || !strings.HasPrefix(msg, c.msg) {
			t.Errorf("%v: %s %q", c.args, code, msg)
		}
	}
	if code, msg := toolstest.Fail(s, "get_app_logs", args(map[string]any{"app": "infra"})); code != "not_found" ||
		msg != "el servicio no existe en la app; en una app compose, service es obligatorio (los servicios están en get_app)" {
		t.Errorf("compose sin service: %s %q", code, msg)
	}
	if code, msg := toolstest.Fail(s, "get_app_logs", args(map[string]any{"app": "nada"})); code != "not_found" || msg != "no existe" {
		t.Errorf("app que no existe: %s %q", code, msg)
	}
	// Primero la app y después los argumentos, como en REST (§2.9): una app
	// que no existe es not_found aunque since, contains o stream no valgan.
	for _, bad := range []map[string]any{{"since": "ayer"}, {"contains": strings.Repeat("ñ", 201)}, {"stream": "stdin"}} {
		bad["app"] = "nada"
		if code, msg := toolstest.Fail(s, "get_app_logs", args(bad)); code != "not_found" || msg != "no existe" {
			t.Errorf("app que no existe con %v: %s %q", bad, code, msg)
		}
	}
}

// El buffer descarta las más viejas al pasar max_bytes, medido en JSON, y
// cada línea, con su prefijo, se corta a 2 KiB de JSON. Peor caso: 1000
// líneas llenas de colores ANSI con max_bytes 32768.
func TestMCPGetAppLogsBuffer(t *testing.T) {
	h := newMCPTest(t, fakeEnv{}, nil)
	h.seed(t)
	h.putAPIService()
	at := time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC)
	var lines []docker.LogLine
	for i := range 1000 {
		lines = append(lines, docker.LogLine{Time: at.Add(time.Duration(i) * time.Millisecond), Stream: "stdout", TaskID: "t1aaaaaaaaaa",
			Text: fmt.Sprintf("%04d ", i) + strings.Repeat("\x1b[31mrojo\x1b[0m \"\\", 100)})
	}
	h.f.AppendServiceLogs(appService, lines...)
	s := h.session(t)
	text, isErr := s.Call("get_app_logs", args(map[string]any{"tail": 1000, "max_bytes": 32768}))
	out := toolstest.Decode[logsResult](t, text)
	if isErr || len(text) > tools.MaxResultBytes || rawLines(t, text) > 32768 || out.Returned+out.OmittedOlder != 1000 || out.OmittedOlder == 0 {
		t.Fatalf("isErr=%v, %d bytes, lines %d bytes, %d + %d", isErr, len(text), rawLines(t, text), out.Returned, out.OmittedOlder)
	}
	for _, l := range out.Lines {
		if tools.QuoteSize(l) > depmcp.MaxLogLineBytes || !strings.HasSuffix(l, "…") {
			t.Fatalf("una línea mide %d en JSON: %.80q", tools.QuoteSize(l), l)
		}
	}
	if !strings.Contains(out.Lines[len(out.Lines)-1], " 0999 ") {
		t.Fatalf("la más nueva: %.80q", out.Lines[len(out.Lines)-1])
	}

	// Con max_bytes 4096 y líneas de 16 KiB sale al menos la más nueva, cortada.
	h.f.AppendServiceLogs(appService, docker.LogLine{Time: at.Add(time.Hour), Stream: "stdout", TaskID: "t1aaaaaaaaaa", Text: strings.Repeat("z", 16<<10)})
	out = toolstest.OK[logsResult](s, "get_app_logs", args(map[string]any{"tail": 5, "max_bytes": 4096}))
	if out.Returned < 1 || !strings.Contains(out.Lines[out.Returned-1], "zzz") || !strings.HasSuffix(out.Lines[out.Returned-1], "…") {
		t.Fatalf("max_bytes 4096: %+v", out.Returned)
	}
}

// endlessLogs es el fake de Docker con el log de un servicio que no termina:
// manda una línea cada 5 ms y no mira el ctx; solo Close lo corta.
type endlessLogs struct{ *dockertest.Fake }

func (e endlessLogs) ServiceLogs(ctx context.Context, nameOrID string, o docker.LogOptions) (docker.LogStream, error) {
	if _, err := e.Fake.ServiceLogs(ctx, nameOrID, o); err != nil {
		return nil, err
	}
	return &endlessStream{closed: make(chan struct{})}, nil
}

type endlessStream struct {
	closed chan struct{}
	once   sync.Once
	n      int
}

func (s *endlessStream) Next() (docker.LogLine, error) {
	select {
	case <-time.After(5 * time.Millisecond):
		s.n++
		return docker.LogLine{Time: time.Now(), Stream: "stdout", TaskID: "t1aaaaaaaaaa", Text: fmt.Sprintf("sigue %d", s.n)}, nil
	case <-s.closed:
		return docker.LogLine{}, io.ErrClosedPipe
	}
}

func (s *endlessStream) Close() error { s.once.Do(func() { close(s.closed) }); return nil }

// Un log que no termina devuelve sus líneas con incomplete: true, y no
// internal, con los plazos inyectados: una lectura de 150 ms dentro de una
// llamada de 300 ms.
func TestMCPGetAppLogsIncomplete(t *testing.T) {
	h := newModuleTest(t, fakeEnv{})
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	m := New(Deps{DB: h.db, Cipher: c, Clock: h.clock, Audit: sqlite.NewAuditRecorder(h.db, h.clock, slog.New(slog.DiscardHandler)),
		Log: slog.New(slog.DiscardHandler), Docker: endlessLogs{h.f}, Env: fakeEnv{}, Ingress: fakeIngress{}})
	m.mcp.LogReadTimeout, m.mcp.LogCallTimeout = 150*time.Millisecond, 300*time.Millisecond
	k := toolstest.NewKit(t, m.RegisterRoutes)
	m.RegisterTools(k)
	h.seed(t)
	one := uint64(1)
	h.f.PutStackService(naming.Stack("auditfy", "staging", "api"), docker.Service{
		Name: appService, Labels: naming.Labels("auditfy", "staging", "api", 1), Replicas: &one,
	}, docker.Task{ID: "t1aaaaaaaaaa", Slot: 1, State: "running", DesiredState: "running"})
	start := time.Now()
	text, isErr := toolstest.Connect(t, k, tp).Call("get_app_logs", args(map[string]any{"tail": 1000}))
	out := toolstest.Decode[logsResult](t, text)
	if isErr || !out.Incomplete || out.Returned == 0 || time.Since(start) > 2*time.Second {
		t.Fatalf("isErr=%v, %v: %.300s", isErr, time.Since(start), text)
	}
}

// Con 8 streams SSE abiertos por HTTP con el mismo token, get_app_logs da
// rate_limited: el contador es el mismo.
func TestMCPGetAppLogsSharesStreamSlots(t *testing.T) {
	h := newMCPTest(t, fakeEnv{}, nil)
	h.seed(t)
	h.putAPIService()
	for range 8 {
		h.stream(t, logsPath("service=api&tail=0&follow=1"), bearer("tp"))
	}
	s := h.session(t)
	code, msg := toolstest.Fail(s, "get_app_logs", apiArgs)
	if code != "rate_limited" || !strings.HasPrefix(msg, "demasiados streams abiertos a la vez") {
		t.Fatalf("%s %q", code, msg)
	}
	// Otro token sí puede.
	other := toolstest.Connect(t, h.kit, entity.Principal{Token: &entity.TokenScope{ID: 99, Name: "otro", ProjectSlug: "auditfy"}})
	toolstest.OK[logsResult](other, "get_app_logs", apiArgs)
}
