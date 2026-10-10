package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// settingRequire2FA es la clave que guarda admin, con valor "true" o "false".
const settingRequire2FA = "require_2fa"

// SettingsReader implementa service.SettingsReader leyendo directo la tabla
// settings de admin (solo lectura).
type SettingsReader struct {
	db *sqlitedb.DB
}

var _ service.SettingsReader = (*SettingsReader)(nil)

func NewSettingsReader(db *sqlitedb.DB) *SettingsReader { return &SettingsReader{db: db} }

func (r *SettingsReader) Require2FA(ctx context.Context) (bool, error) {
	var v []byte
	err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, settingRequire2FA).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return string(v) == "true", nil
}
