package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// SettingsReader implementa service.SettingsReader leyendo directo la tabla
// settings de admin (solo lectura), con la clave y el formato que comparten
// los dos módulos (shared.SettingRequire2FA, shared.ParseBool).
type SettingsReader struct {
	db *sqlitedb.DB
}

var _ service.SettingsReader = (*SettingsReader)(nil)

func NewSettingsReader(db *sqlitedb.DB) *SettingsReader { return &SettingsReader{db: db} }

func (r *SettingsReader) Require2FA(ctx context.Context) (bool, error) {
	var v []byte
	err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, shared.SettingRequire2FA).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return shared.ParseBool(string(v)), nil
}
