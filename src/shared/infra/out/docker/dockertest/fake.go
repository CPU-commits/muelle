// Package dockertest es un Docker en memoria (docker.Engine) para los tests de
// los casos de uso de deploy, ingress e install. Imita lo que esos módulos
// necesitan del daemon real: redes que se conectan en caliente, contenedores
// que exigen la imagen y fallan con un puerto del host ocupado, stacks cuyo
// efecto decide el test (OnDeployStack), digests de registry, tareas, logs y
// stats. Devuelve los mismos errores que el cliente real (docker.ErrNotFound,
// ErrConflict, ErrDenied, ErrUnavailable, ErrCommand) y registra las llamadas.
//
// Es seguro usarlo desde varias goroutines.
package dockertest

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

type service struct {
	svc   docker.Service
	tasks []docker.Task
	logs  *logBuffer
}

type secret struct {
	docker.Secret
	data []byte
}

// DistributionCall es una consulta de digest, con las credenciales que llevó.
type DistributionCall struct {
	Ref  string
	Auth *docker.RegistryAuth
}

// Fake es el Docker en memoria. El cero no sirve: se crea con New.
type Fake struct {
	// OnDeployStack, si no es nil, corre en cada DeployStack (sin el lock
	// tomado, así puede llamar a PutStackService): simula lo que crea el stack.
	// Un error suyo hace fallar el deploy con docker.ErrCommand.
	OnDeployStack func(d docker.StackDeploy) error
	// Now es el reloj de StartedAt y CreatedAt; por defecto time.Now.
	Now func() time.Time

	mu          sync.Mutex
	info        docker.Info
	nodes       map[string]docker.Node
	services    map[string]*service // por nombre
	networks    map[string]*docker.Network
	containers  map[string]*docker.Container // por nombre
	volumes     map[string]docker.Volume
	secrets     map[string]*secret
	digests     map[string]string
	denied      map[string]bool
	images      map[string]bool
	stats       map[string]docker.Stats
	taskLogs    map[string]*logBuffer
	busyPorts   map[int]bool
	unavailable bool
	failNext    map[string]error
	nextID      int

	calls        []string
	deploys      []docker.StackDeploy
	removed      []string
	distribution []DistributionCall
	pulls        []string
}

var _ docker.Engine = (*Fake)(nil)

// New arma un Docker con un Swarm activo de un nodo manager ("node-1", hostname "manager-1").
func New() *Fake {
	f := &Fake{
		Now:        time.Now,
		nodes:      map[string]docker.Node{},
		services:   map[string]*service{},
		networks:   map[string]*docker.Network{},
		containers: map[string]*docker.Container{},
		volumes:    map[string]docker.Volume{},
		secrets:    map[string]*secret{},
		digests:    map[string]string{},
		denied:     map[string]bool{},
		images:     map[string]bool{},
		stats:      map[string]docker.Stats{},
		taskLogs:   map[string]*logBuffer{},
		busyPorts:  map[int]bool{},
		failNext:   map[string]error{},
		info: docker.Info{
			ServerVersion: "29.6.1", Hostname: "manager-1", NCPU: 4, MemTotal: 8 << 30,
			SwarmState: docker.SwarmActive, NodeID: "node-1", NodeAddr: "10.0.0.2", IsManager: true,
		},
	}
	f.nodes["node-1"] = docker.Node{ID: "node-1", Hostname: "manager-1", Role: "manager", State: "ready", Availability: "active", Addr: "10.0.0.2"}
	return f
}

// --- Control desde los tests ----------------------------------------------------------

// SetUnavailable hace que todas las llamadas fallen con docker.ErrUnavailable.
func (f *Fake) SetUnavailable(down bool) { f.mu.Lock(); f.unavailable = down; f.mu.Unlock() }

// FailNext hace fallar con err la próxima llamada al método op ("CreateNetwork", "DeployStack"…).
func (f *Fake) FailNext(op string, err error) { f.mu.Lock(); f.failNext[op] = err; f.mu.Unlock() }

func (f *Fake) SetInfo(i docker.Info) { f.mu.Lock(); f.info = i; f.mu.Unlock() }
func (f *Fake) PutNode(n docker.Node) { f.mu.Lock(); f.nodes[n.ID] = n; f.mu.Unlock() }

// PutService crea o reemplaza un servicio con sus tareas. Sin ID, se le asigna
// uno; las tareas reciben el ServiceID.
func (f *Fake) PutService(s docker.Service, tasks ...docker.Task) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.putServiceLocked(s, tasks)
}

// PutStackService es PutService con la etiqueta de stack que pone docker stack deploy.
func (f *Fake) PutStackService(stack string, s docker.Service, tasks ...docker.Task) {
	labels := maps.Clone(s.Labels)
	if labels == nil {
		labels = map[string]string{}
	}
	labels[naming.LabelStackNamespace] = stack
	s.Labels = labels
	f.PutService(s, tasks...)
}

func (f *Fake) putServiceLocked(s docker.Service, tasks []docker.Task) {
	prev := f.services[s.Name]
	if s.ID == "" {
		if prev != nil {
			s.ID = prev.svc.ID
		} else {
			s.ID = f.id("svc")
		}
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = f.Now()
	}
	for i := range tasks {
		tasks[i].ServiceID = s.ID
		if tasks[i].ID == "" {
			tasks[i].ID = f.id("task")
		}
		if tasks[i].CreatedAt.IsZero() {
			tasks[i].CreatedAt = f.Now()
		}
	}
	logs := &logBuffer{notify: make(chan struct{})}
	if prev != nil {
		logs = prev.logs
	}
	f.services[s.Name] = &service{svc: s, tasks: tasks, logs: logs}
}

// SetTasks reemplaza las tareas de un servicio que ya existe.
func (f *Fake) SetTasks(serviceName string, tasks ...docker.Task) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.services[serviceName]
	if s == nil {
		panic("dockertest: SetTasks de un servicio que no existe: " + serviceName)
	}
	f.putServiceLocked(s.svc, tasks)
}

// AppendServiceLogs agrega líneas al log de un servicio (y las entrega a quien lo sigue).
func (f *Fake) AppendServiceLogs(serviceName string, lines ...docker.LogLine) {
	f.mu.Lock()
	s := f.services[serviceName]
	f.mu.Unlock()
	if s == nil {
		panic("dockertest: AppendServiceLogs de un servicio que no existe: " + serviceName)
	}
	s.logs.append(lines)
}

// AppendTaskLogs agrega líneas al log de una tarea.
func (f *Fake) AppendTaskLogs(taskID string, lines ...docker.LogLine) {
	f.mu.Lock()
	b := f.taskLogs[taskID]
	if b == nil {
		b = &logBuffer{notify: make(chan struct{})}
		f.taskLogs[taskID] = b
	}
	f.mu.Unlock()
	b.append(lines)
}

// SetDigest hace que ref exista en el registry con ese digest.
func (f *Fake) SetDigest(ref, digest string) { f.mu.Lock(); f.digests[ref] = digest; f.mu.Unlock() }

// DenyImage hace que el registry rechace ref (sin permiso), también al bajarla.
func (f *Fake) DenyImage(ref string) { f.mu.Lock(); f.denied[ref] = true; f.mu.Unlock() }

// SetImagePresent deja la imagen en el nodo, como si ya se hubiera bajado.
func (f *Fake) SetImagePresent(ref string) { f.mu.Lock(); f.images[ref] = true; f.mu.Unlock() }

// SetStats fija la muestra de stats de un contenedor (por nombre o id).
func (f *Fake) SetStats(container string, s docker.Stats) {
	f.mu.Lock()
	f.stats[container] = s
	f.mu.Unlock()
}

// BusyHostPort marca un puerto del host como ocupado por otro proceso.
func (f *Fake) BusyHostPort(port int) { f.mu.Lock(); f.busyPorts[port] = true; f.mu.Unlock() }
func (f *Fake) FreeHostPort(port int) { f.mu.Lock(); delete(f.busyPorts, port); f.mu.Unlock() }

// PutVolume crea un volumen (los de un stack los crearía docker stack deploy).
func (f *Fake) PutVolume(v docker.Volume) { f.mu.Lock(); f.volumes[v.Name] = v; f.mu.Unlock() }

// --- Lo que pasó --------------------------------------------------------------------------

// Calls son las llamadas en orden, como "Método arg1 arg2".
func (f *Fake) Calls() []string { f.mu.Lock(); defer f.mu.Unlock(); return slices.Clone(f.calls) }

// StackDeploys son los docker stack deploy ejecutados (también los que fallaron).
func (f *Fake) StackDeploys() []docker.StackDeploy {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.deploys)
}

func (f *Fake) RemovedStacks() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.removed)
}

func (f *Fake) DistributionCalls() []DistributionCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.distribution)
}

func (f *Fake) Pulls() []string { f.mu.Lock(); defer f.mu.Unlock(); return slices.Clone(f.pulls) }

// SecretData devuelve el valor de un secreto (el Docker real no lo deja leer).
func (f *Fake) SecretData(name string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.secrets[name]
	if !ok {
		return nil, false
	}
	return slices.Clone(s.data), true
}

// --- Internos ------------------------------------------------------------------------------

func (f *Fake) id(prefix string) string {
	f.nextID++
	return fmt.Sprintf("%s-%d", prefix, f.nextID)
}

// begin registra la llamada y devuelve el error que corresponda: Docker caído
// o un FailNext pendiente. Se llama con el lock tomado.
func (f *Fake) begin(op string, args ...string) error {
	f.calls = append(f.calls, strings.TrimSpace(op+" "+strings.Join(args, " ")))
	if f.unavailable {
		return fmt.Errorf("dockertest: %s: %w", op, docker.ErrUnavailable)
	}
	if err, ok := f.failNext[op]; ok {
		delete(f.failNext, op)
		return err
	}
	return nil
}

// swarm devuelve el error del daemon cuando el nodo no está en un Swarm
// activo (un 503, como "This node is not a swarm manager"). Se llama con el
// lock tomado.
func (f *Fake) swarm(op string) error {
	if f.info.SwarmState != docker.SwarmActive {
		return fmt.Errorf("dockertest: %s: el nodo no es manager de un swarm: %w", op, docker.ErrUnavailable)
	}
	return nil
}

func copyService(s docker.Service) docker.Service {
	s.Labels = maps.Clone(s.Labels)
	if s.Replicas != nil {
		n := *s.Replicas
		s.Replicas = &n
	}
	if s.Update != nil {
		u := *s.Update
		s.Update = &u
	}
	return s
}

func notFound(what, name string) error {
	return fmt.Errorf("dockertest: %s %s: %w", what, name, docker.ErrNotFound)
}

func conflict(what, name string) error {
	return fmt.Errorf("dockertest: %s %s: %w", what, name, docker.ErrConflict)
}

func hasLabels(have, want map[string]string) bool {
	for k, v := range want {
		if have[k] != v {
			return false
		}
	}
	return true
}

// --- Sistema -------------------------------------------------------------------------------

func (f *Fake) Ping(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.begin("Ping")
}

func (f *Fake) Info(ctx context.Context) (docker.Info, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("Info"); err != nil {
		return docker.Info{}, err
	}
	return f.info, nil
}

func (f *Fake) Node(ctx context.Context, id string) (docker.Node, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("Node", id); err != nil {
		return docker.Node{}, err
	}
	n, ok := f.nodes[id]
	if !ok {
		return docker.Node{}, notFound("nodo", id)
	}
	return n, nil
}

func (f *Fake) InitSwarm(ctx context.Context, advertiseAddr string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("InitSwarm", advertiseAddr); err != nil {
		return "", err
	}
	if f.info.SwarmState == docker.SwarmActive {
		return "", conflict("swarm", "ya activo")
	}
	f.info.SwarmState, f.info.IsManager = docker.SwarmActive, true
	if f.info.NodeID == "" {
		f.info.NodeID = "node-1"
	}
	return f.info.NodeID, nil
}

// --- Servicios -----------------------------------------------------------------------------

func (f *Fake) findService(nameOrID string) *service {
	if s, ok := f.services[nameOrID]; ok {
		return s
	}
	for _, s := range f.services {
		if s.svc.ID == nameOrID {
			return s
		}
	}
	return nil
}

func (f *Fake) ListServices(ctx context.Context, labels map[string]string) ([]docker.Service, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ListServices"); err != nil {
		return nil, err
	}
	if err := f.swarm("ListServices"); err != nil {
		return nil, err
	}
	var out []docker.Service
	for _, s := range f.services {
		if hasLabels(s.svc.Labels, labels) {
			out = append(out, copyService(s.svc))
		}
	}
	slices.SortFunc(out, func(a, b docker.Service) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func (f *Fake) InspectService(ctx context.Context, nameOrID string) (docker.Service, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("InspectService", nameOrID); err != nil {
		return docker.Service{}, err
	}
	if err := f.swarm("InspectService"); err != nil {
		return docker.Service{}, err
	}
	s := f.findService(nameOrID)
	if s == nil {
		return docker.Service{}, notFound("servicio", nameOrID)
	}
	return copyService(s.svc), nil
}

func (f *Fake) RemoveService(ctx context.Context, nameOrID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("RemoveService", nameOrID); err != nil {
		return err
	}
	s := f.findService(nameOrID)
	if s == nil {
		return notFound("servicio", nameOrID)
	}
	delete(f.services, s.svc.Name)
	return nil
}

func (f *Fake) ListTasks(ctx context.Context, filter docker.TaskFilter) ([]docker.Task, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ListTasks", filter.Service); err != nil {
		return nil, err
	}
	if err := f.swarm("ListTasks"); err != nil {
		return nil, err
	}
	var out []docker.Task
	for _, s := range f.services {
		if filter.Service != "" && s.svc.Name != filter.Service && s.svc.ID != filter.Service {
			continue
		}
		if !hasLabels(s.svc.Labels, filter.Labels) {
			continue
		}
		for _, t := range s.tasks {
			if filter.DesiredState == "" || t.DesiredState == filter.DesiredState {
				out = append(out, t)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b docker.Task) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out, nil
}

func (f *Fake) ServiceLogs(ctx context.Context, nameOrID string, o docker.LogOptions) (docker.LogStream, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ServiceLogs", nameOrID); err != nil {
		return nil, err
	}
	s := f.findService(nameOrID)
	if s == nil {
		return nil, notFound("servicio", nameOrID)
	}
	return s.logs.stream(ctx, o), nil
}

func (f *Fake) TaskLogs(ctx context.Context, taskID string, o docker.LogOptions) (docker.LogStream, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("TaskLogs", taskID); err != nil {
		return nil, err
	}
	b := f.taskLogs[taskID]
	if b == nil {
		found := false
		for _, s := range f.services {
			for _, t := range s.tasks {
				found = found || t.ID == taskID
			}
		}
		if !found {
			return nil, notFound("tarea", taskID)
		}
		b = &logBuffer{notify: make(chan struct{})}
		f.taskLogs[taskID] = b
	}
	return b.stream(ctx, o), nil
}

// --- Redes ----------------------------------------------------------------------------------

func (f *Fake) CreateNetwork(ctx context.Context, spec docker.NetworkSpec) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("CreateNetwork", spec.Name); err != nil {
		return "", err
	}
	if spec.Driver == "overlay" {
		if err := f.swarm("CreateNetwork"); err != nil {
			return "", err
		}
	}
	if _, ok := f.networks[spec.Name]; ok {
		return "", conflict("red", spec.Name)
	}
	scope := "local"
	if spec.Driver == "overlay" {
		scope = "swarm"
	}
	n := &docker.Network{
		ID: f.id("net"), Name: spec.Name, Driver: spec.Driver, Scope: scope,
		Attachable: spec.Attachable, Internal: spec.Internal, Labels: maps.Clone(spec.Labels),
		Subnets: []string{fmt.Sprintf("10.0.%d.0/24", len(f.networks)+1)},
	}
	f.networks[spec.Name] = n
	return n.ID, nil
}

func (f *Fake) findNetwork(nameOrID string) *docker.Network {
	if n, ok := f.networks[nameOrID]; ok {
		return n
	}
	for _, n := range f.networks {
		if n.ID == nameOrID {
			return n
		}
	}
	return nil
}

func copyNetwork(n *docker.Network) docker.Network {
	c := *n
	c.Labels = maps.Clone(n.Labels)
	c.Subnets = slices.Clone(n.Subnets)
	c.Containers = slices.Clone(n.Containers)
	slices.Sort(c.Containers)
	return c
}

func (f *Fake) InspectNetwork(ctx context.Context, nameOrID string) (docker.Network, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("InspectNetwork", nameOrID); err != nil {
		return docker.Network{}, err
	}
	n := f.findNetwork(nameOrID)
	if n == nil {
		return docker.Network{}, notFound("red", nameOrID)
	}
	return copyNetwork(n), nil
}

func (f *Fake) ListNetworks(ctx context.Context, labels map[string]string) ([]docker.Network, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ListNetworks"); err != nil {
		return nil, err
	}
	var out []docker.Network
	for _, n := range f.networks {
		if hasLabels(n.Labels, labels) {
			c := copyNetwork(n)
			c.Containers = nil // la lista de Docker no trae los contenedores
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b docker.Network) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func (f *Fake) RemoveNetwork(ctx context.Context, nameOrID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("RemoveNetwork", nameOrID); err != nil {
		return err
	}
	n := f.findNetwork(nameOrID)
	if n == nil {
		return notFound("red", nameOrID)
	}
	if len(n.Containers) > 0 {
		return conflict("red con contenedores conectados", n.Name)
	}
	delete(f.networks, n.Name)
	return nil
}

func (f *Fake) ConnectNetwork(ctx context.Context, network, container string, aliases []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ConnectNetwork", network, container); err != nil {
		return err
	}
	n := f.findNetwork(network)
	if n == nil {
		return notFound("red", network)
	}
	c := f.findContainer(container)
	if c == nil {
		return notFound("contenedor", container)
	}
	if slices.Contains(c.Networks, n.Name) {
		return conflict("ya conectado a "+n.Name, c.Name)
	}
	c.Networks = append(c.Networks, n.Name)
	slices.Sort(c.Networks)
	n.Containers = append(n.Containers, c.Name)
	return nil
}

func (f *Fake) DisconnectNetwork(ctx context.Context, network, container string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DisconnectNetwork", network, container); err != nil {
		return err
	}
	n := f.findNetwork(network)
	if n == nil {
		return notFound("red", network)
	}
	c := f.findContainer(container)
	if c == nil || !slices.Contains(c.Networks, n.Name) {
		return notFound("contenedor conectado a "+n.Name, container)
	}
	c.Networks = slices.DeleteFunc(c.Networks, func(s string) bool { return s == n.Name })
	n.Containers = slices.DeleteFunc(n.Containers, func(s string) bool { return s == c.Name })
	return nil
}

// --- Contenedores -----------------------------------------------------------------------------

func (f *Fake) findContainer(nameOrID string) *docker.Container {
	if c, ok := f.containers[nameOrID]; ok {
		return c
	}
	for _, c := range f.containers {
		if c.ID == nameOrID {
			return c
		}
	}
	return nil
}

func copyContainer(c *docker.Container) docker.Container {
	out := *c
	out.Cmd = slices.Clone(c.Cmd)
	out.Labels = maps.Clone(c.Labels)
	out.Ports = slices.Clone(c.Ports)
	out.Mounts = slices.Clone(c.Mounts)
	out.Networks = slices.Clone(c.Networks)
	return out
}

func (f *Fake) CreateContainer(ctx context.Context, spec docker.ContainerSpec) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("CreateContainer", spec.Name, spec.Image); err != nil {
		return "", err
	}
	if _, ok := f.containers[spec.Name]; ok {
		return "", conflict("contenedor", spec.Name)
	}
	if !f.images[spec.Image] {
		return "", notFound("imagen (falta PullImage)", spec.Image)
	}
	c := &docker.Container{
		ID: f.id("ctr"), Name: spec.Name, Image: spec.Image, ImageID: "sha256:" + spec.Image,
		Cmd: slices.Clone(spec.Cmd), Labels: maps.Clone(spec.Labels), Status: "created",
		RestartPolicy: spec.RestartPolicy, Ports: slices.Clone(spec.Ports),
		Mounts: slices.Clone(spec.Mounts),
	}
	for i := range c.Ports {
		c.Ports[i].Protocol = cmp.Or(c.Ports[i].Protocol, "tcp")
	}
	if spec.Network != "" {
		n := f.findNetwork(spec.Network)
		if n == nil {
			return "", notFound("red", spec.Network)
		}
		c.Networks = []string{n.Name}
		n.Containers = append(n.Containers, c.Name)
	}
	f.containers[spec.Name] = c
	return c.ID, nil
}

func (f *Fake) InspectContainer(ctx context.Context, nameOrID string) (docker.Container, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("InspectContainer", nameOrID); err != nil {
		return docker.Container{}, err
	}
	c := f.findContainer(nameOrID)
	if c == nil {
		return docker.Container{}, notFound("contenedor", nameOrID)
	}
	return copyContainer(c), nil
}

func (f *Fake) StartContainer(ctx context.Context, nameOrID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("StartContainer", nameOrID); err != nil {
		return err
	}
	c := f.findContainer(nameOrID)
	if c == nil {
		return notFound("contenedor", nameOrID)
	}
	if c.Running {
		return nil
	}
	for _, p := range c.Ports {
		busy := f.busyPorts[p.HostPort]
		for _, other := range f.containers {
			if other != c && other.Running && slices.ContainsFunc(other.Ports, func(o docker.PortBinding) bool { return o.HostPort == p.HostPort }) {
				busy = true
			}
		}
		if busy {
			c.Status = "created"
			return fmt.Errorf("dockertest: arrancando %s: Bind for 0.0.0.0:%d failed: port is already allocated", c.Name, p.HostPort)
		}
	}
	c.Running, c.Status, c.StartedAt = true, "running", f.Now()
	return nil
}

func (f *Fake) StopContainer(ctx context.Context, nameOrID string, timeout time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("StopContainer", nameOrID); err != nil {
		return err
	}
	c := f.findContainer(nameOrID)
	if c == nil {
		return notFound("contenedor", nameOrID)
	}
	c.Running, c.Status = false, "exited"
	return nil
}

func (f *Fake) RemoveContainer(ctx context.Context, nameOrID string, force bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("RemoveContainer", nameOrID); err != nil {
		return err
	}
	c := f.findContainer(nameOrID)
	if c == nil {
		return notFound("contenedor", nameOrID)
	}
	if c.Running && !force {
		return conflict("contenedor corriendo (falta force)", c.Name)
	}
	for _, name := range c.Networks {
		if n := f.networks[name]; n != nil {
			n.Containers = slices.DeleteFunc(n.Containers, func(s string) bool { return s == c.Name })
		}
	}
	delete(f.containers, c.Name)
	return nil
}

func (f *Fake) ContainerStats(ctx context.Context, nameOrID string) (docker.Stats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ContainerStats", nameOrID); err != nil {
		return docker.Stats{}, err
	}
	if s, ok := f.stats[nameOrID]; ok {
		return s, nil
	}
	if c := f.findContainer(nameOrID); c != nil {
		if s, ok := f.stats[c.Name]; ok {
			return s, nil
		}
		return docker.Stats{}, nil
	}
	return docker.Stats{}, notFound("contenedor", nameOrID)
}

// --- Volúmenes ------------------------------------------------------------------------------

func (f *Fake) ListVolumes(ctx context.Context, labels map[string]string) ([]docker.Volume, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ListVolumes"); err != nil {
		return nil, err
	}
	var out []docker.Volume
	for _, v := range f.volumes {
		if hasLabels(v.Labels, labels) {
			out = append(out, v)
		}
	}
	slices.SortFunc(out, func(a, b docker.Volume) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func (f *Fake) RemoveVolume(ctx context.Context, name string, force bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("RemoveVolume", name); err != nil {
		return err
	}
	if _, ok := f.volumes[name]; !ok {
		return notFound("volumen", name)
	}
	delete(f.volumes, name)
	return nil
}

// --- Imágenes -------------------------------------------------------------------------------

func copyAuth(a *docker.RegistryAuth) *docker.RegistryAuth {
	if a == nil {
		return nil
	}
	c := *a
	return &c
}

func (f *Fake) DistributionDigest(ctx context.Context, ref string, auth *docker.RegistryAuth) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DistributionDigest", ref); err != nil {
		return "", err
	}
	f.distribution = append(f.distribution, DistributionCall{Ref: ref, Auth: copyAuth(auth)})
	if f.denied[ref] {
		return "", fmt.Errorf("dockertest: resolviendo %s: %w", ref, docker.ErrDenied)
	}
	d, ok := f.digests[ref]
	if !ok {
		return "", notFound("imagen en el registry", ref)
	}
	return d, nil
}

func (f *Fake) PullImage(ctx context.Context, ref string, auth *docker.RegistryAuth) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("PullImage", ref); err != nil {
		return err
	}
	f.pulls = append(f.pulls, ref)
	if f.denied[ref] {
		return fmt.Errorf("dockertest: bajando %s: %w", ref, docker.ErrDenied)
	}
	f.images[ref] = true
	return nil
}

// --- Secretos -------------------------------------------------------------------------------

func (f *Fake) CreateSecret(ctx context.Context, name string, data []byte, labels map[string]string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("CreateSecret", name); err != nil {
		return "", err
	}
	if err := f.swarm("CreateSecret"); err != nil {
		return "", err
	}
	if _, ok := f.secrets[name]; ok {
		return "", conflict("secreto", name)
	}
	s := &secret{Secret: docker.Secret{ID: f.id("secret"), Name: name, Labels: maps.Clone(labels), CreatedAt: f.Now()}, data: slices.Clone(data)}
	f.secrets[name] = s
	return s.ID, nil
}

func (f *Fake) InspectSecret(ctx context.Context, name string) (docker.Secret, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("InspectSecret", name); err != nil {
		return docker.Secret{}, err
	}
	s, ok := f.secrets[name]
	if !ok {
		return docker.Secret{}, notFound("secreto", name)
	}
	return s.Secret, nil
}

func (f *Fake) RemoveSecret(ctx context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("RemoveSecret", name); err != nil {
		return err
	}
	if _, ok := f.secrets[name]; !ok {
		return notFound("secreto", name)
	}
	delete(f.secrets, name)
	return nil
}

// --- Stacks ---------------------------------------------------------------------------------

// DeployStack registra el deploy y corre OnDeployStack. Si FailNext("DeployStack")
// o el hook fallan, devuelve una salida con el error y docker.ErrCommand, como el CLI.
func (f *Fake) DeployStack(ctx context.Context, d docker.StackDeploy) (string, error) {
	f.mu.Lock()
	err := f.begin("DeployStack", d.Name)
	if err == nil {
		err = f.swarm("DeployStack")
	}
	d.Compose = slices.Clone(d.Compose)
	d.Registries = slices.Clone(d.Registries)
	f.deploys = append(f.deploys, d)
	hook := f.OnDeployStack
	f.mu.Unlock()
	if err == nil && hook != nil {
		err = hook(d)
	}
	if err != nil {
		return "failed to deploy stack: " + err.Error() + "\n", fmt.Errorf("%w: docker stack deploy: %w", docker.ErrCommand, err)
	}
	return "Updating service " + d.Name + "\n", nil
}

// RemoveStack borra los servicios con la etiqueta del stack.
func (f *Fake) RemoveStack(ctx context.Context, name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("RemoveStack", name); err != nil {
		return "", fmt.Errorf("%w: docker stack rm: %w", docker.ErrCommand, err)
	}
	f.removed = append(f.removed, name)
	var out strings.Builder
	for svcName, s := range f.services {
		if s.svc.Labels[naming.LabelStackNamespace] == name {
			delete(f.services, svcName)
			fmt.Fprintf(&out, "Removing service %s\n", svcName)
		}
	}
	return out.String(), nil
}

// --- Logs ------------------------------------------------------------------------------------

// logBuffer guarda las líneas de un log y despierta a quienes lo siguen.
type logBuffer struct {
	mu     sync.Mutex
	lines  []docker.LogLine
	notify chan struct{} // se cierra y se reemplaza con cada append
}

func (b *logBuffer) append(lines []docker.LogLine) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = append(b.lines, lines...)
	close(b.notify)
	b.notify = make(chan struct{})
}

func (b *logBuffer) stream(ctx context.Context, o docker.LogOptions) *logStream {
	b.mu.Lock()
	defer b.mu.Unlock()
	start := 0
	if o.Tail >= 0 {
		start = max(len(b.lines)-o.Tail, 0)
	}
	s := &logStream{ctx: ctx, buf: b, pos: start, follow: o.Follow, since: o.Since, closed: make(chan struct{})}
	return s
}

type logStream struct {
	ctx       context.Context
	buf       *logBuffer
	pos       int
	follow    bool
	since     time.Time
	closeOnce sync.Once
	closed    chan struct{}
}

func (s *logStream) Next() (docker.LogLine, error) {
	for {
		s.buf.mu.Lock()
		if s.pos < len(s.buf.lines) {
			l := s.buf.lines[s.pos]
			s.pos++
			s.buf.mu.Unlock()
			if !s.since.IsZero() && l.Time.Before(s.since) {
				continue
			}
			return l, nil
		}
		wait := s.buf.notify
		s.buf.mu.Unlock()
		if !s.follow {
			return docker.LogLine{}, io.EOF
		}
		select {
		case <-wait:
		case <-s.ctx.Done():
			return docker.LogLine{}, s.ctx.Err()
		case <-s.closed:
			return docker.LogLine{}, io.ErrClosedPipe
		}
	}
}

func (s *logStream) Close() error {
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}
