// Package sqlite implementa los repositorios de identity sobre la base
// compartida. Todas las consultas usan Conn(ctx), así participan en la
// transacción del ctx si hay una.
package sqlite

import (
	"database/sql"
	"errors"

	"github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// scanner es lo común entre *sql.Row y *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// notFound traduce sql.ErrNoRows a repository.ErrNotFound.
func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return repository.ErrNotFound
	}
	return err
}

// affectedOne devuelve ErrNotFound si la sentencia no tocó ninguna fila.
func affectedOne(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func nullInt(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

func fromNullInt(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	n := v.Int64
	return &n
}
