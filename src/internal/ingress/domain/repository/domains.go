// Package repository tiene los repositorios de ingress (adaptador SQLite en
// infra/out/repository/sqlite).
package repository

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
)

// Domains es la tabla domains. Los errores de "no existe" son
// repository.ErrNotFound (shared/domain/repository).
type Domains interface {
	// Create guarda d y le asigna d.ID. service.ErrHostnameTaken (que envuelve
	// repository.ErrConflict) si el hostname ya existe, en cualquier app.
	Create(ctx context.Context, d *entity.Domain) error
	// Get es el dominio por hostname; repository.ErrNotFound si no está.
	Get(ctx context.Context, hostname string) (entity.Domain, error)
	// ListByApp son los dominios de la app, por hostname; [] si no hay.
	ListByApp(ctx context.Context, appID int64) ([]entity.Domain, error)
	// List son todos los dominios, por hostname; [] si no hay.
	List(ctx context.Context) ([]entity.Domain, error)
	// Update guarda todas las columnas menos id, app_id, hostname y
	// created_at; repository.ErrNotFound si d.ID no existe.
	Update(ctx context.Context, d entity.Domain) error
	// Delete borra el dominio; repository.ErrNotFound si no existe.
	Delete(ctx context.Context, id int64) error
}
