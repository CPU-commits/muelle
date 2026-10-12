// Package catalog es el módulo de proyectos, stages y apps: su CRUD y el spec
// de las apps de formulario. Es el único lugar que conoce todas sus piezas.
package catalog

import (
	"log/slog"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/catalog/app/usecases"
	cathttp "github.com/CPU-commits/muelle/src/internal/catalog/infra/in/http"
	catsqlite "github.com/CPU-commits/muelle/src/internal/catalog/infra/out/repository/sqlite"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/clock"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

type Deps struct {
	DB    *sqlitedb.DB
	Clock clock.Clock
	Audit audit.Recorder
	Log   *slog.Logger
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
