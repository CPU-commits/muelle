// Package sqlite implementa los repositorios del módulo admin sobre la base compartida.
package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/CPU-commits/muelle/src/internal/admin/domain/repository"
	shared "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

type SettingsRepository struct {
	db *sqlite.DB
}

var _ repository.SettingsRepository = (*SettingsRepository)(nil)

func NewSettingsRepository(db *sqlite.DB) *SettingsRepository {
	return &SettingsRepository{db: db}
}

func (r *SettingsRepository) Get(ctx context.Context, key string) ([]byte, bool, error) {
	var value []byte
	var encrypted int
	err := r.db.Conn(ctx).QueryRowContext(ctx,
		`SELECT value, encrypted FROM settings WHERE key = ?`, key).Scan(&value, &encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, shared.ErrNotFound
	}
	if err != nil {
		return nil, false, err
	}
	return value, encrypted == 1, nil
}

// Set guarda un valor sin cifrar como texto (así lo lee SQL con = 'true') y uno cifrado como blob.
func (r *SettingsRepository) Set(ctx context.Context, key string, value []byte, encrypted bool) error {
	var v any = string(value)
	enc := 0
	if encrypted {
		v, enc = value, 1
	}
	_, err := r.db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO settings (key, value, encrypted) VALUES (?, ?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value, encrypted = excluded.encrypted`,
		key, v, enc)
	return err
}
