package usecases

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
)

// Los plazos de GET …/status (contrato §2.8).
const (
	// statusStructureTTL es cuánto se guarda lo estructural de una app
	// (servicios, réplicas, tareas, health): poco, para que un deploy en
	// curso no se vea atrasado.
	statusStructureTTL = 2 * time.Second
	// statusMeasureTTL es cuánto se guardan las medidas (CPU y memoria de
	// cada tarea): medir es caro (ContainerStats tarda cerca de 1 s).
	statusMeasureTTL = 5 * time.Second
	// statusMeasureTimeout es el plazo total de una medición, y
	// statusMeasureParallel cuántas muestras se toman a la vez.
	statusMeasureTimeout  = 2 * time.Second
	statusMeasureParallel = 8
	// statusReadTimeout acota una lectura de lo estructural, que no depende
	// del ctx de quien la pidió (la comparten los que esperan).
	statusReadTimeout = 15 * time.Second
	// statusNodeTTL es cuánto se guardan el hostname de cada nodo y el id del
	// nodo local.
	statusNodeTTL = 5 * time.Minute
)

// GetStatus es el estado real en Swarm de los servicios de una app (contrato
// §2.8), con dos cachés por app: lo estructural (2 s) y las medidas (5 s).
// Varios GET de la misma app comparten la misma lectura: con la caché
// vencida, el primero lee y los que llegan mientras tanto esperan su
// resultado (cada uno, también su propio ctx). Un error no queda en la caché.
type GetStatus struct {
	apps    service.Apps
	cluster service.Cluster
	env     service.EnvResolver
	clock   clock.Clock

	mu        sync.Mutex
	structure map[int64]*flight[statusSnapshot]
	measures  map[int64]*flight[map[string]service.Stats]
	nodes     map[string]timed[string] // id del nodo → hostname
	local     timed[string]            // el id del nodo local
}

func NewGetStatus(apps service.Apps, cluster service.Cluster, env service.EnvResolver, c clock.Clock) *GetStatus {
	return &GetStatus{
		apps: apps, cluster: cluster, env: env, clock: c,
		structure: map[int64]*flight[statusSnapshot]{},
		measures:  map[int64]*flight[map[string]service.Stats]{},
		nodes:     map[string]timed[string]{},
	}
}

// statusSnapshot es lo estructural de una app, ya enmascarado: los
// servicios sin medidas, y las tareas que se miden (las que corren en el
// nodo local), por id con su contenedor.
type statusSnapshot struct {
	services []dto.ServiceStatus
	targets  map[string]string // id de la tarea → id del contenedor
}

type timed[T any] struct {
	val T
	at  time.Time
}

// Execute: la app (404); lo estructural (503 con Docker caído); las medidas
// (nunca fallan: una tarea que no se pudo medir sale con null).
func (u *GetStatus) Execute(ctx context.Context, ref dto.AppRef) (dto.AppStatus, error) {
	app, err := u.apps.Get(ctx, ref.Project, ref.Stage, ref.App)
	if err != nil {
		return dto.AppStatus{}, err
	}
	snap, err := load(ctx, &u.mu, u.structure, app.ID, statusStructureTTL, u.clock.Now, statusReadTimeout,
		func(ctx context.Context) (statusSnapshot, error) { return u.read(ctx, app) })
	if err != nil {
		return dto.AppStatus{}, err
	}
	stats := map[string]service.Stats{}
	if len(snap.targets) > 0 {
		stats, err = load(ctx, &u.mu, u.measures, app.ID, statusMeasureTTL, u.clock.Now, statusMeasureTimeout,
			func(ctx context.Context) (map[string]service.Stats, error) { return u.measure(ctx, snap.targets), nil })
		if err != nil {
			return dto.AppStatus{}, err
		}
	}
	return withMeasures(snap.services, stats), nil
}

// read lee de Swarm los servicios de la app y sus tareas.
func (u *GetStatus) read(ctx context.Context, app service.App) (statusSnapshot, error) {
	svcs, err := u.cluster.AppServices(ctx, app.Project, app.Stage, app.Slug)
	if err != nil {
		return statusSnapshot{}, err
	}
	snap := statusSnapshot{services: []dto.ServiceStatus{}, targets: map[string]string{}}
	if len(svcs) == 0 {
		return snap, nil
	}
	local, err := u.localNode(ctx)
	if err != nil {
		return statusSnapshot{}, err
	}
	// Task.Err puede traer un argumento de command ya interpolado: se
	// enmascara como el log, y así queda en la caché.
	_, secrets, err := u.env.ResolveEnv(ctx, app.ID)
	if err != nil {
		return statusSnapshot{}, err
	}
	m := service.NewMasker(secrets)
	prefix := naming.Stack(app.Project, app.Stage, app.Slug) + "_"
	for _, s := range svcs {
		tasks, err := u.cluster.Tasks(ctx, s.ID)
		if errors.Is(err, service.ErrSwarmNotFound) {
			continue // se borró entre la lista y sus tareas
		}
		if err != nil {
			return statusSnapshot{}, err
		}
		out := dto.ServiceStatus{
			Name:            strings.TrimPrefix(s.Name, prefix),
			Image:           s.Image,
			ReplicasDesired: s.DesiredTasks,
			ReplicasRunning: s.RunningTasks,
			Tasks:           make([]dto.TaskStatus, 0, len(tasks)),
		}
		if s.Replicas != nil {
			out.ReplicasDesired = *s.Replicas
		}
		if s.MemoryLimitBytes > 0 {
			out.MemoryLimitBytes = new(s.MemoryLimitBytes)
		}
		if s.Update != nil && s.Update.State != "" {
			out.UpdateState = new(s.Update.State)
		}
		sortTasks(tasks)
		for _, t := range tasks {
			ts := dto.TaskStatus{ID: t.ID, State: t.State, DesiredState: t.DesiredState}
			if s.Replicas != nil {
				ts.Slot = new(t.Slot)
			}
			if !t.UpdatedAt.IsZero() {
				ts.Since = new(t.UpdatedAt.UTC().Truncate(time.Millisecond))
			}
			if t.Err != "" {
				ts.Error = new(m.Mask(t.Err))
			}
			if t.NodeID != "" {
				ts.Node = new(u.hostname(ctx, t.NodeID))
			}
			onLocal := t.NodeID == local && t.ContainerID != ""
			if onLocal && t.DesiredState == "running" {
				if h, err := u.cluster.ContainerHealth(ctx, t.ContainerID); err == nil && h != "" {
					ts.Health = new(h)
				}
			}
			if onLocal && t.State == "running" {
				snap.targets[t.ID] = t.ContainerID
			}
			out.Tasks = append(out.Tasks, ts)
		}
		snap.services = append(snap.services, out)
	}
	slices.SortFunc(snap.services, func(a, b dto.ServiceStatus) int { return cmp.Compare(a.Name, b.Name) })
	return snap, nil
}

// sortTasks: primero las de desired_state running; después por slot y por
// UpdatedAt, de la más nueva a la más vieja (contrato §2.8).
func sortTasks(tasks []service.SwarmTask) {
	rank := func(t service.SwarmTask) int {
		if t.DesiredState == "running" {
			return 0
		}
		return 1
	}
	slices.SortStableFunc(tasks, func(a, b service.SwarmTask) int {
		return cmp.Or(cmp.Compare(rank(a), rank(b)), cmp.Compare(a.Slot, b.Slot), b.UpdatedAt.Compare(a.UpdatedAt))
	})
}

// measure toma una muestra de cada contenedor, hasta statusMeasureParallel a
// la vez y dentro del plazo de ctx; la que falla o no llega no está.
func (u *GetStatus) measure(ctx context.Context, targets map[string]string) map[string]service.Stats {
	var mu sync.Mutex
	out := make(map[string]service.Stats, len(targets))
	sem := make(chan struct{}, statusMeasureParallel)
	var wg sync.WaitGroup
	for task, ctr := range targets {
		wg.Go(func() {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			s, err := u.cluster.ContainerStats(ctx, ctr)
			if err != nil || ctx.Err() != nil {
				return
			}
			mu.Lock()
			out[task] = s
			mu.Unlock()
		})
	}
	wg.Wait()
	return out
}

// withMeasures copia los servicios con las medidas de stats, y las sumas
// de cada servicio (null si no se midió ninguna de sus tareas).
func withMeasures(services []dto.ServiceStatus, stats map[string]service.Stats) dto.AppStatus {
	out := dto.AppStatus{Services: make([]dto.ServiceStatus, len(services))}
	for i, s := range services {
		s.Tasks = slices.Clone(s.Tasks)
		var cpu float64
		var mem uint64
		measured := false
		for j, t := range s.Tasks {
			st, ok := stats[t.ID]
			if !ok {
				continue
			}
			measured = true
			s.Tasks[j].CPUPercent, s.Tasks[j].MemoryBytes = new(st.CPUPercent), new(st.MemoryBytes)
			cpu += st.CPUPercent
			mem += st.MemoryBytes
		}
		if measured {
			s.CPUPercent, s.MemoryBytes = new(cpu), new(mem)
		}
		out.Services[i] = s
	}
	return out
}

// localNode es el id del nodo local, guardado 5 min.
func (u *GetStatus) localNode(ctx context.Context) (string, error) {
	u.mu.Lock()
	l := u.local
	u.mu.Unlock()
	if l.val != "" && u.clock.Now().Sub(l.at) < statusNodeTTL {
		return l.val, nil
	}
	id, err := u.cluster.LocalNodeID(ctx)
	if err != nil {
		return "", err
	}
	u.mu.Lock()
	u.local = timed[string]{val: id, at: u.clock.Now()}
	u.mu.Unlock()
	return id, nil
}

// hostname es el hostname del nodo, guardado 5 min; si no se puede leer (el
// nodo ya no está), su id, sin guardarlo.
func (u *GetStatus) hostname(ctx context.Context, nodeID string) string {
	u.mu.Lock()
	n, ok := u.nodes[nodeID]
	u.mu.Unlock()
	if ok && u.clock.Now().Sub(n.at) < statusNodeTTL {
		return n.val
	}
	h, err := u.cluster.NodeHostname(ctx, nodeID)
	if err != nil || h == "" {
		return nodeID
	}
	u.mu.Lock()
	u.nodes[nodeID] = timed[string]{val: h, at: u.clock.Now()}
	u.mu.Unlock()
	return h
}

// flight es una lectura de una caché: en curso hasta que done se cierra, y
// después su resultado y cuándo terminó.
type flight[T any] struct {
	done chan struct{}
	val  T
	err  error
	at   time.Time
}

// load devuelve el valor de key en m si se leyó hace menos de ttl; si hay
// una lectura en curso, espera su resultado; si no, la empieza con fetch.
// fetch corre en su propia goroutine con un ctx sin la cancelación de quien
// la empezó (la comparten los que esperan) y con el plazo timeout; cada uno
// espera también su propio ctx. Un error se devuelve a los que esperaban y no
// queda en m. mu protege m.
func load[T any](ctx context.Context, mu *sync.Mutex, m map[int64]*flight[T], key int64, ttl time.Duration,
	now func() time.Time, timeout time.Duration, fetch func(context.Context) (T, error)) (T, error) {
	mu.Lock()
	f := m[key]
	if f != nil {
		select {
		case <-f.done:
			if now().Sub(f.at) < ttl {
				mu.Unlock()
				return f.val, nil
			}
			f = nil // vencida: se vuelve a leer
		default: // en curso: se espera
		}
	}
	if f == nil {
		f = &flight[T]{done: make(chan struct{})}
		m[key] = f
		pruneFlights(m, now(), ttl)
		go func() {
			fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
			defer cancel()
			v, err := fetch(fctx)
			mu.Lock()
			f.val, f.err, f.at = v, err, now()
			if err != nil && m[key] == f {
				delete(m, key)
			}
			mu.Unlock()
			close(f.done)
		}()
	}
	mu.Unlock()
	select {
	case <-f.done:
		return f.val, f.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

// pruneFlights borra las lecturas terminadas que vencieron hace rato (de
// apps que ya nadie mira, o que se borraron). Se llama con mu tomado.
func pruneFlights[T any](m map[int64]*flight[T], now time.Time, ttl time.Duration) {
	for k, f := range m {
		select {
		case <-f.done:
			if now.Sub(f.at) > 10*ttl {
				delete(m, k)
			}
		default:
		}
	}
}
