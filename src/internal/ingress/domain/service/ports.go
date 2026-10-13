// Package service tiene las reglas y los puertos del módulo ingress: la red
// de cada stage en Docker y los ajustes del proxy. La Fase 3 agrega los del
// contenedor muelle-caddy, la admin API de Caddy, Cloudflare y la
// verificación de certificados (ver docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md).
package service

import (
	"context"

	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// Network es una red de Docker como la ve ingress.
type Network struct {
	Name string
	// Overlay: red de Swarm (scope swarm, driver overlay). Recién creada, el
	// driver puede venir vacío unos milisegundos; el adaptador la cuenta igual.
	Overlay    bool
	Attachable bool
	// Containers son los contenedores del nodo local conectados.
	Containers []string
}

// Networks es Docker para las redes de stage (adaptador: infra/out/service,
// con shared/infra/out/docker). Todos los métodos son idempotentes: lo que ya
// está hecho no es un error. Un Docker caído devuelve un error que envuelve
// entity.ErrDockerUnavailable (503).
type Networks interface {
	// Inspect devuelve la red y si existe.
	Inspect(ctx context.Context, name string) (n Network, found bool, err error)
	// CreateOverlay crea una red overlay attachable con esas etiquetas. Si ya
	// existe (otra petición la creó a la vez) no es un error.
	CreateOverlay(ctx context.Context, name string, labels map[string]string) error
	// Remove borra la red; si no existe no es un error. inUse: no se borró
	// porque algo sigue conectado.
	Remove(ctx context.Context, name string) (inUse bool, err error)
	// Connect conecta un contenedor en caliente (docker network connect); si
	// ya estaba no es un error. found false: el contenedor no existe.
	Connect(ctx context.Context, network, container string) (found bool, err error)
	// Disconnect desconecta un contenedor; si no estaba conectado, o no
	// existe, no es un error.
	Disconnect(ctx context.Context, network, container string) error
}

// Settings lee los ajustes del proxy y el DNS que guarda admin en la tabla
// settings (adaptador SQL en infra/out/service, que descifra el token).
type Settings interface {
	ProxySettings(ctx context.Context) (entity.ProxySettings, error)
}
