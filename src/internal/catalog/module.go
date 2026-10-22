// Package catalog es el módulo de proyectos, stages y apps: su CRUD y el spec
// de las apps de formulario. Es el único lugar que conoce todas sus piezas.
package catalog

import (
	"log/slog"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/cmd/mcp/tools"
	"github.com/CPU-commits/muelle/src/internal/catalog/app/usecases"
	"github.com/CPU-commits/muelle/src/internal/catalog/domain/service"
	cathttp "github.com/CPU-commits/muelle/src/internal/catalog/infra/in/http"
	catsqlite "github.com/CPU-commits/muelle/src/internal/catalog/infra/out/repository/sqlite"
	outservice "github.com/CPU-commits/muelle/src/internal/catalog/infra/out/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/clock"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

type Deps struct {
	DB    *sqlitedb.DB
	Clock clock.Clock
	Audit audit.Recorder
	Log   *slog.Logger

	// Networks crea la red de un stage al crearlo y la borra al borrarlo
	// (§3.2); la implementa ingress.Module. Opcional: con nil, catalog no toca
	// Docker (el comportamiento de la Fase 1, y sus tests).
	Networks service.StageNetworks
	// Runtime ejecuta docker stack rm al borrar una app; lo implementa
	// deploy.Module. Opcional, como Networks.
	Runtime service.AppRuntime
}

type Module struct {
	uc cathttp.UseCases
}

func New(d Deps) *Module {
	ud := usecases.Deps{
		Tx:       d.DB,
		Projects: catsqlite.NewProjectRepository(d.DB),
		Stages:   catsqlite.NewStageRepository(d.DB),
		Apps:     catsqlite.NewAppRepository(d.DB),
		Clock:    d.Clock,
		Audit:    d.Audit,
		Networks: d.Networks,
		Runtime:  d.Runtime,
		Domains:  outservice.NewDomainCounter(d.DB),
	}
	return &Module{uc: cathttp.UseCases{
		CreateProject: usecases.NewCreateProject(ud),
		ListProjects:  usecases.NewListProjects(ud),
		GetProject:    usecases.NewGetProject(ud),
		UpdateProject: usecases.NewUpdateProject(ud),
		DeleteProject: usecases.NewDeleteProject(ud),

		CreateStage: usecases.NewCreateStage(ud),
		ListStages:  usecases.NewListStages(ud),
		GetStage:    usecases.NewGetStage(ud),
		DeleteStage: usecases.NewDeleteStage(ud),

		CreateApp: usecases.NewCreateApp(ud),
		ListApps:  usecases.NewListApps(ud),
		GetApp:    usecases.NewGetApp(ud),
		UpdateApp: usecases.NewUpdateApp(ud),
		DeleteApp: usecases.NewDeleteApp(ud),
	}}
}

// RegisterRoutes registra los endpoints de proyectos, stages y apps.
func (m *Module) RegisterRoutes(k *utils.Kit) { cathttp.Register(k, m.uc) }

// RegisterTools registra las herramientas MCP de catalog (contrato §8.3).
func (m *Module) RegisterTools(k *tools.Kit) {}
