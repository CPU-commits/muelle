package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/repository"
	"github.com/CPU-commits/muelle/src/package/cipher"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// RegistryRepository implementa repository.RegistryRepository. La contraseña
// va cifrada con entity.PasswordAAD(id): el id sale del INSERT, así que Create
// inserta primero una contraseña vacía y la reemplaza en la misma transacción.
type RegistryRepository struct {
	db     *sqlitedb.DB
	cipher cipher.Cipher
}

var _ repository.RegistryRepository = (*RegistryRepository)(nil)

func NewRegistryRepository(db *sqlitedb.DB, c cipher.Cipher) *RegistryRepository {
	return &RegistryRepository{db: db, cipher: c}
}

// scanPublic lee una fila sin la contraseña.
func scanPublic(s scanner) (entity.Registry, error) {
	var r entity.Registry
	var created, updated int64
	if err := s.Scan(&r.ID, &r.Host, &r.Username, &created, &updated); err != nil {
		return entity.Registry{}, err
	}
	r.CreatedAt, r.UpdatedAt = sqlitedb.Time(created), sqlitedb.Time(updated)
	return r, nil
}

const registryPublic = `SELECT id, host, username, created_at, updated_at FROM registries`

func (r *RegistryRepository) List(ctx context.Context) ([]entity.Registry, error) {
	rows, err := r.db.Conn(ctx).QueryContext(ctx, registryPublic+` ORDER BY host`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entity.Registry{}
	for rows.Next() {
		reg, err := scanPublic(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, reg)
	}
	return out, rows.Err()
}

func (r *RegistryRepository) Get(ctx context.Context, host string) (entity.Registry, error) {
	reg, err := scanPublic(r.db.Conn(ctx).QueryRowContext(ctx, registryPublic+` WHERE host = ?`, host))
	return reg, sqlitedb.NotFound(err)
}

func (r *RegistryRepository) Create(ctx context.Context, reg *entity.Registry) error {
	return r.db.InTx(ctx, func(ctx context.Context) error {
		var id int64
		err := r.db.Conn(ctx).QueryRowContext(ctx,
			`INSERT INTO registries (host, username, password, created_at, updated_at) VALUES (?, ?, x'', ?, ?) RETURNING id`,
			reg.Host, reg.Username, sqlitedb.Millis(reg.CreatedAt), sqlitedb.Millis(reg.UpdatedAt)).Scan(&id)
		if sqlitedb.IsUnique(err) {
			return srepo.ErrConflict
		}
		if err != nil {
			return err
		}
		ct := r.cipher.Encrypt([]byte(reg.Password), entity.PasswordAAD(id))
		if _, err := r.db.Conn(ctx).ExecContext(ctx, `UPDATE registries SET password = ? WHERE id = ?`, ct, id); err != nil {
			return err
		}
		reg.ID = id
		return nil
	})
}

func (r *RegistryRepository) Update(ctx context.Context, reg *entity.Registry) error {
	return r.db.InTx(ctx, func(ctx context.Context) error {
		var id, created int64
		err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT id, created_at FROM registries WHERE host = ?`, reg.Host).Scan(&id, &created)
		if err != nil {
			return sqlitedb.NotFound(err)
		}
		ct := r.cipher.Encrypt([]byte(reg.Password), entity.PasswordAAD(id))
		if _, err := r.db.Conn(ctx).ExecContext(ctx,
			`UPDATE registries SET username = ?, password = ?, updated_at = ? WHERE id = ?`,
			reg.Username, ct, sqlitedb.Millis(reg.UpdatedAt), id); err != nil {
			return err
		}
		reg.ID, reg.CreatedAt = id, sqlitedb.Time(created)
		return nil
	})
}

func (r *RegistryRepository) Delete(ctx context.Context, host string) error {
	res, err := r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM registries WHERE host = ?`, host)
	return affected(res, err)
}

// Credentials descifra solo las filas de los hosts pedidos. Un error de
// descifrado nombra el host, nunca la contraseña.
func (r *RegistryRepository) Credentials(ctx context.Context, hosts []string) (map[string]entity.Registry, error) {
	out := map[string]entity.Registry{}
	if len(hosts) == 0 {
		return out, nil
	}
	args := make([]any, len(hosts))
	for i, h := range hosts {
		args[i] = h
	}
	rows, err := r.db.Conn(ctx).QueryContext(ctx,
		`SELECT id, host, username, password, created_at, updated_at FROM registries WHERE host IN (?`+
			strings.Repeat(", ?", len(hosts)-1)+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var reg entity.Registry
		var ct []byte
		var created, updated int64
		if err := rows.Scan(&reg.ID, &reg.Host, &reg.Username, &ct, &created, &updated); err != nil {
			return nil, err
		}
		pt, err := r.cipher.Decrypt(ct, entity.PasswordAAD(reg.ID))
		if err != nil {
			return nil, fmt.Errorf("registries: descifrando la contraseña de %s: %w", reg.Host, err)
		}
		reg.Password = string(pt)
		reg.CreatedAt, reg.UpdatedAt = sqlitedb.Time(created), sqlitedb.Time(updated)
		out[reg.Host] = reg
	}
	return out, rows.Err()
}
