// Package repository define los puertos de persistencia del catálogo.
// Los errores comunes (ErrNotFound, ErrConflict, ErrNotEmpty) están en
// shared/domain/repository.
package repository

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/catalog/domain/entity"
)

type ProjectRepository interface {
	// Create guarda p y le asigna el ID. ErrConflict si el slug ya existe.
	Create(ctx context.Context, p *entity.Project) error
	// GetBySlug devuelve ErrNotFound si no existe.
	GetBySlug(ctx context.Context, slug string) (entity.Project, error)
	// List ordena por slug. Nunca devuelve nil.
	List(ctx context.Context) ([]entity.Project, error)
	// Update guarda el nombre. ErrNotFound si no existe.
	Update(ctx context.Context, p entity.Project) error
	// Delete devuelve ErrNotFound si no existe y ErrNotEmpty si tiene stages.
	Delete(ctx context.Context, id int64) error
}

type StageRepository interface {
	// Create guarda s (con ProjectID) y le asigna el ID. ErrConflict si el slug ya existe en el proyecto.
	Create(ctx context.Context, s *entity.Stage) error
	// Get devuelve ErrNotFound si no existe.
	Get(ctx context.Context, projectSlug, slug string) (entity.Stage, error)
	// ListByProject ordena por slug. Nunca devuelve nil.
	ListByProject(ctx context.Context, projectID int64) ([]entity.Stage, error)
	CountByProject(ctx context.Context, projectID int64) (int, error)
	// Delete devuelve ErrNotFound si no existe y ErrNotEmpty si tiene apps.
	Delete(ctx context.Context, id int64) error
}

type AppRepository interface {
	// Create guarda a (con StageID) y le asigna el ID. ErrConflict si el slug ya existe en el stage.
	Create(ctx context.Context, a *entity.App) error
	// Get devuelve ErrNotFound si no existe.
	Get(ctx context.Context, projectSlug, stageSlug, slug string) (entity.App, error)
	// ListByStage ordena por slug. Nunca devuelve nil.
	ListByStage(ctx context.Context, stageID int64) ([]entity.App, error)
	CountByStage(ctx context.Context, stageID int64) (int, error)
	// Update guarda spec, compose, timeout y config_changed_at. ErrNotFound si no existe.
	Update(ctx context.Context, a entity.App) error
	Delete(ctx context.Context, id int64) error
}
