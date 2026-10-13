// Package service tiene las reglas y los puertos del módulo deploy. Los
// puertos hacia otros módulos usan solo tipos de la biblioteca estándar y de
// shared: el contenedor conecta los módulos por tipado estructural (§4.1
// regla 1), así que una firma distinta no compila en container.go.
//
// La Fase 2 agrega los suyos (lectura de las apps de catalog por SQL, los
// deploys y registries, el render, las imágenes y la espera de convergencia);
// ver docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md.
package service

import "context"

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
