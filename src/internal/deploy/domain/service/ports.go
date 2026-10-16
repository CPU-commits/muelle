// Package service tiene las reglas y los puertos del módulo deploy. Los
// puertos hacia otros módulos usan solo tipos de la biblioteca estándar y de
// shared: el contenedor conecta los módulos por tipado estructural (§4.1
// regla 1), así que una firma distinta no compila en container.go.
//
// Las apps de catalog y los dominios de ingress se leen por SQL (Apps y
// DomainTargets, adaptadores en infra/out/service). La Fase 2 agrega los suyos
// (los deploys y registries, el render, las imágenes y la espera de
// convergencia); ver docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md.
package service

import (
	"context"
	"time"
)

// Los tipos de app (apps.kind).
const (
	KindService = "service" // app de formulario: Spec
	KindCompose = "compose" // app compose: ComposeYAML
)

// App es una app como la necesita el deploy: la fila de apps (de catalog) con
// los slugs de su stage y su proyecto.
type App struct {
	ID      int64
	Project string
	Stage   string
	Slug    string
	Kind    string // KindService o KindCompose
	// Spec es el JSON de apps.spec (§6.1), ya validado por catalog; nil en una app compose.
	Spec []byte
	// ComposeYAML es apps.compose_yaml; "" en una app de formulario.
	ComposeYAML    string
	DeployTimeoutS int
	// StorageNodeID es el nodo donde viven sus volúmenes; "" hasta el primer
	// deploy con volúmenes (§6.2).
	StorageNodeID   string
	ConfigChangedAt time.Time
	LastDeployedAt  *time.Time
}

// Apps lee y marca las apps de catalog (tabla apps, §4.1 regla 1). deploy solo
// escribe storage_node_id y last_deployed_at; el resto es de catalog.
type Apps interface {
	// Get y GetByID: repository.ErrNotFound si la app no existe.
	Get(ctx context.Context, project, stage, app string) (App, error)
	GetByID(ctx context.Context, id int64) (App, error)
	// SetStorageNode guarda storage_node_id solo si está vacío y devuelve el
	// que quedó: el del primer deploy con volúmenes gana, aunque dos corran a la vez.
	SetStorageNode(ctx context.Context, appID int64, nodeID string) (string, error)
	// MarkDeployed adelanta last_deployed_at hasta at, que es cuando el deploy
	// leyó la configuración de la app (su started_at), no cuando terminó: un
	// cambio guardado mientras el deploy corría sigue como "cambios
	// pendientes" (§5.2 regla 4). Nunca la atrasa.
	MarkDeployed(ctx context.Context, appID int64, at time.Time) error
}

// DomainTarget es el destino de un dominio de la app (tabla domains, de ingress).
type DomainTarget struct {
	Hostname string
	Service  string // el nombre corto del servicio en el compose
	Port     int
}

// DomainTargets son los destinos de los dominios de una app, que el render
// valida contra el compose (§6.3). Se leen por SQL: así deploy no depende de
// que ingress esté armado.
type DomainTargets interface {
	// ByApp los devuelve ordenados por hostname; [] si no tiene.
	ByApp(ctx context.Context, appID int64) ([]DomainTarget, error)
}

// EnvResolver son las variables que recibe una app al desplegarse. Lo
// implementa secrets.Module.ResolveEnv: vars son las del stage con las de la
// app encima, y secretValues los valores de las secretas de los dos, sin
// repetidos ni vacíos y de los más largos a los más cortos, que el log del
// deploy reemplaza por *** (§7). repository.ErrNotFound si la app no existe.
type EnvResolver interface {
	ResolveEnv(ctx context.Context, appID int64) (vars map[string]string, secretValues []string, err error)
}

// Ingress es lo que deploy le pide al proxy. Lo implementa ingress.Module.
type Ingress interface {
	// EnsureStageNetwork asegura la red del stage y que muelle-caddy esté
	// conectado (§7 paso 4). Si falla, el deploy falla y Swarm no se toca.
	EnsureStageNetwork(ctx context.Context, project, stage string) error
	// SyncRoutes regenera las rutas de Caddy (§7 paso 7). Un error es una
	// advertencia en el log del deploy, nunca lo hace fallar (§13).
	SyncRoutes(ctx context.Context) error
}

// Stacks es Docker para los stacks de las apps (adaptador en
// infra/out/service, con shared/infra/out/docker).
type Stacks interface {
	// Deploy ejecuta docker stack deploy del compose final (§7 paso 5), con
	// las credenciales de creds para --with-registry-auth, y devuelve su
	// salida para el log, sin enmascarar: quien la escribe la enmascara. Si
	// el comando falla, también devuelve la salida.
	Deploy(ctx context.Context, stack string, compose []byte, creds []Credential) (string, error)
	// Remove ejecuta docker stack rm del stack (no espera a que los servicios
	// terminen ni borra los volúmenes). Un stack que no existe no es un error.
	// Docker caído o sin Swarm: un error que envuelve entity.ErrDockerUnavailable.
	Remove(ctx context.Context, stack string) error
}

// Images resuelve las imágenes en su registry (§7 paso 2).
type Images interface {
	// Digest consulta el registry por ref (también si ya trae un digest: así
	// verifica que exista y que la credencial llegue) y devuelve el digest de
	// su manifiesto ("sha256:…"). cred nil es anónimo. ErrImageNotFound,
	// ErrImageDenied o un error que envuelve entity.ErrDockerUnavailable.
	Digest(ctx context.Context, ref string, cred *Credential) (string, error)
}

// Cluster es lo que deploy lee de Swarm: los servicios de una app, sus
// tareas y sus logs, y el nodo local (adaptador en infra/out/service). Con
// Docker caído o sin Swarm, un error que envuelve entity.ErrDockerUnavailable.
type Cluster interface {
	// AppServices son los servicios con las etiquetas de la app
	// (naming.AppLabels), ordenados por nombre.
	AppServices(ctx context.Context, project, stage, app string) ([]SwarmService, error)
	// Service es el servicio de ese nombre de Swarm; ErrSwarmNotFound si no está.
	Service(ctx context.Context, name string) (SwarmService, error)
	// Tasks son las tareas del servicio (también las terminadas), de la más
	// nueva a la más vieja.
	Tasks(ctx context.Context, serviceID string) ([]SwarmTask, error)
	// TaskLogs son las últimas tail líneas del log de una tarea, sin seguirlo;
	// los pedazos de una línea larga vienen tal cual (LogLine).
	// ErrSwarmNotFound si la tarea no existe.
	TaskLogs(ctx context.Context, taskID string, tail int) ([]LogLine, error)
	// LocalNodeID es el id del nodo local en el Swarm (Info().NodeID).
	LocalNodeID(ctx context.Context) (string, error)
	// ServiceLogs es el log de todas las tareas del servicio serviceID (GET
	// …/logs, contrato §2.9), con los pedazos de una línea larga tal cual
	// (LogLine). Hay que cerrarlo siempre. ErrSwarmNotFound si el servicio no
	// está.
	ServiceLogs(ctx context.Context, serviceID string, o LogOptions) (LogStream, error)
	// NodeHostname es el hostname de un nodo del Swarm (GET …/status);
	// ErrSwarmNotFound si el nodo ya no está.
	NodeHostname(ctx context.Context, nodeID string) (string, error)
	// ContainerHealth es el health de un contenedor del nodo local:
	// "healthy", "unhealthy", "starting", o "" si no tiene healthcheck.
	// ErrSwarmNotFound si no está en este nodo.
	ContainerHealth(ctx context.Context, containerID string) (string, error)
	// ContainerStats es una muestra del consumo de un contenedor del nodo
	// local (tarda cerca de 1 s). ErrSwarmNotFound si no está en este nodo.
	ContainerStats(ctx context.Context, containerID string) (Stats, error)
}

// LogOptions elige las líneas de ServiceLogs: las últimas Tail (con Since,
// las últimas Tail desde esa hora, inclusive) y, con Follow, las que siguen.
type LogOptions struct {
	Tail   int
	Follow bool
	Since  time.Time // cero: sin límite
}

// LogStream entrega las líneas de un log en orden. Next devuelve io.EOF al
// terminar (sin Follow, o con Follow cuando el servicio se borra); con
// Follow, termina también al cancelar el ctx o con Close. Close se puede
// llamar desde otra goroutine mientras Next espera.
type LogStream interface {
	Next() (LogLine, error)
	Close() error
}

// Stats es una muestra del consumo de un contenedor.
type Stats struct {
	CPUPercent  float64 // el % de un núcleo, como docker stats
	MemoryBytes uint64  // sin la caché de páginas
}

// ManagerNode es settings.manager_node_id (contrato §1.1 y §1.2; la clave es
// entity.SettingManagerNodeID de shared/domain/entity): el nodo donde quedan
// los volúmenes de una app si no tiene storage_node_id (§6.2).
type ManagerNode interface {
	// Get es el id guardado, o "" si no está.
	Get(ctx context.Context) (string, error)
	// SetIfMissing guarda id si no había uno y devuelve el que quedó: nunca
	// pisa el que ya estaba.
	SetIfMissing(ctx context.Context, id string) (string, error)
}

// Queue es la cola de deploys: un worker por app (§7 paso 1). La implementa
// el despachador (usecases.Dispatcher).
type Queue interface {
	// Wake avisa que la app tiene un deploy queued nuevo.
	Wake(appID int64)
	// Accepting es false mientras la app se está borrando (contrato §1.1):
	// un deploy o un rollback nuevo es 409 conflict.
	Accepting(appID int64) bool
}

// Marks son marcas de una sola vez en la tabla settings (de admin, §4.1
// regla 1): deploy guarda init_registry_applied (contrato §1.1).
type Marks interface {
	// MarkOnce guarda la marca key si no estaba y dice si la guardó. Usa la
	// transacción del ctx.
	MarkOnce(ctx context.Context, key string) (bool, error)
}

// Estados de una actualización de un servicio de Swarm (UpdateStatus.State,
// §7 paso 6): los mismos de shared/infra/out/docker, que el adaptador copia tal cual.
const (
	UpdateUpdating          = "updating"
	UpdatePaused            = "paused"
	UpdateCompleted         = "completed"
	UpdateRollbackStarted   = "rollback_started"
	UpdateRollbackPaused    = "rollback_paused"
	UpdateRollbackCompleted = "rollback_completed"
)

// SwarmService es un servicio de Swarm como lo lee deploy.
type SwarmService struct {
	ID, Name         string
	Labels           map[string]string // deploy.labels (muelle.deploy, …)
	Image            string
	Replicas         *uint64 // nil: global
	RunningTasks     uint64
	DesiredTasks     uint64
	MemoryLimitBytes int64
	Update           *UpdateStatus // nil: nunca se actualizó (o Swarm la reinició)
	CreatedAt        time.Time
}

// UpdateStatus es el estado de la última actualización de un servicio. Queda
// el de la actualización anterior hasta que empieza la siguiente: quien espera
// un deploy mira StartedAt (Evaluate).
type UpdateStatus struct {
	State, Message string // Update*
	StartedAt      *time.Time
}

// SwarmTask es una tarea de Swarm (una réplica en un momento dado).
type SwarmTask struct {
	ID, ServiceID, NodeID    string
	Slot                     int // 0 en un servicio global
	State, DesiredState, Err string
	ContainerID              string
	CreatedAt, UpdatedAt     time.Time
}

// LogLine es una línea de log de una tarea; Time cero y TaskID vacío en el
// pedazo de una línea larga (contrato §2.5).
type LogLine struct {
	Time           time.Time
	Stream, TaskID string
	Text           string
}
