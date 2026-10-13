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
	// Remove ejecuta docker stack rm del stack (no espera a que los servicios
	// terminen ni borra los volúmenes). Un stack que no existe no es un error.
	// Docker caído o sin Swarm: un error que envuelve entity.ErrDockerUnavailable.
	Remove(ctx context.Context, stack string) error
}
