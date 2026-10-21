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

// storable da el valor y la marca como se guardan: un valor sin cifrar como
// texto (así lo lee SQL con = 'true') y uno cifrado como blob.
func storable(value []byte, encrypted bool) (any, int) {
	if encrypted {
		return value, 1
	}
	return string(value), 0
}

func (r *SettingsRepository) Set(ctx context.Context, key string, value []byte, encrypted bool) error {
	v, enc := storable(value, encrypted)
	_, err := r.db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO settings (key, value, encrypted) VALUES (?, ?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value, encrypted = excluded.encrypted`,
		key, v, enc)
	return err
}

func (r *SettingsRepository) Delete(ctx context.Context, key string) error {
	_, err := r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, key)
	return err
}

func (r *SettingsRepository) Insert(ctx context.Context, key string, value []byte, encrypted bool) error {
	v, enc := storable(value, encrypted)
	res, err := r.db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO settings (key, value, encrypted) VALUES (?, ?, ?) ON CONFLICT (key) DO NOTHING`,
		key, v, enc)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return shared.ErrConflict
	}
	return nil
}
