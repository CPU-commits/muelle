// Package service tiene los adaptadores de los puertos de servicio del módulo admin.
package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/CPU-commits/muelle/src/internal/admin/domain/service"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// TOTPStatus lee users.totp_secret directo (las tablas de identity), sin importar identity.
type TOTPStatus struct {
	db *sqlite.DB
}

var _ service.TOTPStatus = (*TOTPStatus)(nil)

func NewTOTPStatus(db *sqlite.DB) *TOTPStatus { return &TOTPStatus{db: db} }

// HasTOTP dice si el usuario tiene 2FA activo. Un usuario que no existe no lo tiene.
func (s *TOTPStatus) HasTOTP(ctx context.Context, userID int64) (bool, error) {
	var has bool
	err := s.db.Conn(ctx).QueryRowContext(ctx,
		`SELECT totp_secret IS NOT NULL FROM users WHERE id = ?`, userID).Scan(&has)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return has, err
}
