package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
)

// simOpts es cómo responde el Swarm simulado a los próximos docker stack
// deploy (swarmSim.set los cambia entre un deploy y otro).
type simOpts struct {
	// State es el UpdateStatus de un servicio que ya existía y cuya plantilla
	// cambió; completed por defecto. Message va con él.
	State, Message string
	// Running son las tareas que corren en un servicio nuevo o cambiado; nil
	// es todas las deseadas.
	Running *uint64
	// FailedErr, si no es "", agrega una tarea fallida (la más nueva) con ese
	// Err y FailedLogs líneas de log ("línea 0", "línea 1"…).
	FailedErr  string
	FailedLogs int
	// Block retiene el deploy dentro de OnDeployStack hasta que se cierra (no
	// ve el ctx: solo para los tests que no cancelan).
	Block chan struct{}
	// Delay es lo que tarda el CLI simulado.
	Delay time.Duration
	// NoUpdate deja un servicio cambiado sin UpdateStatus nuevo y con sus
	// tareas, como Swarm justo antes de empezar el rollout.
	NoUpdate bool
	// Fail hace fallar el deploy con ese error (la salida del CLI lo trae).
	Fail error
}

// simService es lo que el simulador sabe de un servicio: su plantilla (el
// servicio del compose sin deploy.labels), el servicio y sus tareas.
type simService struct {
	template string
	svc      docker.Service
	tasks    []docker.Task
}

// swarmSim es un Swarm simulado sobre dockertest: por cada docker stack
// deploy crea o actualiza los servicios del compose como lo haría Swarm
// (§7 paso 6) y borra los que ya no están (--prune).
type swarmSim struct {
	f *dockertest.Fake

	mu       sync.Mutex
	opts     simOpts
	services map[string]*simService // por nombre de Swarm
	returned []time.Time            // cuándo volvió cada OnDeployStack que aplicó
}

func simulate(f *dockertest.Fake, o simOpts) *swarmSim {
	s := &swarmSim{f: f, opts: o, services: map[string]*simService{}}
	f.OnDeployStack = s.deploy
	return s
}

func (s *swarmSim) set(o simOpts) { s.mu.Lock(); s.opts = o; s.mu.Unlock() }

// returns son las horas en que volvió cada OnDeployStack que aplicó un compose.
func (s *swarmSim) returns() []time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Time(nil), s.returned...)
}

// age atrasa d la creación de todos los servicios, el inicio de su última
// actualización y la creación de sus tareas: así un deploy que sigue no los
// toma por suyos (Evaluate mira 2 s hacia atrás).
func (s *swarmSim) age(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ss := range s.services {
		ss.svc.CreatedAt = ss.svc.CreatedAt.Add(-d)
		if u := ss.svc.Update; u != nil && u.StartedAt != nil {
			at := u.StartedAt.Add(-d)
			ss.svc.Update = &docker.UpdateStatus{State: u.State, Message: u.Message, StartedAt: &at}
		}
		for i := range ss.tasks {
			ss.tasks[i].CreatedAt = ss.tasks[i].CreatedAt.Add(-d)
		}
		s.put(ss)
	}
}

func (s *swarmSim) put(ss *simService) {
	stack := ss.svc.Labels[naming.LabelStackNamespace]
	s.f.PutStackService(stack, ss.svc, append([]docker.Task(nil), ss.tasks...)...)
	// PutStackService asigna los ids que faltan: se leen de vuelta.
	got, err := s.f.InspectService(context.Background(), ss.svc.Name)
	if err == nil {
		ss.svc.ID = got.ID
	}
	tasks, err := s.f.ListTasks(context.Background(), docker.TaskFilter{Service: ss.svc.Name})
	if err == nil && len(tasks) == len(ss.tasks) {
		ss.tasks = tasks
	}
}

type simComposeService struct {
	Image  string `json:"image"`
	Deploy *struct {
		Mode     string            `json:"mode"`
		Replicas *uint64           `json:"replicas"`
		Labels   map[string]string `json:"labels"`
	} `json:"deploy"`
}

func (s *swarmSim) deploy(d docker.StackDeploy) error {
	s.mu.Lock()
	o := s.opts
	s.mu.Unlock()
	if o.Block != nil {
		<-o.Block
	}
	if o.Delay > 0 {
		time.Sleep(o.Delay)
	}
	if o.Fail != nil {
		return o.Fail
	}
	var raw struct {
		Services map[string]json.RawMessage `json:"services"`
	}
	if err := json.Unmarshal(d.Compose, &raw); err != nil {
		return fmt.Errorf("simulador: el compose no es JSON: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	keep := map[string]bool{}
	for name, msg := range raw.Services {
		var cs simComposeService
		if err := json.Unmarshal(msg, &cs); err != nil {
			return err
		}
		tmpl, err := template(msg)
		if err != nil {
			return err
		}
		full := d.Name + "_" + name
		keep[full] = true
		replicas := uint64(1)
		global := false
		labels := map[string]string{}
		if cs.Deploy != nil {
			global = cs.Deploy.Mode == "global"
			if cs.Deploy.Replicas != nil {
				replicas = *cs.Deploy.Replicas
			}
			maps.Copy(labels, cs.Deploy.Labels)
		}
		labels[naming.LabelStackNamespace] = d.Name
		running := replicas
		if o.Running != nil {
			running = min(*o.Running, replicas)
		}

		prev := s.services[full]
		ss := &simService{template: tmpl}
		ss.svc = docker.Service{Name: full, Labels: labels, Image: cs.Image, RunningTasks: running, DesiredTasks: replicas}
		if !global {
			r := replicas
			ss.svc.Replicas = &r
		}
		switch {
		case prev == nil: // nuevo: sin UpdateStatus
			ss.svc.CreatedAt = now
			ss.tasks = s.tasks(full, replicas, running, o, now)
		case prev.template == tmpl || o.NoUpdate: // solo cambiaron las etiquetas, o Swarm todavía no empezó
			ss.svc.ID, ss.svc.CreatedAt, ss.svc.Update = prev.svc.ID, prev.svc.CreatedAt, prev.svc.Update
			ss.svc.RunningTasks = prev.svc.RunningTasks
			ss.tasks = prev.tasks
		default: // cambió la plantilla: Swarm la actualiza
			state := o.State
			if state == "" {
				state = docker.UpdateCompleted
			}
			started := now
			ss.svc.ID, ss.svc.CreatedAt = prev.svc.ID, prev.svc.CreatedAt
			ss.svc.Update = &docker.UpdateStatus{State: state, Message: o.Message, StartedAt: &started}
			ss.tasks = s.tasks(full, replicas, running, o, now)
		}
		s.services[full] = ss
		s.put(ss)
		if o.FailedErr != "" && o.FailedLogs > 0 && (prev == nil || prev.template != tmpl) {
			lines := make([]docker.LogLine, o.FailedLogs)
			for i := range lines {
				lines[i] = docker.LogLine{Time: now.Add(time.Duration(i) * time.Millisecond), Stream: docker.StreamStderr,
					TaskID: ss.tasks[0].ID, Text: fmt.Sprintf("línea %d", i)}
			}
			s.f.AppendTaskLogs(ss.tasks[0].ID, lines...)
		}
	}
	for full, ss := range s.services {
		if ss.svc.Labels[naming.LabelStackNamespace] == d.Name && !keep[full] {
			_ = s.f.RemoveService(context.Background(), full)
			delete(s.services, full)
		}
	}
	s.returned = append(s.returned, time.Now())
	return nil
}

// tasks arma las tareas de un servicio nuevo o cambiado: la fallida primero
// (la más nueva), si hay, y una por réplica, corriendo las primeras running.
func (s *swarmSim) tasks(name string, replicas, running uint64, o simOpts, now time.Time) []docker.Task {
	var out []docker.Task
	if o.FailedErr != "" {
		out = append(out, docker.Task{ID: fmt.Sprintf("%s-fallida-%d", name, now.UnixNano()), Slot: 1,
			State: docker.TaskFailed, DesiredState: docker.TaskShutdown, Err: o.FailedErr, CreatedAt: now.Add(time.Millisecond)})
	}
	for i := range replicas {
		state := docker.TaskRunning
		if i >= running {
			state = docker.TaskStarting
		}
		out = append(out, docker.Task{ID: fmt.Sprintf("%s-%d-%d", name, i+1, now.UnixNano()), Slot: int(i + 1),
			NodeID: "node-1", State: state, DesiredState: docker.TaskRunning, CreatedAt: now})
	}
	return out
}

// template es el servicio del compose sin deploy.labels, en JSON con las
// claves ordenadas: si cambia, Swarm actualiza el servicio.
func template(msg json.RawMessage) (string, error) {
	var m map[string]any
	if err := json.Unmarshal(msg, &m); err != nil {
		return "", err
	}
	if dep, ok := m["deploy"].(map[string]any); ok {
		delete(dep, "labels")
	}
	b, err := json.Marshal(m)
	return string(b), err
}

// composeService es el servicio name del compose de un docker stack deploy.
func composeService(d docker.StackDeploy, name string) (map[string]any, error) {
	var raw struct {
		Services map[string]map[string]any `json:"services"`
	}
	if err := json.Unmarshal(d.Compose, &raw); err != nil {
		return nil, err
	}
	svc, ok := raw.Services[name]
	if !ok {
		return nil, fmt.Errorf("el compose no tiene el servicio %s: %s", name, strings.Join(slicesKeys(raw.Services), ", "))
	}
	return svc, nil
}

func slicesKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
