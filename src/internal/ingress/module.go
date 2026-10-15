// Package ingress es el módulo del proxy y el DNS (Fase 3): dominios,
// Cloudflare, el contenedor muelle-caddy y sus rutas, y la verificación de
// certificados (§3.1, §8 y §9 del spec). module.go es el único lugar que
// conoce todas sus piezas.
//
// Ya funcionan la red de cada stage (la usan catalog y deploy) y SyncRoutes,
// que manda la configuración a muelle-caddy por su admin API. Lo demás no
// hace nada todavía, como dice cada método, y las rutas no existen (404
// no_route). El contrato está en
// docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md.
package ingress

import (
	"context"
	"log/slog"

	"github.com/CPU-commits/muelle/src/cmd/cron"
	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/ingress/app/dto"
	"github.com/CPU-commits/muelle/src/internal/ingress/app/usecases"
	inhttp "github.com/CPU-commits/muelle/src/internal/ingress/infra/in/http"
	reposqlite "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/repository/sqlite"
	outservice "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/caddy"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/cipher"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

type Deps struct {
	DB     *sqlite.DB
	Cipher cipher.Cipher // el token de Cloudflare (settings:cloudflare_token)
	Clock  clock.Clock
	Audit  audit.Recorder
	Log    *slog.Logger
	Docker docker.Engine
	// Version es la de este binario (main.version): con caddy_image vacío,
	// muelle-caddy usa naming.CaddyImageFor(Version) (Fase 3).
	Version string
	// Options cambian los nombres de Docker, cómo se llega a Caddy y a
	// Cloudflare, y los plazos. El cero es producción; solo los tests las cambian.
	Options Options
}

type Module struct {
	ensureNetwork *usecases.EnsureStageNetwork
	removeNetwork *usecases.RemoveStageNetwork
	state         *usecases.ProxyState
	syncer        *usecases.Syncer
	log           *slog.Logger
}

// New arma el módulo. DB, Cipher y Docker son obligatorios; Clock, Log y
// Options, opcionales.
func New(d Deps) *Module {
	switch {
	case d.DB == nil:
		panic("ingress.New: falta Deps.DB")
	case d.Cipher == nil:
		panic("ingress.New: falta Deps.Cipher")
	case d.Docker == nil:
		panic("ingress.New: falta Deps.Docker")
	}
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	o := d.Options.withDefaults()
	nets := outservice.NewDockerNetworks(d.Docker)
	settings := outservice.NewProxySettingsReader(d.DB, d.Cipher)
	catalog := outservice.NewCatalogReader(d.DB)
	domains := reposqlite.NewDomains(d.DB)
	state := usecases.NewProxyState()
	// onFail: los reintentos con backoff llegan con el ciclo de vida de muelle-caddy.
	syncer := usecases.NewSyncer(settings, domains, catalog, caddy.NewAdmin(o.CaddyAdminDial), state, d.Clock,
		naming.CaddyImageFor(d.Version), o.InternalCA, func() {}, d.Log)
	return &Module{
		ensureNetwork: usecases.NewEnsureStageNetwork(nets, o.CaddyContainer, d.Log),
		removeNetwork: usecases.NewRemoveStageNetwork(nets, o.CaddyContainer, d.Log),
		state:         state,
		syncer:        syncer,
		log:           d.Log,
	}
}

// RegisterRoutes registra los errores del módulo. Fase 3: los dominios
// (GET/POST …/apps/{a}/domains, PATCH/DELETE …/domains/{hostname}, POST
// …/adopt y …/retry) y GET /api/v1/proxy, todos solo con sesión.
func (m *Module) RegisterRoutes(k *utils.Kit) { inhttp.RegisterErrors(k) }

// Jobs son las tareas periódicas. Fase 3: "proxy" cada 5 min (cron.Run lo
// corre también al arrancar: asegura muelle-caddy y las redes de todos los
// stages, borra las redes de stage huérfanas y reenvía la configuración,
// §3.1 y §7 "Reconciliación al arrancar") y "certificados" cada hora (§8).
func (m *Module) Jobs() []cron.Job { return nil }

// Run corre lo que ingress necesita en segundo plano hasta que ctx se cancela,
// y vuelve cuando terminó. Fase 3: los reintentos con backoff (1 s, 2 s… 60 s)
// de una configuración que Caddy no aceptó, la verificación de un dominio 30 s
// después de crearlo (§8) y la recreación de muelle-caddy que agenda
// ProxySettingsChanged. El esqueleto no tiene nada que correr.
func (m *Module) Run(ctx context.Context) {}

// EnsureStageNetwork asegura la red overlay attachable del stage
// (proyecto_stage, con las etiquetas de naming.StageLabels) y conecta
// muelle-caddy en caliente si existe (§3.2). Es idempotente. La llaman catalog
// al crear un stage y deploy antes de cada deploy (§7 paso 4). Errores: Docker
// caído (envuelve entity.ErrDockerUnavailable, 503) o una red con ese nombre
// que no es overlay attachable (409 network_conflict).
func (m *Module) EnsureStageNetwork(ctx context.Context, project, stage string) error {
	return m.ensureNetwork.Execute(ctx, dto.StageRef{Project: project, Stage: stage})
}

// RemoveStageNetwork desconecta muelle-caddy de la red del stage y la borra
// (§3.2). Una red que no existe no es un error; una que sigue en uso queda
// para la reconciliación (devuelve nil). La llama catalog al borrar un stage
// vacío. Error: Docker caído (503), y entonces el stage no se borra.
func (m *Module) RemoveStageNetwork(ctx context.Context, project, stage string) error {
	return m.removeNetwork.Execute(ctx, dto.StageRef{Project: project, Stage: stage})
}

// SyncRoutes regenera la configuración completa de Caddy desde la base y la
// envía por la admin API (§8), de a una. La llama deploy después de cada
// deploy (§7 paso 7). Con el proxy deshabilitado (desarrollo, contrato §3.4)
// devuelve nil sin mandar nada. Si Caddy no la acepta o no responde, devuelve
// un error sin el token, que deploy registra como advertencia: nunca hace
// fallar el deploy, el fallo queda en GET /api/v1/proxy e ingress reintenta
// por su cuenta.
func (m *Module) SyncRoutes(ctx context.Context) error { return m.syncer.Sync(ctx, false) }

// CheckCloudflareToken verifica un token nuevo antes de que admin lo guarde
// (PATCH /settings) y devuelve cuántas zonas ve. Fase 3: un token que
// Cloudflare rechaza, o que no ve ninguna zona, devuelve un *validate.Error en
// "cloudflare_token" (422); si Cloudflare no responde, un error que envuelve
// entity.ErrCloudflareUnavailable (502). El esqueleto no verifica: devuelve
// (nil, nil) y el token se guarda con las zonas desconocidas.
func (m *Module) CheckCloudflareToken(ctx context.Context, token string) (zones *int, err error) {
	return nil, nil
}

// ProxySettingsChanged aplica los ajustes nuevos después de que admin los
// guardó (PATCH /settings): Caddy (email ACME, IPs del panel, token) y los
// registros DNS que maneja muelle (IP pública) (§3.1, §9). Corre antes de que
// el PATCH responda, con su propio plazo (no el ctx de la petición: un cliente
// que se va no puede dejar el cambio a medias). Si cambiaron los puertos o la
// imagen, no recrea muelle-caddy acá: lo agenda en Run, 2 s después de volver,
// porque la respuesta del PATCH sale por ese mismo contenedor (contrato
// §3.5). No devuelve error: un fallo queda en el estado del proxy (GET
// /api/v1/proxy) y en el de cada dominio. Fase 3; el esqueleto no hace nada.
func (m *Module) ProxySettingsChanged(ctx context.Context, old, next entity.ProxySettings) {
	m.log.Debug("ingress: los ajustes del proxy cambiaron; se aplican con la Fase 3", "claves", next.ChangedKeys(old))
}
