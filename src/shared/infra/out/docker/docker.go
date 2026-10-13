// Package docker es el cliente de Docker que comparten deploy (Fase 2),
// ingress (Fase 3) e install (Fase 5): servicios y tareas de Swarm, logs y
// stats, inspección de distribución, redes, contenedores (muelle-caddy),
// volúmenes, el nodo y los secretos de Swarm, y docker stack deploy/rm por el
// CLI (§7 paso 5 del spec).
//
// Los tipos de este paquete son structs propios, sin tipos del SDK: un módulo
// declara en su domain/service el puerto que necesita y su adaptador de
// infra/out/service lo implementa con Engine (o con una de sus partes). Los
// tests de los casos de uso usan el fake de dockertest.
//
// Errores: todo método devuelve un error que envuelve ErrNotFound,
// ErrConflict, ErrDenied o ErrUnavailable cuando corresponde, para
// compararlo con errors.Is. ErrUnavailable es entity.ErrDockerUnavailable,
// que cmd/http responde como 503 docker_unavailable.
package docker

import (
	"context"
	"errors"
	"time"

	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

var (
	// ErrNotFound: lo pedido (servicio, tarea, red, contenedor, volumen,
	// secreto, imagen o tag en el registry) no existe.
	ErrNotFound = errors.New("docker: no existe")
	// ErrConflict: ya existe algo con ese nombre, o está en uso (una red con
	// contenedores conectados, un contenedor ya conectado a la red).
	ErrConflict = errors.New("docker: ya existe o está en uso")
	// ErrDenied: el registry rechazó las credenciales o no da acceso a la imagen.
	ErrDenied = errors.New("docker: el registry no da acceso a la imagen")
	// ErrUnavailable: el daemon no responde, o responde 503 porque el nodo no
	// está en un Swarm o no es manager (§13: los endpoints que usan Docker
	// responden 503).
	ErrUnavailable = entity.ErrDockerUnavailable
	// ErrCommand: el CLI de docker (stack deploy/rm) terminó con error; su
	// salida va en el valor que devuelve el método.
	ErrCommand = errors.New("docker: el comando falló")
)

// Engine es todo lo que muelle usa de Docker. Cada módulo pide solo las
// partes que necesita.
type Engine interface {
	System
	Services
	Networks
	Containers
	Volumes
	Images
	Secrets
	Stacks
}

// --- Sistema y nodo -----------------------------------------------------------

// Estados del nodo local en Swarm (Info.SwarmState).
const (
	SwarmInactive = "inactive"
	SwarmPending  = "pending"
	SwarmActive   = "active"
	SwarmError    = "error"
	SwarmLocked   = "locked"
)

// Info describe el daemon y el nodo local.
type Info struct {
	ServerVersion string
	Hostname      string
	NCPU          int
	MemTotal      int64
	SwarmState    string // Swarm*
	NodeID        string // vacío si el nodo no está en un Swarm
	NodeAddr      string
	IsManager     bool // el nodo local es manager (ControlAvailable)
}

// Node es un nodo del Swarm.
type Node struct {
	ID           string
	Hostname     string
	Role         string // "manager" o "worker"
	State        string // "ready", "down"…
	Availability string // "active", "pause", "drain"
	Addr         string
}

type System interface {
	// Ping verifica que el daemon responda (ErrUnavailable si no).
	Ping(ctx context.Context) error
	Info(ctx context.Context) (Info, error)
	// Node inspecciona un nodo del Swarm por id (para mostrar su hostname).
	Node(ctx context.Context, id string) (Node, error)
	// InitSwarm inicia un Swarm en el nodo local (solo install, §3.3 paso 1)
	// y devuelve el id del nodo. advertiseAddr vacío deja que Docker elija.
	// Nunca se llama en los tests contra el Docker real.
	InitSwarm(ctx context.Context, advertiseAddr string) (nodeID string, err error)
}

// --- Servicios, tareas y logs ---------------------------------------------------

// Estados de una actualización de servicio (Service.Update.State, §7 paso 6).
const (
	UpdateUpdating          = "updating"
	UpdatePaused            = "paused"
	UpdateCompleted         = "completed"
	UpdateRollbackStarted   = "rollback_started"
	UpdateRollbackPaused    = "rollback_paused"
	UpdateRollbackCompleted = "rollback_completed"
)

// UpdateStatus es el estado de la última actualización de un servicio. Ojo:
// queda el de la actualización anterior hasta que empieza la siguiente, así
// que quien espera un deploy compara StartedAt con el momento en que aplicó.
type UpdateStatus struct {
	State       string // Update*
	Message     string
	StartedAt   *time.Time
	CompletedAt *time.Time
}

// Service es un servicio de Swarm.
type Service struct {
	ID     string
	Name   string
	Labels map[string]string // las del servicio (deploy.labels del compose)
	Image  string            // la imagen de sus tareas (ref@sha256:…)
	// Replicas es nil en un servicio global.
	Replicas *uint64
	// RunningTasks y DesiredTasks los calcula Swarm (ServiceStatus).
	RunningTasks uint64
	DesiredTasks uint64
	// HasHealthcheck dice si sus contenedores tienen un healthcheck activo.
	HasHealthcheck bool
	// MemoryLimitBytes y NanoCPUs son los límites de cada tarea
	// (deploy.resources.limits); 0 si no tiene.
	MemoryLimitBytes int64
	NanoCPUs         int64
	// Update es nil si el servicio nunca se actualizó (recién creado).
	Update    *UpdateStatus
	Version   uint64 // índice de la versión del spec; sube con cada cambio
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Estados de una tarea de Swarm (Task.State y Task.DesiredState). Con
// healthcheck, una tarea queda en starting hasta que su contenedor está healthy.
const (
	TaskNew       = "new"
	TaskPending   = "pending"
	TaskAssigned  = "assigned"
	TaskAccepted  = "accepted"
	TaskPreparing = "preparing"
	TaskStarting  = "starting"
	TaskRunning   = "running"
	TaskComplete  = "complete"
	TaskShutdown  = "shutdown"
	TaskFailed    = "failed"
	TaskRejected  = "rejected"
	TaskRemove    = "remove"
	TaskOrphaned  = "orphaned"
)

// TaskFilter elige tareas. Los campos vacíos no filtran.
type TaskFilter struct {
	Service      string            // nombre o id del servicio
	Labels       map[string]string // etiquetas del servicio (todas), no las de la tarea
	DesiredState string            // "running", "shutdown" o "accepted"
}

// Task es una tarea de Swarm (una réplica en un momento dado). Swarm guarda
// las últimas tareas de cada slot, así que hay también las ya terminadas.
type Task struct {
	ID           string
	ServiceID    string
	NodeID       string
	Slot         int // 0 en un servicio global
	State        string
	DesiredState string
	Message      string
	Err          string // el motivo de una tarea failed o rejected
	ContainerID  string // vacío hasta que se crea el contenedor
	ExitCode     int
	Image        string
	CreatedAt    time.Time
	UpdatedAt    time.Time // el último cambio de estado ("desde")
}

// Los streams de una línea de log.
const (
	StreamStdout = "stdout"
	StreamStderr = "stderr"
)

// LogOptions elige qué líneas devolver.
type LogOptions struct {
	// Tail son las últimas N líneas; 0 con Follow es "solo lo nuevo", y un
	// valor negativo es todo el log.
	Tail int
	// Follow deja el stream abierto con las líneas nuevas hasta que se cierra
	// o se cancela el ctx.
	Follow bool
	// Since, si no es cero, descarta las líneas anteriores.
	Since time.Time
}

// LogLine es una línea de log de una tarea.
type LogLine struct {
	Time      time.Time
	Stream    string // StreamStdout o StreamStderr
	ServiceID string
	TaskID    string
	NodeID    string
	Text      string // sin el salto de línea final
}

// LogStream entrega las líneas de un log en orden. Next devuelve io.EOF al
// terminar; con Follow, termina al cancelar el ctx o con Close, con un error
// que no es io.EOF. Close se puede llamar desde otra goroutine mientras Next
// espera, y hay que llamarlo siempre.
type LogStream interface {
	Next() (LogLine, error)
	Close() error
}

type Services interface {
	// ListServices devuelve los servicios que tienen todas las etiquetas
	// pedidas (nil: todos), con RunningTasks y DesiredTasks.
	ListServices(ctx context.Context, labels map[string]string) ([]Service, error)
	InspectService(ctx context.Context, nameOrID string) (Service, error)
	RemoveService(ctx context.Context, nameOrID string) error
	ListTasks(ctx context.Context, f TaskFilter) ([]Task, error)
	// ServiceLogs es el log de todas las tareas de un servicio (GET …/logs, §10).
	ServiceLogs(ctx context.Context, nameOrID string, o LogOptions) (LogStream, error)
	// TaskLogs es el log de una tarea (las últimas 50 líneas de la que falló, §7 paso 6).
	TaskLogs(ctx context.Context, taskID string, o LogOptions) (LogStream, error)
}

// --- Redes ---------------------------------------------------------------------

// NetworkSpec es una red a crear. Las de stage son overlay y attachable (§3.2).
type NetworkSpec struct {
	Name       string
	Driver     string // "overlay" o "bridge"
	Attachable bool
	Internal   bool
	Labels     map[string]string
}

// Network es una red de Docker.
type Network struct {
	ID         string
	Name       string
	Driver     string
	Scope      string // "swarm" o "local"
	Attachable bool
	Internal   bool
	Labels     map[string]string
	// Subnets son los rangos de la red (para verificar que sean privados, §8).
	Subnets []string
	// Containers son los nombres de los contenedores del nodo local conectados.
	Containers []string
}

type Networks interface {
	// CreateNetwork devuelve el id de la red nueva; ErrConflict si ya existe una con ese nombre.
	CreateNetwork(ctx context.Context, spec NetworkSpec) (id string, err error)
	// InspectNetwork: ojo, una red de Swarm recién creada puede venir sin
	// Driver ni Subnets durante unos milisegundos (el allocator de Swarm
	// todavía no la procesó); Scope y Attachable ya vienen bien.
	InspectNetwork(ctx context.Context, nameOrID string) (Network, error)
	// ListNetworks devuelve las redes que tienen todas las etiquetas pedidas (nil: todas).
	ListNetworks(ctx context.Context, labels map[string]string) ([]Network, error)
	// RemoveNetwork: ErrNotFound si no existe, ErrConflict si algo sigue
	// conectado (un contenedor, o las tareas de un stack que se está borrando:
	// Swarm las suelta unos segundos después de docker stack rm).
	RemoveNetwork(ctx context.Context, nameOrID string) error
	// ConnectNetwork conecta un contenedor en caliente; ErrConflict si ya está conectado.
	ConnectNetwork(ctx context.Context, network, container string, aliases []string) error
	// DisconnectNetwork: ErrNotFound si el contenedor o la red no existen, o
	// no estaba conectado (una overlay a la que el nodo nunca se conectó
	// también da "red no existe").
	DisconnectNetwork(ctx context.Context, network, container string) error
}

// --- Contenedores (muelle-caddy) -------------------------------------------------

// PortBinding publica un puerto del contenedor en el host.
type PortBinding struct {
	HostIP        string // vacío: todas las interfaces
	HostPort      int
	ContainerPort int
	Protocol      string // "tcp" (por defecto) o "udp"
}

// Mount monta un volumen nombrado (los bind mounts no se usan).
type Mount struct {
	Volume   string
	Target   string
	ReadOnly bool
}

// ContainerSpec es un contenedor standalone a crear (§3.1).
type ContainerSpec struct {
	Name           string
	Image          string
	Cmd            []string
	Env            []string
	Labels         map[string]string
	Ports          []PortBinding
	Mounts         []Mount
	Network        string   // la red con la que se crea; las demás con ConnectNetwork
	NetworkAliases []string // en Network
	RestartPolicy  string   // "unless-stopped", "always"…; vacío: no se reinicia
}

// Container es un contenedor inspeccionado.
type Container struct {
	ID            string
	Name          string // sin la "/" inicial
	Image         string // la ref con la que se creó (Config.Image)
	ImageID       string
	Cmd           []string
	Labels        map[string]string
	Status        string // "created", "running", "exited"…
	Running       bool
	Health        string // "", "starting", "healthy" o "unhealthy"
	StartedAt     time.Time
	RestartPolicy string
	Ports         []PortBinding
	Mounts        []Mount
	Networks      []string // nombres de las redes conectadas, ordenados
}

// Stats es una muestra del consumo de un contenedor.
type Stats struct {
	CPUPercent       float64 // 100 = un core entero
	MemoryBytes      uint64  // sin la caché de páginas, como docker stats
	MemoryLimitBytes uint64  // el límite del contenedor, o la memoria del host si no tiene
}

type Containers interface {
	// CreateContainer devuelve el id del contenedor; ErrConflict si el nombre
	// está tomado, ErrNotFound si la imagen no está en el nodo (PullImage antes).
	CreateContainer(ctx context.Context, spec ContainerSpec) (id string, err error)
	InspectContainer(ctx context.Context, nameOrID string) (Container, error)
	StartContainer(ctx context.Context, nameOrID string) error
	// StopContainer espera hasta timeout antes de matarlo.
	StopContainer(ctx context.Context, nameOrID string, timeout time.Duration) error
	RemoveContainer(ctx context.Context, nameOrID string, force bool) error
	// ContainerStats toma una muestra (tarda ~1 s: compara con la anterior para el CPU).
	ContainerStats(ctx context.Context, nameOrID string) (Stats, error)
}

// --- Volúmenes --------------------------------------------------------------------

// Volume es un volumen de Docker. Los de un stack son <stack>_<nombre>.
type Volume struct {
	Name   string
	Driver string
	Labels map[string]string
}

type Volumes interface {
	ListVolumes(ctx context.Context, labels map[string]string) ([]Volume, error)
	// RemoveVolume: ErrNotFound si no existe, ErrConflict si está en uso.
	RemoveVolume(ctx context.Context, name string, force bool) error
}

// --- Imágenes y registries --------------------------------------------------------

// RegistryAuth son las credenciales de un registry (tabla registries). Host es
// el dominio de la imagen tal como lo da distribution/reference ("ghcr.io",
// "docker.io" para Docker Hub, "registry:5000").
type RegistryAuth struct {
	Host     string
	Username string
	Password string
}

// Image es una imagen del nodo local.
type Image struct {
	ID          string
	RepoTags    []string
	RepoDigests []string // ref@sha256:… de los registries de los que se bajó
}

type Images interface {
	// InspectImage busca la imagen en el nodo local, sin ir al registry:
	// ErrNotFound si no está (ingress la baja con PullImage antes de crear
	// muelle-caddy).
	InspectImage(ctx context.Context, ref string) (Image, error)
	// DistributionDigest consulta el registry (GET /distribution/{ref}/json,
	// §7 paso 2) y devuelve el digest del manifiesto ("sha256:…"). auth nil es
	// anónimo. ErrNotFound si la imagen o el tag no existen, ErrDenied si no hay acceso.
	DistributionDigest(ctx context.Context, ref string, auth *RegistryAuth) (digest string, err error)
	// PullImage baja una imagen al nodo local (la de muelle-caddy, antes de crearlo).
	PullImage(ctx context.Context, ref string, auth *RegistryAuth) error
}

// --- Secretos de Swarm (install) ------------------------------------------------

// Secret es un secreto de Swarm. Su valor nunca se puede volver a leer.
type Secret struct {
	ID        string
	Name      string
	Labels    map[string]string
	CreatedAt time.Time
}

type Secrets interface {
	// CreateSecret: ErrConflict si ya existe uno con ese nombre.
	CreateSecret(ctx context.Context, name string, data []byte, labels map[string]string) (id string, err error)
	InspectSecret(ctx context.Context, name string) (Secret, error)
	RemoveSecret(ctx context.Context, name string) error
}

// --- Stacks (CLI) ---------------------------------------------------------------

// StackDeploy es lo que recibe docker stack deploy.
type StackDeploy struct {
	Name string
	// Compose es el compose final (§6), que va por stdin y nunca se escribe a
	// disco. El CLI interpola ${VAR} con su propio entorno, que DeployStack
	// reduce a PATH, HOME y DOCKER_*: todo "$" literal tiene que venir como "$$".
	Compose []byte
	// Registries son las credenciales que van al DOCKER_CONFIG temporal, para
	// --with-registry-auth.
	Registries []RegistryAuth
}

type Stacks interface {
	// DeployStack ejecuta docker stack deploy --with-registry-auth --prune
	// --detach=true --resolve-image=never -c - <name> (§7 paso 5), con un
	// DOCKER_CONFIG temporal 0700 que se borra aunque el comando falle.
	// Devuelve la salida del comando (stdout y stderr, a lo más 64 KiB) para el
	// log del deploy; quien la registra enmascara los secretos. Si el comando
	// falla, el error envuelve ErrCommand.
	DeployStack(ctx context.Context, d StackDeploy) (output string, err error)
	// RemoveStack ejecuta docker stack rm <name>. No espera a que los servicios
	// terminen ni borra los volúmenes (los datos quedan).
	RemoveStack(ctx context.Context, name string) (output string, err error)
}
