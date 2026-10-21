// Package repository define los puertos de persistencia de deploy: los
// deploys y los registries (migración 0002). Los errores comunes
// (ErrNotFound, ErrConflict) están en shared/domain/repository.
package repository

import (
	"context"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
)

// DeployRepository guarda los deploys (contrato §2.1) y su log (§2.5).
// Todos los métodos usan la transacción del ctx si trae una.
type DeployRepository interface {
	// Create inserta d como queued con d.CreatedAt, y le asigna ID y Status.
	// d.Images son los overrides ({} si no hay). repository.ErrNotFound si la
	// app ya no existe (un DELETE de catalog entre la lectura de la app y este
	// INSERT), no el error de la clave foránea.
	Create(ctx context.Context, d *entity.Deploy) error
	// SupersedeQueued pasa a superseded los queued de la app salvo by, con
	// superseded_by = by y finished_at = at (contrato §2.2). Devuelve sus ids,
	// ordenados, para avisarles a los streams después del commit
	// (Hub.Notify); nunca nil.
	SupersedeQueued(ctx context.Context, appID, by int64, at time.Time) ([]int64, error)
	// Get es el deploy id de la app: repository.ErrNotFound si no existe o es
	// de otra app (contrato §2.4).
	Get(ctx context.Context, appID, id int64) (entity.Deploy, error)
	GetByID(ctx context.Context, id int64) (entity.Deploy, error)
	// List son los deploys de la app, del más nuevo al más viejo; nunca nil.
	List(ctx context.Context, appID int64) ([]entity.Deploy, error)
	// Current es el deploy actual (el succeeded más reciente), o nil.
	Current(ctx context.Context, appID int64) (*entity.Deploy, error)
	// PreviousSucceeded es el succeeded más reciente anterior a before (el
	// destino de un rollback, contrato §2.7), o nil.
	PreviousSucceeded(ctx context.Context, appID, before int64) (*entity.Deploy, error)
	// NextQueued es el queued más viejo de la app, o nil.
	NextQueued(ctx context.Context, appID int64) (*entity.Deploy, error)
	// Running es el deploy running de la app, o nil.
	Running(ctx context.Context, appID int64) (*entity.Deploy, error)
	// ActiveApps son las apps con un deploy queued o running (Run, contrato
	// §1.2), ordenadas; nunca nil.
	ActiveApps(ctx context.Context) ([]int64, error)
	// Claim pasa un queued a running con started_at = at; false si ya no
	// estaba queued.
	Claim(ctx context.Context, id int64, at time.Time) (bool, error)
	// SetImages guarda las refs pedidas, las de la configuración y las
	// resueltas (§7 paso 2, contrato §2.2). repository.ErrNotFound si no existe.
	SetImages(ctx context.Context, id int64, requested, config, resolved map[string]string) error
	// SetRoutesWarning guarda si el paso 7 no pudo actualizar Caddy (warn) o
	// sí (deploys.routes_warning, contrato §2.1). repository.ErrNotFound si no
	// existe.
	SetRoutesWarning(ctx context.Context, id int64, warn bool) error
	// AppendLog agrega lines (ya formateadas y enmascaradas: FormatLine) con
	// service.AppendPlan, en una transacción que lee el largo en bytes y
	// log_truncated y guarda los dos (contrato §2.5). final son las líneas de
	// cierre, que entran en la reserva; marker es TruncatedMarker formateada.
	// repository.ErrNotFound si no existe.
	AppendLog(ctx context.Context, id int64, lines []string, final bool, marker string) error
	// ReadLog devuelve desde offset hasta max bytes del log, en líneas
	// completas, con el largo total, el estado y superseded_by.
	// repository.ErrNotFound si el deploy no es de esa app;
	// service.ErrOffsetBeyond si offset pasa el largo del log.
	ReadLog(ctx context.Context, appID, id int64, offset, max int) (LogChunk, error)
	// Finish cierra un deploy running o queued: status (final), error y
	// finished_at = at. repository.ErrNotFound si no existe o ya estaba cerrado.
	Finish(ctx context.Context, id int64, status entity.Status, errMsg *string, at time.Time) error
	// FailQueued pasa a failed los queued de la app, con reason como error y
	// finished_at = at. Devuelve sus ids, como SupersedeQueued, para el
	// Hub.Notify; nunca nil.
	FailQueued(ctx context.Context, appID int64, reason string, at time.Time) ([]int64, error)
	// ApplyRetention borra los deploys de la app que no guarda la retención
	// (contrato §2.3) y dice cuántos borró: se quedan los 20 más nuevos, el
	// actual y el succeeded anterior, y los queued y running.
	ApplyRetention(ctx context.Context, appID int64) (int, error)
}

// LogChunk es un pedazo del log de un deploy (contrato §2.5).
type LogChunk struct {
	Chunk        []byte // desde el offset pedido, en líneas completas
	Length       int    // el largo total del log, en bytes
	Status       entity.Status
	SupersededBy *int64
}

// RegistryRepository guarda las credenciales de los registries (contrato
// §2.10), con la contraseña cifrada con la AAD registries:password:<id>.
type RegistryRepository interface {
	// List los devuelve ordenados por host, sin la contraseña; nunca nil.
	List(ctx context.Context) ([]entity.Registry, error)
	// Get es el registry de ese host, sin la contraseña. repository.ErrNotFound si no está.
	Get(ctx context.Context, host string) (entity.Registry, error)
	// Create guarda r (Host, Username, Password, CreatedAt y UpdatedAt) y le
	// asigna el ID. repository.ErrConflict si el host ya está.
	Create(ctx context.Context, r *entity.Registry) error
	// Update reemplaza el usuario y la contraseña del registry de r.Host, con
	// r.UpdatedAt, y completa r.ID y r.CreatedAt. repository.ErrNotFound si no está.
	Update(ctx context.Context, r *entity.Registry) error
	// Delete borra el registry de ese host. repository.ErrNotFound si no está.
	Delete(ctx context.Context, host string) error
	// Credentials descifra las credenciales de esos hosts: host → registry,
	// con la contraseña. Los que no tienen una no vienen.
	Credentials(ctx context.Context, hosts []string) (map[string]entity.Registry, error)
}
