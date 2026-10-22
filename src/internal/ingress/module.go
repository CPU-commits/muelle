// Package ingress es el módulo del proxy y el DNS (Fase 3): dominios,
// Cloudflare, el contenedor muelle-caddy y sus rutas, y la verificación de
// certificados (§3.1, §8 y §9 del spec). module.go es el único lugar que
// conoce todas sus piezas.
//
// Tiene la red de cada stage (la usan catalog y deploy), SyncRoutes, que
// manda la configuración a muelle-caddy por su admin API, el ciclo de vida
// de muelle-caddy (el job "proxy", los reintentos y la recreación), GET
// /api/v1/proxy, los dominios con su DNS en Cloudflare (…/domains), el
// registro DNS del panel, la verificación de los certificados, los
// certificados de Cloudflare (Origin CA, opcionales: emitirlos, renovarlos y
// revocarlos) y los hooks de admin (el token de Cloudflare, el permiso de
// Origin CA y los ajustes del proxy). El contrato está en
// docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md.
package ingress

import (
	"context"
	"log/slog"
	"sync"

	"github.com/CPU-commits/muelle/src/cmd/cron"
	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/cmd/mcp/tools"
	"github.com/CPU-commits/muelle/src/internal/ingress/app/dto"
	"github.com/CPU-commits/muelle/src/internal/ingress/app/usecases"
	incron "github.com/CPU-commits/muelle/src/internal/ingress/infra/in/cron"
	inhttp "github.com/CPU-commits/muelle/src/internal/ingress/infra/in/http"
	ingmcp "github.com/CPU-commits/muelle/src/internal/ingress/infra/in/mcp"
	reposqlite "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/repository/sqlite"
	outservice "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/caddy"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare"
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
	// PortsRolledBack, opcional, se llama cuando ingress vuelve los puertos
	// del proxy a los anteriores porque muelle-caddy no arrancó con los
	// nuevos (contrato §3.5). El contenedor invalida ahí el origen del panel
	// en caché de admin.
	PortsRolledBack func()
}

type Module struct {
	ensureNetwork *usecases.EnsureStageNetwork
	removeNetwork *usecases.RemoveStageNetwork
	state         *usecases.ProxyState
	syncer        *usecases.Syncer
	caddy         *usecases.EnsureCaddy
	orphans       *usecases.RemoveOrphanNetworks
	proxyJob      *usecases.ProxyJob
	retrier       *usecases.Retrier
	recreator     *usecases.Recreator
	ops           *usecases.Ops
	panelDNS      *usecases.EnsurePanelDNS
	checkToken    *usecases.CheckCloudflareToken
	checkOrigin   *usecases.CheckOriginCAPermission
	checkPorts    *usecases.CheckProxyPorts
	settingsHook  *usecases.ProxySettingsChanged
	certs         *usecases.CertChecker
	origin        *usecases.EnsureOriginCA
	jobs          []cron.Job
	httpUC        inhttp.UseCases
}

// New arma el módulo. DB, Cipher, Docker y Audit (los dominios auditan cada
// cambio) son obligatorios; Clock, Log y Options, opcionales.
func New(d Deps) *Module {
	switch {
	case d.DB == nil:
		panic("ingress.New: falta Deps.DB")
	case d.Cipher == nil:
		panic("ingress.New: falta Deps.Cipher")
	case d.Docker == nil:
		panic("ingress.New: falta Deps.Docker")
	case d.Audit == nil:
		panic("ingress.New: falta Deps.Audit")
	}
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	o := d.Options.withDefaults()
	defImage := naming.CaddyImageFor(d.Version)
	nets := outservice.NewDockerNetworks(d.Docker)
	settings := outservice.NewProxySettingsReader(d.DB, d.Cipher)
	catalog := outservice.NewCatalogReader(d.DB)
	registries := outservice.NewRegistriesReader(d.DB, d.Cipher)
	domains := reposqlite.NewDomains(d.DB)
	origin := usecases.OriginRepos{Certs: reposqlite.NewOriginCerts(d.DB, d.Cipher), Hosts: reposqlite.NewOriginHosts(d.DB)}
	runtime := caddy.NewRuntime(d.Docker, caddy.Names{
		Container: o.CaddyContainer, PanelNetwork: o.PanelNetwork,
		DataVolume: o.CaddyDataVolume, ConfigVolume: o.CaddyConfigVolume, AdminVolume: o.CaddyAdminVolume,
	}, d.Log)

	m := &Module{state: usecases.NewProxyState()}
	// Un Sync que falla despierta al Retrier, que reintenta con backoff.
	m.syncer = usecases.NewSyncer(settings, domains, catalog, origin, caddy.NewAdmin(o.CaddyAdminDial), m.state, d.Clock,
		defImage, o.InternalCA, func() { m.retrier.Kick() }, d.Log)
	m.retrier = usecases.NewRetrier(func(ctx context.Context) error { return m.syncer.Sync(ctx, false) },
		o.Timing.RetryMin, o.Timing.RetryMax)
	m.ensureNetwork = usecases.NewEnsureStageNetwork(nets, o.CaddyContainer, d.Log)
	m.removeNetwork = usecases.NewRemoveStageNetwork(nets, o.CaddyContainer, d.Log)
	m.caddy = usecases.NewEnsureCaddy(runtime, settings, catalog, registries, m.state, defImage,
		usecases.PortsRollbackDeps{Audit: d.Audit, Clock: d.Clock, Done: d.PortsRolledBack}, d.Log)
	m.orphans = usecases.NewRemoveOrphanNetworks(nets, catalog, o.CaddyContainer, o.NetworkPrefix, d.Log)
	m.ops = &usecases.Ops{}
	dns := cloudflare.New(o.CloudflareURL)
	m.panelDNS = usecases.NewEnsurePanelDNS(settings, dns, m.ops, o.Timing.DomainOp, d.Log)
	m.origin = usecases.NewEnsureOriginCA(usecases.OriginDeps{
		Settings: settings, Domains: domains, Repos: origin, DNS: dns, CA: dns, Keys: outservice.OriginKeys{},
		Syncer: m.syncer, Clock: d.Clock, DefImage: defImage, Pass: o.Timing.OriginPass, Grace: o.Timing.OriginRevokeGrace, Log: d.Log,
	})
	m.proxyJob = usecases.NewProxyJob(settings, catalog, m.ensureNetwork, m.caddy, m.orphans, m.syncer, m.panelDNS, m.origin,
		m.state, defImage, o.Timing.RetryMin, o.Timing.RetryMax, d.Log)
	m.recreator = usecases.NewRecreator(m.caddy, m.syncer, m.state, o.Timing.RecreateDelay, o.Timing.RetryMin, o.Timing.RetryMax, d.Log)
	probe := outservice.TLSProbe{Addr: o.CaddyTLSAddr, Roots: o.TLSRoots, Timeout: outservice.DefaultProbeTimeout}
	m.certs = usecases.NewCertChecker(probe, domains, settings, origin, m.syncer, m.state, m.ops, d.Clock, o.Timing.FirstCheck, o.Timing.PendingEvery,
		o.Timing.DomainOp, d.Log)
	m.jobs = incron.Jobs(m.proxyJob.Run, func(ctx context.Context) error { return m.certs.CheckAll(ctx, usecases.EligibleHourly) })
	dd := usecases.DomainDeps{
		Repo: domains, Catalog: catalog, Settings: settings, DNS: dns, Syncer: m.syncer,
		Certs: m.certs, Origin: m.origin, Ops: m.ops, Clock: d.Clock, Audit: d.Audit, Log: d.Log,
		DomainOp: o.Timing.DomainOp, DomainRetry: o.Timing.DomainRetry,
	}
	m.checkToken = usecases.NewCheckCloudflareToken(dns)
	m.checkOrigin = usecases.NewCheckOriginCAPermission(dns, dns, outservice.OriginKeys{}, origin.Certs, d.Clock, d.Log)
	m.checkPorts = usecases.NewCheckProxyPorts(runtime, defImage, d.Log)
	m.settingsHook = usecases.NewProxySettingsChanged(m.recreator, usecases.NewUpdateManagedRecords(dd), m.panelDNS, m.syncer,
		m.origin, m.state, o.Timing.Hook, d.Log)
	m.httpUC = inhttp.UseCases{
		ProxyStatus: usecases.NewGetProxyStatus(settings, runtime, m.state, m.origin, d.Clock, defImage),
		ListDomains: usecases.NewListDomains(dd), CreateDomain: usecases.NewCreateDomain(dd),
		UpdateDomain: usecases.NewUpdateDomain(dd), DeleteDomain: usecases.NewDeleteDomain(dd),
		AdoptDomain: usecases.NewAdoptDomain(dd), RetryDomain: usecases.NewRetryDomain(dd),
	}
	return m
}

// RegisterRoutes registra los errores y las rutas del módulo (contrato §3):
// GET /api/v1/proxy y los dominios (GET/POST …/apps/{a}/domains, PATCH/DELETE
// …/domains/{hostname}, POST …/adopt y …/retry), solo con sesión, salvo GET
// …/apps/{a}/domains, que también lee un token en su alcance (§8.1).
func (m *Module) RegisterRoutes(k *utils.Kit) { inhttp.Register(k, m.httpUC) }

// RegisterTools registra la herramienta MCP de ingress (contrato §8.3):
// list_domains.
func (m *Module) RegisterTools(k *tools.Kit) {
	ingmcp.Register(k, ingmcp.UseCases{ListDomains: m.httpUC.ListDomains})
}

// Jobs son las tareas periódicas: "proxy" cada 5 min (cron.Run lo corre
// también al arrancar: asegura muelle-caddy y las redes de todos los
// stages, borra las redes de stage huérfanas, reenvía la configuración y,
// con token, actualiza cloudflare_zones y el registro DNS del panel, §3.1 y
// §7 "Reconciliación al arrancar") y "certificados" cada hora, que verifica
// los dominios pending, active y error + certificate (§8).
func (m *Module) Jobs() []cron.Job { return m.jobs }

// Run corre lo que ingress necesita en segundo plano hasta que ctx se cancela,
// y vuelve cuando terminó: los reintentos con backoff (1 s, 2 s… 60 s) de una
// configuración que Caddy no aceptó, la recreación de muelle-caddy que
// agenda ProxySettingsChanged (si no arranca con los puertos nuevos, vuelve
// a los anteriores: contrato §3.5), la verificación de un dominio 30 s
// después de crearlo, adoptarlo o reintentarlo, la de los pending cada
// minuto (§8) y la reconciliación de los certificados de Cloudflare (Origin
// CA, contrato §3.7): una pasada a la vez, cada vez que la despierta el job
// "proxy", un PATCH /settings o un cambio de un dominio.
func (m *Module) Run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Go(func() { m.retrier.Run(ctx) })
	wg.Go(func() { m.recreator.Run(ctx) })
	wg.Go(func() { m.certs.Run(ctx) })
	wg.Go(func() { m.origin.Run(ctx) })
	wg.Wait()
}

// scheduleRecreate agenda la recreación de muelle-caddy (la corre Run). La
// usan los tests; ProxySettingsChanged usa recreator.Hold.
func (m *Module) scheduleRecreate() { m.recreator.Schedule() }

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
// (PATCH /settings, contrato §3.5) y devuelve cuántas zonas ve, con un plazo
// de 30 s sobre el ctx de la petición. Un token que Cloudflare rechaza (y que
// tampoco lista zonas), o que no puede listar zonas o no ve ninguna, devuelve
// un *validate.Error en "cloudflare_token" (422); si Cloudflare no responde,
// un error que envuelve entity.ErrCloudflareUnavailable (502). Un token de
// una cuenta, que /user/tokens/verify rechaza pero que lista zonas, se acepta.
func (m *Module) CheckCloudflareToken(ctx context.Context, token string) (zones *int, err error) {
	return m.checkToken.Execute(ctx, token)
}

// CheckCloudflareOriginCA comprueba, antes de que admin guarde un PATCH
// /settings que enciende los certificados de Cloudflare o cambia el token
// con la opción encendida, que el token tenga Zone → SSL and Certificates →
// Edit en las zonas activas que ve (contrato §3.5), con un plazo de 30 s
// sobre el ctx de la petición. field es el campo de los 422
// (cloudflare_origin_ca o cloudflare_token). Sin el permiso en alguna zona,
// un token que no lista zonas o no ve ninguna, o una prueba que Cloudflare
// rechazó, devuelve un *validate.Error en field (422); si Cloudflare no
// responde, un error que envuelve entity.ErrCloudflareUnavailable (502).
// Ninguno trae el token.
func (m *Module) CheckCloudflareOriginCA(ctx context.Context, token, field string) error {
	return m.checkOrigin.Execute(ctx, token, field)
}

// CheckProxyPorts prueba, antes de que admin guarde otros puertos del proxy
// (PATCH /settings, contrato §4.1), que muelle-caddy los pueda tomar en el
// host, con un contenedor descartable y un plazo de 30 s sobre el ctx de la
// petición. Prueba solo los que cambian y que el muelle-caddy que sirve no
// tiene ya. Uno ocupado devuelve un *validate.Error en proxy_http_port o
// proxy_https_port (422); Docker caído, un error que envuelve
// entity.ErrDockerUnavailable (503). Con el proxy deshabilitado (desarrollo)
// no toca Docker.
func (m *Module) CheckProxyPorts(ctx context.Context, old, next entity.ProxySettings) error {
	return m.checkPorts.Execute(ctx, old, next)
}

// ProxySettingsChanged aplica los ajustes nuevos después de que admin los
// guardó (PATCH /settings): Caddy (email ACME, IPs del panel, token) y los
// registros DNS que maneja muelle (IP pública) (§3.1, §9). Corre antes de que
// el PATCH responda, con su propio plazo (no el ctx de la petición: un cliente
// que se va no puede dejar el cambio a medias). Si cambiaron los puertos, no
// recrea muelle-caddy acá: lo agenda en Run, 2 s después de volver, porque la
// respuesta del PATCH sale por ese mismo contenedor (contrato §3.5); si no
// arranca con los puertos nuevos, Run vuelve a los anteriores y llama a
// Deps.PortsRolledBack. No devuelve error: un fallo queda en el estado del
// proxy (GET /api/v1/proxy) y en el de cada dominio. Un token nuevo, o la IP
// pública, también llevan el registro DNS del panel (§3.4).
func (m *Module) ProxySettingsChanged(ctx context.Context, old, next entity.ProxySettings) {
	m.settingsHook.Execute(ctx, old, next)
}
