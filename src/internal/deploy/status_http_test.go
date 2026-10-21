package deploy

import (
	"net/http"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

// GET …/status (contrato §2.8): todos los campos con su tipo, null cuando
// corresponde; las horas truncadas al milisegundo.
func TestStatusShape(t *testing.T) {
	h := newHTTPTest(t)
	h.seed(t)
	stack := naming.Stack("auditfy", "staging", "api")
	one := uint64(1)
	h.f.PutStackService(stack, docker.Service{
		Name: naming.Service("auditfy", "staging", "api", "api"), Labels: naming.Labels("auditfy", "staging", "api", 1),
		Image: "ghcr.io/x/api:1@sha256:9b2d", Replicas: &one, RunningTasks: 1, DesiredTasks: 1, MemoryLimitBytes: 536870912,
		Update: &docker.UpdateStatus{State: docker.UpdateCompleted},
	}, docker.Task{ID: "k3x9", Slot: 1, State: "running", DesiredState: "running", NodeID: "node-1", ContainerID: "c1",
		UpdatedAt: time.Date(2026, 10, 10, 14, 20, 5, 987654321, time.UTC)})
	h.f.PutContainer(docker.Container{ID: "c1", Running: true, Health: "healthy"})
	h.f.SetStats("c1", docker.Stats{CPUPercent: 1.7, MemoryBytes: 311427072})
	h.f.PutStackService(stack, docker.Service{
		Name: naming.Service("auditfy", "staging", "api", "cron"), Labels: naming.AppLabels("auditfy", "staging", "api"),
		Image: "ghcr.io/x/cron:1",
	}, docker.Task{ID: "g1", State: "pending", DesiredState: "running", Err: "no suitable node"})

	w := h.c.Do("GET", apiPath+"/status", nil, httptestutil.WithBearer("tp"))
	httptestutil.Expect(t, w, http.StatusOK)
	got := httptestutil.Decode[map[string][]map[string]any](t, w)
	svcs := got["services"]
	if len(svcs) != 2 {
		t.Fatalf("services: %s", w.Body)
	}
	api, cron := svcs[0], svcs[1]
	want := map[string]any{
		"name": "api", "image": "ghcr.io/x/api:1@sha256:9b2d", "replicas_desired": 1.0, "replicas_running": 1.0,
		"cpu_percent": 1.7, "memory_bytes": 311427072.0, "memory_limit_bytes": 536870912.0, "update_state": "completed",
	}
	for k, v := range want {
		if api[k] != v {
			t.Errorf("api.%s = %v, esperaba %v", k, api[k], v)
		}
	}
	if len(api) != len(want)+1 {
		t.Errorf("api tiene otros campos: %v", api)
	}
	tasks, _ := api["tasks"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("tasks: %v", api["tasks"])
	}
	task, _ := tasks[0].(map[string]any)
	wantTask := map[string]any{
		"id": "k3x9", "slot": 1.0, "state": "running", "desired_state": "running", "health": "healthy",
		"node": "manager-1", "since": "2026-10-10T14:20:05.987Z", "error": nil, "cpu_percent": 1.7, "memory_bytes": 311427072.0,
	}
	if len(task) != len(wantTask) {
		t.Errorf("la tarea tiene otros campos: %v", task)
	}
	for k, v := range wantTask {
		if got, ok := task[k]; !ok || got != v {
			t.Errorf("task.%s = %v, esperaba %v", k, got, v)
		}
	}
	// Un servicio global, sin medir, sin límite ni actualización.
	for _, k := range []string{"cpu_percent", "memory_bytes", "memory_limit_bytes", "update_state"} {
		if v, ok := cron[k]; !ok || v != nil {
			t.Errorf("cron.%s = %v, esperaba null", k, v)
		}
	}
	ctask := cron["tasks"].([]any)[0].(map[string]any)
	for _, k := range []string{"slot", "health", "node", "since", "cpu_percent", "memory_bytes"} {
		if v, ok := ctask[k]; !ok || v != nil {
			t.Errorf("cron.tasks[0].%s = %v, esperaba null", k, v)
		}
	}
	if ctask["error"] != "no suitable node" {
		t.Errorf("cron.tasks[0].error = %v", ctask["error"])
	}
}

// Sin servicios en Swarm, services: []; la app que no existe es 404, Docker
// caído es 503, y un token de otro proyecto no llega.
func TestStatusHTTPErrors(t *testing.T) {
	h := newHTTPTest(t)
	h.seed(t)
	w := h.c.Do("GET", apiPath+"/status", nil, session())
	httptestutil.Expect(t, w, http.StatusOK)
	if w.Body.String() != "{\"services\":[]}\n" {
		t.Fatalf("sin servicios: %q", w.Body)
	}
	expectError(t, h.c.Do("GET", nadaPath+"/status", nil, session()), 404, "not_found", "")
	expectError(t, h.c.Do("GET", apiPath+"/status", nil, httptestutil.WithBearer("tx")), 403, "out_of_scope", "")
	h.clock.Advance(time.Minute) // vence la caché de lo estructural
	h.f.SetUnavailable(true)
	expectError(t, h.c.Do("GET", apiPath+"/status", nil, session()), 503, "docker_unavailable", "")
}
