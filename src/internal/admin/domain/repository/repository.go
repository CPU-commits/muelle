// Package repository define los puertos de persistencia del módulo admin.
// Los errores comunes (ErrNotFound) están en shared/domain/repository.
package repository

import (
	"context"
	"time"

	"github.com/CPU-commits/muelle/src/internal/admin/domain/entity"
)

// SettingsRepository guarda pares clave/valor. Get devuelve
// repository.ErrNotFound si la clave no existe. Un valor cifrado se guarda
// tal cual lo recibe: cifrar y descifrar es cosa del caso de uso.
type SettingsRepository interface {
	Get(ctx context.Context, key string) (value []byte, encrypted bool, err error)
	Set(ctx context.Context, key string, value []byte, encrypted bool) error
	// Insert guarda el valor solo si la clave no existe; si existe devuelve
	// repository.ErrConflict y no toca nada.
	Insert(ctx context.Context, key string, value []byte, encrypted bool) error
}

// AuditRepository lista y purga los eventos. Escribirlos es del audit.Recorder.
type AuditRepository interface {
	// List devuelve los eventos más nuevos primero. beforeID 0 = desde el último.
	List(ctx context.Context, beforeID int64, limit int) ([]entity.AuditEvent, error)
	// DeleteBefore borra los eventos anteriores a t y devuelve cuántos.
	DeleteBefore(ctx context.Context, t time.Time) (int64, error)
}
