// Package usecases tiene los casos de uso del catálogo. Dependen solo de
// los puertos del dominio, del reloj, de la auditoría y del Transactor.
package usecases

import (
	"github.com/CPU-commits/muelle/src/internal/catalog/domain/repository"
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
}

func projectTarget(p string) string { return "project:" + p }

func stageTarget(p, s string) string { return "stage:" + p + "/" + s }

func appTarget(p, s, a string) string { return "app:" + p + "/" + s + "/" + a }
