// Package sqlite implementa los repositorios de secrets sobre la tabla env_vars.
package sqlite

import (
	"context"
	"database/sql"

	"github.com/CPU-commits/muelle/src/internal/secrets/domain/entity"
	secretsrepo "github.com/CPU-commits/muelle/src/internal/secrets/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// EnvRepository guarda las variables ya cifradas en env_vars.
type EnvRepository struct {
	db *sqlitedb.DB
}

var _ secretsrepo.EnvRepository = (*EnvRepository)(nil)

func NewEnvRepository(db *sqlitedb.DB) *EnvRepository { return &EnvRepository{db: db} }

// where devuelve la condición sobre la columna del dueño (stage_id o app_id).
func where(o entity.Owner) (string, int64, error) {
	if err := o.Check(); err != nil {
		return "", 0, err
	}
	if o.Kind == entity.OwnerApp {
		return "app_id = ?", o.ID, nil
	}
	return "stage_id = ?", o.ID, nil
}

func (r *EnvRepository) List(ctx context.Context, o entity.Owner) ([]entity.EncryptedEnvVar, error) {
	cond, id, err := where(o)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Conn(ctx).QueryContext(ctx,
		`SELECT key, value, secret, updated_at FROM env_vars WHERE `+cond+` ORDER BY key`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entity.EncryptedEnvVar{}
	for rows.Next() {
		var v entity.EncryptedEnvVar
		var updated int64
		if err := rows.Scan(&v.Key, &v.Ciphertext, &v.Secret, &updated); err != nil {
			return nil, err
		}
		v.UpdatedAt = sqlitedb.Time(updated)
		out = append(out, v)
	}
	return out, rows.Err()
}

// Replace corre en su propia transacción, o en la del ctx si ya hay una. No hay
// savepoints: dentro de otra transacción, quien llama tiene que devolver el
// error para que se revierta todo.
func (r *EnvRepository) Replace(ctx context.Context, o entity.Owner, vars []entity.EncryptedEnvVar) error {
	cond, id, err := where(o)
	if err != nil {
		return err
	}
	return r.db.InTx(ctx, func(ctx context.Context) error {
		if _, err := r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM env_vars WHERE `+cond, id); err != nil {
			return err
		}
		for _, v := range vars {
			if err := r.insert(ctx, o, v); err != nil {
				return err
			}
		}
		return nil
	})
}

// Patch corre en su propia transacción, o en la del ctx si ya hay una (sin
// savepoints, como Replace).
func (r *EnvRepository) Patch(ctx context.Context, o entity.Owner, set []entity.EncryptedEnvVar, unset []string) error {
	cond, id, err := where(o)
	if err != nil {
		return err
	}
	return r.db.InTx(ctx, func(ctx context.Context) error {
		conn := r.db.Conn(ctx)
		for _, key := range unset {
			if _, err := conn.ExecContext(ctx, `DELETE FROM env_vars WHERE `+cond+` AND key = ?`, id, key); err != nil {
				return err
			}
		}
		for _, v := range set {
			if _, err := conn.ExecContext(ctx, `DELETE FROM env_vars WHERE `+cond+` AND key = ?`, id, v.Key); err != nil {
				return err
			}
			if err := r.insert(ctx, o, v); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *EnvRepository) insert(ctx context.Context, o entity.Owner, v entity.EncryptedEnvVar) error {
	var stageID, appID sql.NullInt64
	if o.Kind == entity.OwnerApp {
		appID = sql.NullInt64{Int64: o.ID, Valid: true}
	} else {
		stageID = sql.NullInt64{Int64: o.ID, Valid: true}
	}
	_, err := r.db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO env_vars (stage_id, app_id, key, value, secret, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		stageID, appID, v.Key, v.Ciphertext, v.Secret, sqlitedb.Millis(v.UpdatedAt))
	if sqlitedb.IsForeignKey(err) {
		// El stage o la app no existe (o se borró entre medio).
		return repository.ErrNotFound
	}
	return err
}
