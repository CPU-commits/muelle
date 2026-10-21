package service

import "context"

// StageNetworks es la red overlay de cada stage y la conexión de muelle-caddy
// (§3.2). La implementa ingress.Module; el contenedor la conecta. Los dos
// métodos son idempotentes. Docker caído: un error que envuelve
// entity.ErrDockerUnavailable (503).
type StageNetworks interface {
	EnsureStageNetwork(ctx context.Context, project, stage string) error
	RemoveStageNetwork(ctx context.Context, project, stage string) error
}

// AppRuntime es lo que una app tiene en Swarm. La implementa deploy.Module.
type AppRuntime interface {
	// RemoveApp ejecuta docker stack rm del stack de la app (y, desde la Fase
	// 2, detiene su worker). Un stack que no existe no es un error.
	RemoveApp(ctx context.Context, project, stage, app string) error
}

// AppDomains cuenta los dominios de una app (tabla domains, de ingress; el
// adaptador SQL está en infra/out/service).
type AppDomains interface {
	CountByApp(ctx context.Context, appID int64) (int, error)
}
