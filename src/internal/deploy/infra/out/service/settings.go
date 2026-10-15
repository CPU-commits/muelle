package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// DeploySettings lee y escribe en la tabla settings (de admin, §4.1 regla 1)
// solo las claves de deploy (contrato §1.1): init_registry_applied y
// manager_node_id.
type DeploySettings struct {
	db *sqlite.DB
}

var (
	_ service.Marks       = (*DeploySettings)(nil)
	_ service.ManagerNode = (*DeploySettings)(nil)
)

func NewDeploySettings(db *sqlite.DB) *DeploySettings { return &DeploySettings{db: db} }

// MarkOnce inserta key = "true" sin cifrar si no estaba, con la transacción
// del ctx; dice si la insertó.
func (s *DeploySettings) MarkOnce(ctx context.Context, key string) (bool, error) {
	res, err := s.db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO settings (key, value, encrypted) VALUES (?, ?, 0) ON CONFLICT (key) DO NOTHING`,
		key, []byte(shared.FormatBool(true)))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// Get es settings.manager_node_id, o "" si no está.
func (s *DeploySettings) Get(ctx context.Context) (string, error) {
	var v []byte
	err := s.db.Conn(ctx).QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = ?`, shared.SettingManagerNodeID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return string(v), err
}

// SetIfMissing guarda manager_node_id = id sin cifrar si no estaba (o estaba
// vacío) y devuelve el que quedó, en una transacción.
func (s *DeploySettings) SetIfMissing(ctx context.Context, id string) (string, error) {
	var out string
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		_, err := s.db.Conn(ctx).ExecContext(ctx,
			`INSERT INTO settings (key, value, encrypted) VALUES (?, ?, 0)
			 ON CONFLICT (key) DO UPDATE SET value = excluded.value WHERE length(settings.value) = 0`,
			shared.SettingManagerNodeID, []byte(id))
		if err != nil {
			return err
		}
		out, err = s.Get(ctx)
		return err
	})
	return out, err
}
