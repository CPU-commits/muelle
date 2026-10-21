// Package deploy es el módulo de los deploys (Fase 2): render del compose,
// cola por app, resolución de digests, docker stack deploy, convergencia, log,
// rollback, reconciliación al arrancar, estado y logs en vivo de las apps, y
// los registries con sus credenciales (§5.1, §6, §7 y §10 del spec). module.go
// es el único lugar que conoce todas sus piezas.
//
// Tiene: los registries (CRUD y el registry inicial de install); encolar y
// leer deploys y rollbacks por HTTP, con el log por polling y en vivo (SSE);
// el worker (una cola por app que corre los pasos de §7, con la
// reconciliación al arrancar); el estado de las apps en Swarm y los logs de
// sus servicios en vivo; y RemoveApp, que detiene el worker de la app y hace
// docker stack rm (lo usa catalog al borrar una app). El contrato está en
// docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md.
package deploy

import (
	"context"
	"log/slog"

	"github.com/CPU-commits/muelle/src/cmd/cron"
	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/app/usecases"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	dephttp "github.com/CPU-commits/muelle/src/internal/deploy/infra/in/http"
	depsqlite "github.com/CPU-commits/muelle/src/internal/deploy/infra/out/repository/sqlite"
	outservice "github.com/CPU-commits/muelle/src/internal/deploy/infra/out/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/cipher"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

type Deps struct {
	DB     *sqlite.DB
	Cipher cipher.Cipher // contraseñas de los registries (registries:password:<id>)
	Clock  clock.Clock
	Audit  audit.Recorder
	Log    *slog.Logger
	Docker docker.Engine
	// Env son las variables de la app (secrets.Module).
	Env service.EnvResolver
	// Ingress es la red del stage y las rutas de Caddy (ingress.Module).
	Ingress service.Ingress
	// InitRegistry es el registry inicial de install
	// (MUELLE_INIT_REGISTRY_FILE). Opcional: nil es "no hay nada que sembrar".
	InitRegistry *InitRegistry
}

// InitRegistry es la credencial de un registry que install le pasa a muelle
// (contrato §2.10). Se imprime sin la contraseña.
type InitRegistry = dto.InitRegistry

type Module struct {
	removeApp    *usecases.RemoveApp
	seed         *usecases.SeedInitRegistry
	initRegistry *InitRegistry
	uc           dephttp.UseCases
	log          *slog.Logger

	deploys      *depsqlite.DeployRepository
	dispatcher   *usecases.Dispatcher
	storeManager *usecases.StoreManagerNode

	// Los streams (contrato §0.3): el Hub que comparten el worker, los casos
	// de uso y los streams; un ctx que Run cancela al apagar (así
	// srv.Shutdown no espera a los streams abiertos); y sus límites, que los
	// tests acortan antes de RegisterRoutes.
	hub         *usecases.Hub
	streamCtx   context.Context
	stopStreams context.CancelFunc
	sse         dephttp.SSEConfig
}

// New arma el módulo. DB, Cipher, Audit, Docker, Env e Ingress son
// obligatorios; Clock y Log, opcionales.
func New(d Deps) *Module {
	switch {
	case d.DB == nil:
		panic("deploy.New: falta Deps.DB")
	case d.Cipher == nil:
		panic("deploy.New: falta Deps.Cipher")
	case d.Audit == nil:
		panic("deploy.New: falta Deps.Audit")
	case d.Docker == nil:
		panic("deploy.New: falta Deps.Docker")
	case d.Env == nil:
		panic("deploy.New: falta Deps.Env")
	case d.Ingress == nil:
		panic("deploy.New: falta Deps.Ingress")
	}
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	regs := depsqlite.NewRegistryRepository(d.DB, d.Cipher)
	deploys := depsqlite.NewDeployRepository(d.DB)
	apps := outservice.NewCatalogApps(d.DB)
	hub := usecases.NewHub()
	settings := outservice.NewDeploySettings(d.DB)
	dk := outservice.NewDocker(d.Docker)
	stacks := outservice.NewDockerStacks(d.Docker)
	// El worker (§7). El despachador se arma después y queda como su Gate.
	run := &usecases.RunDeploy{
		Apps: apps, Deploys: deploys, Registries: regs, Env: d.Env,
		Domains: outservice.NewDomainTargets(d.DB), Ingress: d.Ingress,
		Images: dk, Stacks: stacks, Cluster: dk, Manager: settings,
		Log: usecases.NewLogWriter(deploys, hub, d.Clock), Hub: hub, Tx: d.DB,
		Clock: d.Clock, Timing: usecases.DefaultTiming(),
	}
	dispatcher := usecases.NewDispatcher(run, d.Log)
	ud := usecases.Deps{
		Tx:      d.DB,
		Apps:    apps,
		Deploys: deploys,
		Queue:   dispatcher,
		Hub:     hub,
		Clock:   d.Clock,
		Audit:   d.Audit,
	}
	streamCtx, stopStreams := context.WithCancel(context.Background())
	return &Module{
		hub:          hub,
		streamCtx:    streamCtx,
		stopStreams:  stopStreams,
		sse:          dephttp.DefaultSSEConfig(),
		deploys:      deploys,
		dispatcher:   dispatcher,
		storeManager: usecases.NewStoreManagerNode(dk, settings, d.Log),
		removeApp:    usecases.NewRemoveApp(apps, deploys, stacks, dispatcher, hub, d.Clock),
		seed:         usecases.NewSeedInitRegistry(d.DB, settings, regs, d.Clock, d.Audit),
		initRegistry: d.InitRegistry,
		uc: dephttp.UseCases{
			ListRegistries: usecases.NewListRegistries(regs),
			CreateRegistry: usecases.NewCreateRegistry(d.DB, regs, d.Clock, d.Audit),
			UpdateRegistry: usecases.NewUpdateRegistry(d.DB, regs, d.Clock, d.Audit),
			DeleteRegistry: usecases.NewDeleteRegistry(d.DB, regs, d.Audit),

			CreateDeploy: usecases.NewCreateDeploy(ud),
			Rollback:     usecases.NewRollback(ud),
			ListDeploys:  usecases.NewListDeploys(ud),
			GetDeploy:    usecases.NewGetDeploy(ud),
			ReadLog:      usecases.NewReadLog(ud),
			FollowLog:    usecases.NewFollowLog(ud),
			GetStatus:    usecases.NewGetStatus(apps, dk, d.Env, d.Clock),
			AppLogs:      usecases.NewAppLogs(apps, dk, d.Env),
		},
		log: d.Log,
	}
}

// RegisterRoutes registra los errores y todas las rutas del módulo
// (contrato §1 y §2): POST/GET …/apps/{a}/deploys, GET …/deploys/{id}, …/log
// y …/log/stream, POST …/apps/{a}/rollback, GET …/apps/{a}/status y
// …/logs (AccessToken), y /api/v1/registries (AccessSession).
func (m *Module) RegisterRoutes(k *utils.Kit) {
	dephttp.Register(k, m.uc, dephttp.Streams{Ctx: m.streamCtx, Config: m.sse})
}

// SeedInitRegistry guarda Deps.InitRegistry como un registry solo en el
// primer arranque de la base (contrato §1.2 y §2.10), con la marca
// init_registry_applied, que guarda en ese arranque haya o no registry. El
// contenedor lo llama en cada arranque, justo después de
// admin.ApplyStartupSettings (y por eso de VerifyMasterKey: nunca cifra con
// una clave que no se verificó). Un registry que el admin borró no vuelve.
func (m *Module) SeedInitRegistry(ctx context.Context) error {
	return m.seed.Execute(ctx, m.initRegistry)
}

// Jobs son las tareas periódicas. Fase 2: ninguna obligatoria (la retención de
// los últimos 20 deploys por app se hace al cerrar cada deploy).
func (m *Module) Jobs() []cron.Job { return nil }

// Run corre los workers de deploy hasta que ctx se cancela y vuelve cuando
// todos terminaron (el contenedor lo espera antes de cerrar la base). Primero
// la reconciliación al arrancar (§7, contrato §1.2): guarda manager_node_id si
// no está y arranca un worker por cada app con un deploy running o queued
// (cada uno cierra primero el running que quedó, según el estado real de
// Swarm, y después retoma el queued); después, un worker por app con trabajo,
// con un deploy a la vez. El apagado nunca cierra un deploy: lo deja running.
// Se llama una sola vez.
func (m *Module) Run(ctx context.Context) {
	m.storeManager.Execute(ctx)
	apps, err := m.deploys.ActiveApps(ctx)
	if err != nil && ctx.Err() == nil {
		m.log.Error("deploy: no se pudieron leer los deploys pendientes", "error", err)
	}
	m.dispatcher.Start(ctx, apps)
	<-ctx.Done()
	// Los streams se cierran enseguida: si no, srv.Shutdown esperaría sus 8 s.
	m.stopStreams()
	m.dispatcher.Wait()
}

// RemoveApp borra lo que la app tiene en Swarm (contrato §1.1). La llama
// catalog al borrar la app, antes de borrar la fila: deja de aceptar deploys
// de la app por 2 min (409 conflict), detiene su worker (el deploy en curso
// cierra failed), marca failed sus queued y hace docker stack rm de
// proyecto_stage_app (los volúmenes quedan); un stack que no existe no es un
// error. Docker caído o sin Swarm: un error que envuelve
// entity.ErrDockerUnavailable (503). Si el worker no se detiene en 15 s,
// service.ErrWorkerBusy (409). En los dos casos la app no se borra y vuelve a
// aceptar deploys.
func (m *Module) RemoveApp(ctx context.Context, project, stage, app string) error {
	return m.removeApp.Execute(ctx, dto.AppRef{Project: project, Stage: stage, App: app})
}
