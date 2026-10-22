package deploy

import (
	"context"
	"encoding/json/jsontext"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/mcp/tools"
	"github.com/CPU-commits/muelle/src/cmd/mcp/tools/toolstest"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	depsqlite "github.com/CPU-commits/muelle/src/internal/deploy/infra/out/repository/sqlite"
)

// enqueue encola un deploy de app con el token tp por MCP y devuelve su id.
func (h *mcpTest) enqueue(t *testing.T, s *toolstest.Session, app string) int64 {
	t.Helper()
	out := toolstest.OK[struct {
		ID int64 `json:"id"`
	}](s, "deploy", args(map[string]any{"app": app}))
	return out.ID
}

// setLog deja el log del deploy tal cual (sin pasar por el LogWriter, que
// limpia los controles): así se prueba lo peor que puede escapar.
func (h *mcpTest) setLog(t *testing.T, id int64, log string) {
	t.Helper()
	h.exec(t, `UPDATE deploys SET log = ? WHERE id = ?`, log, id)
}

type deployResult struct {
	ID              int64             `json:"id"`
	Status          string            `json:"status"`
	Trigger         string            `json:"trigger"`
	ActorName       string            `json:"actor_name"`
	Error           *string           `json:"error"`
	RequestedImages map[string]string `json:"requested_images"`
	Log             *struct {
		Lines      []string `json:"lines"`
		FromOffset int      `json:"from_offset"`
		NextOffset int      `json:"next_offset"`
		Length     int      `json:"length"`
		Earlier    bool     `json:"earlier"`
	} `json:"log"`
	RequestedImagesOmitted bool  `json:"requested_images_omitted"`
	WaitedS                *int  `json:"waited_s"`
	TimedOut               *bool `json:"timed_out"`
}

// deploy y rollback por MCP: el deploy queda encolado con trigger api y el
// token como actor, y deploy.create lleva via mcp; las reglas de imagen son
// las de REST.
func TestMCPDeployAndRollback(t *testing.T) {
	h := newMCPTest(t, fakeEnv{}, nil)
	h.seed(t)
	s := h.session(t)

	text, isErr := s.Call("deploy", apiArgs)
	if isErr || !strings.HasSuffix(text, `,"status":"queued","status_url":"/api/v1/projects/auditfy/stages/staging/apps/api/deploys/1","next":"get_deploy con wait_s para seguirlo"}`) {
		t.Fatalf("deploy: %s", text)
	}
	d, err := depsqlite.NewDeployRepository(h.db).GetByID(context.Background(), 1)
	if err != nil || d.Trigger != entity.TriggerAPI || d.ActorName != "github-actions" || d.ActorType != entity.ActorToken {
		t.Fatalf("deploy=%+v err=%v", d, err)
	}
	var details string
	if err := h.db.Conn(context.Background()).QueryRowContext(context.Background(),
		`SELECT details FROM audit_events WHERE action = 'deploy.create' ORDER BY id DESC LIMIT 1`).Scan(&details); err != nil ||
		details != `{"deploy":1,"images":{},"trigger":"api","via":"mcp"}` {
		t.Fatalf("auditoría: %s %v", details, err)
	}
	ref := "ghcr.io/x/api:1@sha256:" + strings.Repeat("a", 64)
	h.finish(t, 1, entity.StatusSucceeded, map[string]string{"api": ref})

	for _, c := range []struct {
		args map[string]any
		msg  string
	}{
		{args(map[string]any{"app": "infra", "image": "ghcr.io/x/web:2"}), "image: una app compose recibe images, por servicio"},
		{args(map[string]any{"image": "ghcr.io/x/api:2", "images": map[string]string{"api": "x"}}), "image: manda image o images, no los dos"},
		{args(map[string]any{"image": ""}), "image: "},
		{args(map[string]any{"images": map[string]string{"nope": "ghcr.io/x/a:1"}}), "images.nope: el servicio no existe en la app"},
		{args(map[string]any{"images": manyImages(21)}), "images: máximo 20 servicios"},
	} {
		code, msg := toolstest.Fail(s, "deploy", c.args)
		if code != "invalid" || !strings.HasPrefix(msg, c.msg) {
			t.Errorf("%v: %s %q", c.args, code, msg)
		}
	}
	if code, _ := toolstest.Fail(s, "deploy", args(map[string]any{"app": "nada"})); code != "not_found" {
		t.Errorf("app que no existe: %s", code)
	}
	// Rollback: sin anterior, no_previous_deploy; con dos succeeded, encola
	// el rollback con su nota.
	if code, msg := toolstest.Fail(s, "rollback", apiArgs); code != "no_previous_deploy" || msg != "no hay un deploy exitoso anterior al actual" {
		t.Fatalf("rollback sin anterior: %s %q", code, msg)
	}
	toolstest.OK[map[string]any](s, "deploy", args(map[string]any{"image": "ghcr.io/x/api:2"}))
	h.finish(t, 2, entity.StatusSucceeded, map[string]string{"api": "ghcr.io/x/api:2@sha256:" + strings.Repeat("b", 64)})
	text, isErr = s.Call("rollback", apiArgs)
	if isErr || !strings.HasSuffix(text, `"next":"get_deploy con wait_s para seguirlo","note":"El próximo deploy sin image repite estas imágenes con su digest (contrato §2.2)."}`) {
		t.Fatalf("rollback: %s", text)
	}
	rb := toolstest.Decode[struct {
		ID int64 `json:"id"`
	}](t, text)
	d, _ = depsqlite.NewDeployRepository(h.db).GetByID(context.Background(), rb.ID)
	if d.Trigger != entity.TriggerRollback || d.Images["api"] != ref {
		t.Fatalf("rollback: %+v", d)
	}
}

func manyImages(n int) map[string]string {
	out := map[string]string{}
	for i := range n {
		out[fmt.Sprintf("s%02d", i)] = "ghcr.io/x/a:1"
	}
	return out
}

// list_deploys: del más nuevo al más viejo, con limit, sin
// requested_images y con el error cortado.
func TestMCPListDeploys(t *testing.T) {
	h := newMCPTest(t, fakeEnv{}, nil)
	h.seed(t)
	s := h.session(t)
	for range 3 {
		h.enqueue(t, s, "api")
		h.finish(t, h.lastDeploy(t), entity.StatusFailed, map[string]string{"api": "ghcr.io/x/api:1"})
	}
	h.exec(t, `UPDATE deploys SET error = ? WHERE id = 3`, strings.Repeat("é", 400))
	text, isErr := s.Call("list_deploys", args(map[string]any{"limit": 2}))
	out := toolstest.Decode[struct {
		Deploys []map[string]any `json:"deploys"`
		Omitted int              `json:"omitted"`
		Warning string           `json:"warning"`
	}](t, text)
	if isErr || len(out.Deploys) != 2 || out.Omitted != 0 || out.Deploys[0]["id"] != float64(3) || out.Warning == "" {
		t.Fatalf("list_deploys: %s", text)
	}
	if _, ok := out.Deploys[0]["requested_images"]; ok {
		t.Fatalf("list_deploys no trae requested_images: %v", out.Deploys[0])
	}
	if e := out.Deploys[0]["error"].(string); len([]rune(e)) != 300 || !strings.HasSuffix(e, "…") {
		t.Fatalf("error de %d caracteres", len([]rune(e)))
	}
	if n := len(toolstest.OK[struct {
		Deploys []any `json:"deploys"`
	}](s, "list_deploys", apiArgs).Deploys); n != 3 {
		t.Fatalf("con el limit por defecto: %d", n)
	}
}

func (h *mcpTest) lastDeploy(t *testing.T) int64 {
	t.Helper()
	var id int64
	if err := h.db.Conn(context.Background()).QueryRowContext(context.Background(), `SELECT MAX(id) FROM deploys`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// Peor caso de list_deploys: una app compose de 20 servicios con refs con
// digest, 22 deploys guardados, errores de 300 caracteres y limit 22: entra
// en el tope, del más nuevo al más viejo, con omitted > 0.
func TestMCPListDeploysWorstCase(t *testing.T) {
	h := newMCPTest(t, fakeEnv{}, nil)
	_, infra, _ := h.seed(t)
	images := map[string]string{}
	for i := range 20 {
		images[fmt.Sprintf("s%02d", i)] = "ghcr.io/" + strings.Repeat("o", 300) + fmt.Sprintf("/svc%02d:sha-1@sha256:%s", i, strings.Repeat("c", 64))
	}
	for i := range 22 {
		h.exec(t, `INSERT INTO deploys (app_id, status, images, trigger, actor_type, actor_id, actor_name, created_at, error)
			VALUES (?, 'failed', ?, 'api', 'token', 4, 'ci', ?, ?)`, infra, toJSON(t, images), int64(i), strings.Repeat("\x1b[31m\"", 100))
	}
	text, isErr := h.session(t).Call("list_deploys", args(map[string]any{"app": "infra", "limit": 22}))
	out := toolstest.Decode[struct {
		Deploys []map[string]any `json:"deploys"`
		Omitted int              `json:"omitted"`
	}](t, text)
	if isErr || len(text) > tools.MaxResultBytes || out.Omitted == 0 || len(out.Deploys)+out.Omitted != 22 || out.Deploys[0]["id"] != float64(22) {
		t.Fatalf("isErr=%v, %d bytes, %d deploys, omitted %d", isErr, len(text), len(out.Deploys), out.Omitted)
	}
}

func toJSON(t *testing.T, v map[string]string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("{")
	first := true
	for k, val := range v {
		if !first {
			b.WriteString(",")
		}
		first = false
		fmt.Fprintf(&b, "%q:%q", k, val)
	}
	b.WriteString("}")
	return b.String()
}

// get_deploy: el deploy y el final de su log en líneas completas, con
// earlier; sin log con log_tail_bytes 0; not_found de otra app.
func TestMCPGetDeploy(t *testing.T) {
	h := newMCPTest(t, fakeEnv{}, nil)
	h.seed(t)
	s := h.session(t)
	id := h.enqueue(t, s, "api")
	log := h.logWriter(id)
	for i := range 200 {
		if err := log.Line("paso %03d %s", i, strings.Repeat("x", 100)); err != nil {
			t.Fatal(err)
		}
	}
	out := toolstest.OK[deployResult](s, "get_deploy", args(map[string]any{"id": id}))
	l := out.Log
	if out.ID != id || out.Status != "queued" || out.RequestedImages == nil || out.RequestedImagesOmitted || out.WaitedS != nil || out.TimedOut != nil ||
		l == nil || !l.Earlier || l.NextOffset != l.Length || !strings.HasSuffix(l.Lines[len(l.Lines)-1], "paso 199 "+strings.Repeat("x", 100)) {
		t.Fatalf("get_deploy: %+v", out)
	}
	lines := toolstest.Decode[struct {
		Log struct {
			Lines jsontext.Value `json:"lines"`
		} `json:"log"`
	}](t, mustCall(t, s, "get_deploy", args(map[string]any{"id": id})))
	if len(lines.Log.Lines) > 12288 || len(lines.Log.Lines) < 12000 {
		t.Fatalf("log.lines mide %d", len(lines.Log.Lines))
	}
	// from_offset es un comienzo de línea: leer desde ahí da la misma primera.
	page := toolstest.OK[struct {
		Lines []string `json:"lines"`
	}](s, "read_deploy_log", args(map[string]any{"id": id, "offset": l.FromOffset}))
	if page.Lines[0] != l.Lines[0] {
		t.Fatalf("from_offset %d: %q vs %q", l.FromOffset, page.Lines[0], l.Lines[0])
	}
	if out := toolstest.OK[deployResult](s, "get_deploy", args(map[string]any{"id": id, "log_tail_bytes": 0})); out.Log != nil {
		t.Fatalf("log_tail_bytes 0: %+v", out.Log)
	}
	other := h.enqueue(t, s, "infra")
	if code, _ := toolstest.Fail(s, "get_deploy", args(map[string]any{"id": other})); code != "not_found" {
		t.Fatalf("deploy de otra app: %s", code)
	}
}

func mustCall(t *testing.T, s *toolstest.Session, name string, a map[string]any) string {
	t.Helper()
	text, isErr := s.Call(name, a)
	if isErr {
		t.Fatalf("%s: %s", name, text)
	}
	return text
}

// wait_s vuelve apenas el deploy termina, y da timed_out si no.
func TestMCPGetDeployWait(t *testing.T) {
	h := newMCPTest(t, fakeEnv{}, nil)
	h.seed(t)
	s := h.session(t)
	id := h.enqueue(t, s, "api")

	start := time.Now()
	out := toolstest.OK[deployResult](s, "get_deploy", args(map[string]any{"id": id, "wait_s": 1}))
	if out.TimedOut == nil || !*out.TimedOut || out.WaitedS == nil || *out.WaitedS != 1 || out.Status != "queued" || time.Since(start) < time.Second {
		t.Fatalf("sin terminar: %+v (%v)", out, time.Since(start))
	}

	done := make(chan deployResult)
	go func() {
		done <- toolstest.OK[deployResult](s, "get_deploy", args(map[string]any{"id": id, "wait_s": 30}))
	}()
	time.Sleep(200 * time.Millisecond)
	start = time.Now()
	h.finish(t, id, entity.StatusSucceeded, map[string]string{"api": "ghcr.io/x/api:1@sha256:" + strings.Repeat("a", 64)})
	h.m.hub.Notify(id)
	select {
	case out = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("wait_s no volvió al terminar el deploy")
	}
	if out.Status != "succeeded" || out.TimedOut == nil || *out.TimedOut || time.Since(start) > 3*time.Second {
		t.Fatalf("al terminar: %+v", out)
	}
	// Un deploy que ya terminó no espera.
	out = toolstest.OK[deployResult](s, "get_deploy", args(map[string]any{"id": id, "wait_s": 45}))
	if *out.WaitedS != 0 || *out.TimedOut {
		t.Fatalf("ya terminado: %+v", out)
	}
}

// Peor caso de get_deploy: 32 servicios con refs de 512 caracteres y
// log_tail_bytes 16384 sobre un log de líneas llenas de ESC: entra, sin
// requested_images, y log.lines mide a lo más log_tail_bytes de JSON.
func TestMCPGetDeployWorstCase(t *testing.T) {
	h := newMCPTest(t, fakeEnv{}, nil)
	_, infra, _ := h.seed(t)
	images := map[string]string{}
	for i := range 32 {
		images[fmt.Sprintf("s%02d", i)] = "ghcr.io/" + strings.Repeat("r", 430) + fmt.Sprintf("/s%02d@sha256:%s", i, strings.Repeat("d", 64))
	}
	id := h.exec(t, `INSERT INTO deploys (app_id, status, images, requested_images, trigger, actor_type, actor_id, actor_name, created_at)
		VALUES (?, 'running', ?, ?, 'api', 'token', 4, 'ci', 0)`, infra, toJSON(t, images), toJSON(t, images))
	var log strings.Builder
	for i := range 40 {
		fmt.Fprintf(&log, "2026-10-10T14:00:00.000Z %03d %s\n", i, strings.Repeat("\x1b", 1500))
	}
	h.setLog(t, id, log.String())
	text, isErr := h.session(t).Call("get_deploy", args(map[string]any{"app": "infra", "id": id, "log_tail_bytes": 16384}))
	out := toolstest.Decode[deployResult](t, text)
	raw := toolstest.Decode[struct {
		Log struct {
			Lines jsontext.Value `json:"lines"`
		} `json:"log"`
	}](t, text)
	if isErr || len(text) > tools.MaxResultBytes || !out.RequestedImagesOmitted || out.RequestedImages != nil ||
		len(raw.Log.Lines) > 16384 || len(out.Log.Lines) == 0 || !out.Log.Earlier {
		t.Fatalf("isErr=%v, %d bytes, omitted %v, lines %d bytes (%d)", isErr, len(text), out.RequestedImagesOmitted, len(raw.Log.Lines), len(out.Log.Lines))
	}
	// Una última línea que no entra (ni sola) sale cortada.
	h.setLog(t, id, "2026-10-10T14:00:00.000Z "+strings.Repeat("\x1b", 8000)+"\n")
	out = toolstest.OK[deployResult](h.session(t), "get_deploy", args(map[string]any{"app": "infra", "id": id, "log_tail_bytes": 1000}))
	if len(out.Log.Lines) != 1 || tools.QuoteSize(out.Log.Lines[0])+2 > 1000 || !strings.HasSuffix(out.Log.Lines[0], "…") || out.Log.FromOffset != 0 {
		t.Fatalf("una línea que no entra: %+v", out.Log)
	}
}

// Con un log_tail_bytes menor que ["…"] (7 bytes), ni la última línea
// cortada entra: log.lines sale vacía ([], que mide 2), con from_offset en
// next_offset (como cuando se va la última por el tope) y earlier. Desde 7,
// sale "…".
func TestMCPGetDeployTinyTail(t *testing.T) {
	h := newMCPTest(t, fakeEnv{}, nil)
	h.seed(t)
	s := h.session(t)
	id := h.enqueue(t, s, "api")
	log := h.logWriter(id)
	for i := range 3 {
		if err := log.Line("paso %d", i); err != nil {
			t.Fatal(err)
		}
	}
	for n := 1; n <= 7; n++ {
		text := mustCall(t, s, "get_deploy", args(map[string]any{"id": id, "log_tail_bytes": n}))
		out := toolstest.Decode[deployResult](t, text)
		raw := toolstest.Decode[struct {
			Log struct {
				Lines jsontext.Value `json:"lines"`
			} `json:"log"`
		}](t, text)
		l := out.Log
		if l == nil || len(raw.Log.Lines) > max(n, 2) || l.NextOffset != l.Length || !l.Earlier {
			t.Fatalf("log_tail_bytes %d: %s", n, text)
		}
		switch {
		case n < 7 && (len(l.Lines) != 0 || l.FromOffset != l.NextOffset):
			t.Fatalf("log_tail_bytes %d: %+v", n, l)
		case n == 7 && (len(l.Lines) != 1 || l.Lines[0] != "…" || l.FromOffset >= l.NextOffset):
			t.Fatalf("log_tail_bytes 7: %+v", l)
		}
	}
}

type readLogResult struct {
	Lines      []string `json:"lines"`
	Offset     int      `json:"offset"`
	NextOffset int      `json:"next_offset"`
	Length     int      `json:"length"`
	Status     string   `json:"status"`
}

// read_deploy_log: líneas completas desde offset, next_offset en bytes del
// log, y el 422 de un offset que pasa el largo.
func TestMCPReadDeployLog(t *testing.T) {
	h := newMCPTest(t, fakeEnv{}, nil)
	h.seed(t)
	s := h.session(t)
	id := h.enqueue(t, s, "api")
	log := h.logWriter(id)
	for i := range 3 {
		if err := log.Line("línea %d", i); err != nil {
			t.Fatal(err)
		}
	}
	out := toolstest.OK[readLogResult](s, "read_deploy_log", args(map[string]any{"id": id}))
	if len(out.Lines) != 3 || out.Offset != 0 || out.NextOffset != out.Length || out.Status != "queued" {
		t.Fatalf("desde 0: %+v", out)
	}
	first := len(out.Lines[0]) + 1
	out = toolstest.OK[readLogResult](s, "read_deploy_log", args(map[string]any{"id": id, "offset": first}))
	if len(out.Lines) != 2 || out.Offset != first || out.NextOffset != out.Length {
		t.Fatalf("desde la segunda: %+v", out)
	}
	if code, msg := toolstest.Fail(s, "read_deploy_log", args(map[string]any{"id": id, "offset": out.Length + 1})); code != "invalid" || msg != "offset: pasa el largo del log" {
		t.Fatalf("offset más allá: %s %q", code, msg)
	}
	if code, _ := toolstest.Fail(s, "read_deploy_log", args(map[string]any{"id": id + 100})); code != "not_found" {
		t.Fatalf("deploy que no existe: %s", code)
	}
}

// Peor caso de read_deploy_log: líneas de 8 KiB llenas de ESC con max_bytes
// 9216: cada llamada avanza (la primera sale cortada y next_offset pasa su
// final), y lines mide a lo más max_bytes de JSON. Con 32768 y líneas
// normales, el resultado entra en el tope.
func TestMCPReadDeployLogWorstCase(t *testing.T) {
	h := newMCPTest(t, fakeEnv{}, nil)
	h.seed(t)
	s := h.session(t)
	id := h.enqueue(t, s, "api")
	var log strings.Builder
	for range 5 {
		log.WriteString("2026-10-10T14:00:00.000Z " + strings.Repeat("\x1b", 8192-25) + "\n")
	}
	h.setLog(t, id, log.String())
	offset, calls := 0, 0
	for offset < log.Len() {
		text, isErr := s.Call("read_deploy_log", args(map[string]any{"id": id, "offset": offset, "max_bytes": 9216}))
		out := toolstest.Decode[readLogResult](t, text)
		raw := toolstest.Decode[struct {
			Lines jsontext.Value `json:"lines"`
		}](t, text)
		if isErr || len(out.Lines) != 1 || len(raw.Lines) > 9216 || out.NextOffset != offset+8193 || !strings.HasSuffix(out.Lines[0], "…") {
			t.Fatalf("desde %d: isErr=%v, %d líneas, %d bytes, next %d", offset, isErr, len(out.Lines), len(raw.Lines), out.NextOffset)
		}
		offset = out.NextOffset
		calls++
	}
	if calls != 5 {
		t.Fatalf("%d llamadas", calls)
	}

	log.Reset()
	for i := range 2000 {
		fmt.Fprintf(&log, "2026-10-10T14:00:00.000Z línea normal %04d \"con\" comillas\n", i)
	}
	h.setLog(t, id, log.String())
	text, isErr := s.Call("read_deploy_log", args(map[string]any{"id": id}))
	raw := toolstest.Decode[struct {
		Lines jsontext.Value `json:"lines"`
	}](t, text)
	if isErr || len(text) > tools.MaxResultBytes || len(raw.Lines) > 32768 || len(raw.Lines) < 30000 {
		t.Fatalf("isErr=%v, %d bytes, lines %d", isErr, len(text), len(raw.Lines))
	}
}
