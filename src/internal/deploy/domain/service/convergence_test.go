package service

import (
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/shared/domain/naming"
)

func u64(n uint64) *uint64 { return &n }

type evalCase struct {
	name     string
	expected []string
	services []SwarmService
	now      time.Time
	done     bool
	failed   string // el servicio (corto) que falla
	reason   []string
	progress map[string]string
}

func TestEvaluate(t *testing.T) {
	applied := t0                     // justo antes de docker stack deploy
	settleFrom := t0.Add(time.Second) // cuando el CLI volvió
	settle := 5 * time.Second
	before := t0.Add(-time.Hour) // un servicio que ya existía
	ours := t0.Add(time.Second)  // un UpdateStatus de este deploy
	skewed := t0.Add(-1500 * time.Millisecond)
	old := t0.Add(-10 * time.Minute) // un UpdateStatus de un deploy anterior
	upd := func(state string, at time.Time, msg string) *UpdateStatus {
		return &UpdateStatus{State: state, StartedAt: &at, Message: msg}
	}
	const api, web = "p_s_a_api", "p_s_a_web"

	cases := []evalCase{
		{name: "nuevo y todas corriendo: listo", expected: []string{api},
			services: []SwarmService{{ID: "s1", Name: api, Replicas: u64(2), RunningTasks: 2, DesiredTasks: 2, CreatedAt: ours}},
			now:      t0.Add(2 * time.Second), done: true,
			progress: map[string]string{"api": "2/2 réplicas corriendo"}},
		{name: "nuevo a medias: espera", expected: []string{api},
			services: []SwarmService{{ID: "s1", Name: api, Replicas: u64(2), RunningTasks: 1, DesiredTasks: 2, CreatedAt: ours}},
			now:      t0.Add(time.Minute),
			progress: map[string]string{"api": "1/2 réplicas corriendo"}},
		{name: "update de este deploy completed: listo", expected: []string{api},
			services: []SwarmService{{ID: "s1", Name: api, Replicas: u64(1), RunningTasks: 1, CreatedAt: before, Update: upd(UpdateCompleted, ours, "")}},
			now:      t0.Add(2 * time.Second), done: true},
		{name: "update de este deploy con el margen de 2 s: completed, listo", expected: []string{api},
			services: []SwarmService{{ID: "s1", Name: api, Replicas: u64(1), RunningTasks: 1, CreatedAt: before, Update: upd(UpdateCompleted, skewed, "")}},
			now:      t0.Add(2 * time.Second), done: true},
		{name: "updating: espera", expected: []string{api},
			services: []SwarmService{{ID: "s1", Name: api, Replicas: u64(1), RunningTasks: 1, CreatedAt: before, Update: upd(UpdateUpdating, ours, "")}},
			now:      t0.Add(time.Minute)},
		{name: "update viejo completed antes del margen: espera", expected: []string{api},
			services: []SwarmService{{ID: "s1", Name: api, Replicas: u64(1), RunningTasks: 1, CreatedAt: before, Update: upd(UpdateCompleted, old, "")}},
			now:      settleFrom.Add(settle - time.Millisecond)},
		{name: "update viejo completed después del margen, con las deseadas: listo", expected: []string{api},
			services: []SwarmService{{ID: "s1", Name: api, Replicas: u64(1), RunningTasks: 1, CreatedAt: before, Update: upd(UpdateCompleted, old, "")}},
			now:      settleFrom.Add(settle), done: true},
		{name: "update viejo en rollback: no es de este deploy, listo después del margen", expected: []string{api},
			services: []SwarmService{{ID: "s1", Name: api, Replicas: u64(1), RunningTasks: 1, CreatedAt: before, Update: upd(UpdateRollbackCompleted, old, "viejo")}},
			now:      settleFrom.Add(settle), done: true},
		{name: "después del margen sin las deseadas: espera", expected: []string{api},
			services: []SwarmService{{ID: "s1", Name: api, Replicas: u64(2), RunningTasks: 1, CreatedAt: before}},
			now:      settleFrom.Add(time.Minute),
			progress: map[string]string{"api": "1/2 réplicas corriendo"}},
		{name: "Update nil en uno que ya existía: antes del margen espera", expected: []string{api},
			services: []SwarmService{{ID: "s1", Name: api, Replicas: u64(1), RunningTasks: 1, CreatedAt: before}},
			now:      settleFrom.Add(time.Second)},
		{name: "Update nil en uno que ya existía: después del margen, listo", expected: []string{api},
			services: []SwarmService{{ID: "s1", Name: api, Replicas: u64(1), RunningTasks: 1, CreatedAt: before}},
			now:      settleFrom.Add(settle), done: true},
		{name: "replicas 0: listo", expected: []string{api},
			services: []SwarmService{{ID: "s1", Name: api, Replicas: u64(0), CreatedAt: ours}},
			now:      t0.Add(time.Second), done: true,
			progress: map[string]string{"api": "0/0 réplicas corriendo"}},
		{name: "global usa DesiredTasks: a medias espera", expected: []string{api},
			services: []SwarmService{{ID: "s1", Name: api, RunningTasks: 2, DesiredTasks: 3, CreatedAt: ours}},
			now:      t0.Add(time.Minute),
			progress: map[string]string{"api": "2/3 réplicas corriendo"}},
		{name: "global usa DesiredTasks: listo", expected: []string{api},
			services: []SwarmService{{ID: "s1", Name: api, RunningTasks: 3, DesiredTasks: 3, CreatedAt: ours}},
			now:      t0.Add(time.Second), done: true},
		{name: "un servicio que todavía no aparece: espera", expected: []string{api, web},
			services: []SwarmService{{ID: "s1", Name: api, Replicas: u64(1), RunningTasks: 1, CreatedAt: ours}},
			now:      t0.Add(time.Minute),
			progress: map[string]string{"api": "1/1 réplicas corriendo", "web": "todavía no está en Swarm"}},
		{name: "los servicios de otra app no cuentan", expected: []string{api},
			services: []SwarmService{
				{ID: "x", Name: "p_s_otra_api", Replicas: u64(1), CreatedAt: ours},
				{ID: "s1", Name: api, Replicas: u64(1), RunningTasks: 1, CreatedAt: ours},
			},
			now: t0.Add(time.Second), done: true},
	}
	for _, state := range []string{UpdateRollbackStarted, UpdateRollbackCompleted, UpdateRollbackPaused, UpdatePaused} {
		cases = append(cases, evalCase{name: state + ": falla con el Message", expected: []string{api},
			services: []SwarmService{{ID: "s1", Name: api, Replicas: u64(1), RunningTasks: 1, CreatedAt: before, Update: upd(state, ours, "update paused due to failure of task abc")}},
			now:      t0.Add(time.Minute), failed: "api", reason: []string{state, "update paused due to failure of task abc"}})
	}
	cases = append(cases, evalCase{name: "dos servicios, uno falla: el primero por nombre", expected: []string{web, api},
		services: []SwarmService{
			{ID: "s2", Name: web, Replicas: u64(1), CreatedAt: before, Update: upd(UpdateRollbackCompleted, ours, "web falló")},
			{ID: "s1", Name: api, Replicas: u64(1), CreatedAt: before, Update: upd(UpdatePaused, ours, "api falló")},
		},
		now: t0.Add(time.Minute), failed: "api", reason: []string{"api falló"}})

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := Evaluate(c.expected, c.services, applied, settleFrom, c.now, settle)
			if v.Done != c.done {
				t.Errorf("Done = %v (%+v)", v.Done, v)
			}
			if (c.failed != "") != v.Failed || v.Service != c.failed {
				t.Errorf("Failed = %v, Service = %q (%+v)", v.Failed, v.Service, v)
			}
			if v.Failed && v.Done {
				t.Error("Done y Failed a la vez")
			}
			for _, r := range c.reason {
				if !strings.Contains(v.Reason, r) {
					t.Errorf("Reason %q no contiene %q", v.Reason, r)
				}
			}
			if c.failed != "" {
				for _, s := range c.services {
					if s.Name == naming.Service("p", "s", "a", c.failed) && v.ServiceID != s.ID {
						t.Errorf("ServiceID = %q", v.ServiceID)
					}
				}
			}
			for k, want := range c.progress {
				if v.Progress[k] != want {
					t.Errorf("Progress[%s] = %q, esperaba %q (%v)", k, v.Progress[k], want, v.Progress)
				}
			}
			if len(v.Progress) != len(c.expected) {
				t.Errorf("Progress = %v", v.Progress)
			}
		})
	}
}

// Decisión 6 del plan: el margen se cuenta desde que docker stack deploy
// volvió, no desde antes de llamarlo. Con un CLI que tardó más que el margen,
// contar desde appliedAt daría por convergido un servicio cuyo rollout Swarm
// todavía no empezó.
func TestEvaluateSlowCLI(t *testing.T) {
	applied, settleFrom := t0, t0.Add(8*time.Second)
	svc := []SwarmService{{ID: "s1", Name: "p_s_a_api", Replicas: u64(1), RunningTasks: 1, CreatedAt: t0.Add(-time.Hour)}}
	if v := Evaluate([]string{"p_s_a_api"}, svc, applied, settleFrom, t0.Add(9*time.Second), 5*time.Second); v.Done || v.Failed {
		t.Fatalf("a t0+9s debe esperar: %+v", v)
	}
	if v := Evaluate([]string{"p_s_a_api"}, svc, applied, settleFrom, t0.Add(13*time.Second), 5*time.Second); !v.Done {
		t.Fatalf("a t0+13s converge: %+v", v)
	}
}

func TestEvaluateNoServices(t *testing.T) {
	v := Evaluate(nil, nil, t0, t0, t0, 5*time.Second)
	if !v.Done || v.Failed || len(v.Progress) != 0 {
		t.Fatalf("%+v", v)
	}
	// Progress es una copia nueva en cada llamada.
	v.Progress["x"] = "y"
	if w := Evaluate(nil, nil, t0, t0, t0, 5*time.Second); len(w.Progress) != 0 {
		t.Fatalf("%+v", w)
	}
}

func TestAppliedState(t *testing.T) {
	expected := []string{"p_s_a_api", "p_s_a_web"}
	labels := func(id int64) map[string]string { return naming.Labels("p", "s", "a", id) }
	cases := []struct {
		name     string
		services []SwarmService
		want     Applied
	}{
		{"ninguno: no existen", nil, AppliedNone},
		{"ninguno: llevan el deploy anterior", []SwarmService{
			{Name: "p_s_a_api", Labels: labels(144)}, {Name: "p_s_a_web", Labels: labels(144)},
		}, AppliedNone},
		{"algunos", []SwarmService{
			{Name: "p_s_a_api", Labels: labels(145)}, {Name: "p_s_a_web", Labels: labels(144)},
		}, AppliedPartial},
		{"algunos: uno no existe", []SwarmService{{Name: "p_s_a_api", Labels: labels(145)}}, AppliedPartial},
		{"todos", []SwarmService{
			{Name: "p_s_a_web", Labels: labels(145)}, {Name: "p_s_a_api", Labels: labels(145)},
			{Name: "p_s_a_viejo", Labels: labels(140)},
		}, AppliedAll},
		{"uno de otra app con la misma etiqueta no cuenta", []SwarmService{
			{Name: "p_s_a_api", Labels: labels(145)}, {Name: "p_s_otra_web", Labels: labels(145)},
		}, AppliedPartial},
	}
	for _, c := range cases {
		if got := AppliedState(expected, c.services, 145); got != c.want {
			t.Errorf("%s: %v, esperaba %v", c.name, got, c.want)
		}
	}
}
