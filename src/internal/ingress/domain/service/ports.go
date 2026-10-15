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
	// Labels son sus etiquetas (las de un stage: naming.StageLabels).
	Labels map[string]string
	// Containers son los contenedores del nodo local conectados. List no los
	// trae: solo Inspect.
	Containers []string
}

// Networks es Docker para las redes de stage (adaptador: infra/out/service,
// con shared/infra/out/docker). Todos los métodos son idempotentes: lo que ya
// está hecho no es un error. Un Docker caído devuelve un error que envuelve
// entity.ErrDockerUnavailable (503).
type Networks interface {
	// Inspect devuelve la red y si existe.
	Inspect(ctx context.Context, name string) (n Network, found bool, err error)
	// List devuelve todas las redes, con sus etiquetas y sin los contenedores.
	List(ctx context.Context) ([]Network, error)
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
// settings, y escribe las claves que son de ingress (contrato §3.6). El
// adaptador SQL (infra/out/service) descifra el token.
type Settings interface {
	ProxySettings(ctx context.Context) (entity.ProxySettings, error)
	PanelDNS(ctx context.Context) (PanelDNS, error)
	// SavePanelDNS escribe las claves no vacías y borra las vacías.
	SavePanelDNS(ctx context.Context, p PanelDNS) error
	// SaveZones guarda cloudflare_zones solo si el token guardado sigue siendo
	// token (en una transacción); saved dice si lo guardó.
	SaveZones(ctx context.Context, token string, zones int) (saved bool, err error)
}

// Zone es una zona de Cloudflare visible para el token.
type Zone struct{ ID, Name string }

// Record es un registro DNS de Cloudflare.
type Record struct {
	ID, Type, Name, Content string
	Proxied                 bool
}

// RecordChange es un cambio de un registro: los campos nil no cambian.
type RecordChange struct {
	Type, Content *string
	Proxied       *bool
}

// DNS es la API de Cloudflare (adaptador: infra/out/service/cloudflare). Cada
// método recibe el token: los ajustes pueden cambiar entre una llamada y otra.
// Errores: un error que envuelve entity.ErrCloudflareUnavailable (sin red, 5xx,
// 429, respuesta ilegible), ErrDNSNotFound (404) o un *DNSError (Cloudflare
// rechazó el pedido). Ninguno trae el token.
type DNS interface {
	// VerifyToken verifica el token (GET /user/tokens/verify). Uno rechazado,
	// o que no está activo (desactivado o vencido), es un *DNSError.
	VerifyToken(ctx context.Context, token string) error
	// Zones son todas las zonas que ve el token, de todas las páginas y en
	// cualquier estado.
	Zones(ctx context.Context, token string) ([]Zone, error)
	// Records son los registros de la zona con ese nombre exacto, de todos los tipos.
	Records(ctx context.Context, token, zoneID, name string) ([]Record, error)
	Record(ctx context.Context, token, zoneID, id string) (Record, error)
	// CreateRecord crea r con ttl automático y el comentario "muelle", y
	// devuelve el registro creado (con su id).
	CreateRecord(ctx context.Context, token, zoneID string, r Record) (Record, error)
	UpdateRecord(ctx context.Context, token, zoneID, id string, c RecordChange) (Record, error)
	DeleteRecord(ctx context.Context, token, zoneID, id string) error
}

// App es una app de catalog como la necesita ingress.
type App struct {
	ID                   int64
	Project, Stage, Slug string
	Kind                 string // KindService o KindCompose
}

// Los tipos de app (apps.kind).
const (
	KindService = "service"
	KindCompose = "compose"
)

// Stage es un stage de catalog.
type Stage struct{ Project, Stage string }

// Catalog lee apps, stages y projects (de catalog, §4.1 regla 1; adaptador
// SQL en infra/out/service).
type Catalog interface {
	// App es la app por sus slugs; repository.ErrNotFound si no existe.
	App(ctx context.Context, project, stage, app string) (App, error)
	// Apps son todas las apps, por id.
	Apps(ctx context.Context) (map[int64]App, error)
	// Stages son todos los stages, ordenados por proyecto y stage.
	Stages(ctx context.Context) ([]Stage, error)
}

// Credential es la credencial de un registry (tabla registries, de deploy).
// La contraseña va en claro: String, GoString, LogValue y MarshalJSON la
// ocultan (credential.go).
type Credential struct{ Host, Username, Password string }

// Registries lee las credenciales de los registries (de deploy, §4.1 regla 1;
// adaptador SQL en infra/out/service, que descifra la contraseña).
type Registries interface {
	// Credential devuelve nil, nil si no hay una para ese host.
	Credential(ctx context.Context, host string) (*Credential, error)
}

// PanelDNS son las claves panel_cf_* (contrato §3.6); "" = la clave no está.
type PanelDNS struct{ ZoneID, RecordID, RecordIDAAAA string }

// CaddyAdmin es la admin API de muelle-caddy (adaptador: caddy.Admin, por el
// socket unix de naming.CaddyAdminSocket).
type CaddyAdmin interface {
	// Load manda la configuración completa (POST /load); force agrega
	// Cache-Control: must-revalidate (recarga aunque sea igual). Errores:
	// ErrPlaceholderInSettings (no se manda nada), ErrCaddyUnreachable o un
	// *CaddyRejected. Ninguno trae el token.
	Load(ctx context.Context, cfg ProxyConfig, force bool) error
}

// CaddyWant es cómo tiene que quedar muelle-caddy (contrato §3.4).
type CaddyWant struct {
	Image               string
	HTTPPort, HTTPSPort int
	// Networks son las redes de stage a las que tiene que estar conectado.
	Networks []string
	// Auth es la credencial del registry de Image (nil: anónima). Su
	// contraseña nunca sale en un log, un error ni un JSON (credential.go).
	Auth *Credential
}

// CaddyRuntime es el contenedor muelle-caddy (adaptador: caddy.Runtime, D2
// del plan de la Fase 3).
type CaddyRuntime interface {
	// Ensure lo deja como pide want (contrato §3.4) y devuelve si lo creó de
	// nuevo. Errores: la imagen que no baja (ErrCaddyImagePull; el contenedor
	// anterior sigue intacto), ErrPortInUse (el contenedor queda borrado) o
	// Docker caído (entity.ErrDockerUnavailable).
	Ensure(ctx context.Context, want CaddyWant) (recreated bool, err error)
	// Status es lo que corre de verdad: la imagen con que se creó y si corre;
	// found false si el contenedor no existe.
	Status(ctx context.Context) (image string, running, found bool, err error)
}
