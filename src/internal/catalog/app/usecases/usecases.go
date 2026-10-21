// Package usecases tiene los casos de uso del catálogo. Dependen solo de
// los puertos del dominio, del reloj, de la auditoría y del Transactor.
package usecases

import (
	"github.com/CPU-commits/muelle/src/internal/catalog/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/catalog/domain/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/clock"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// Deps son las piezas que usan todos los casos de uso.
type Deps struct {
	Tx       srepo.Transactor
	Projects repository.ProjectRepository
	Stages   repository.StageRepository
	Apps     repository.AppRepository
	Clock    clock.Clock
	Audit    audit.Recorder

	// Networks crea y borra la red de cada stage (ingress). nil: catalog no
	// toca Docker, como en la Fase 1 (los tests de un módulo solo).
	Networks service.StageNetworks
	// Runtime borra el stack de una app (deploy). nil: no toca Docker.
	Runtime service.AppRuntime
	// Domains cuenta los dominios de una app antes de borrarla. nil: solo la
	// clave foránea lo impide, y eso pasa después de borrar el stack.
	Domains service.AppDomains
}

func projectTarget(p string) string { return "project:" + p }

func stageTarget(p, s string) string { return "stage:" + p + "/" + s }

func appTarget(p, s, a string) string { return "app:" + p + "/" + s + "/" + a }
