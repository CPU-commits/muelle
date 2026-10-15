package service

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// DeploySettings lee y escribe en la tabla settings (de admin, §4.1 regla 1)
// solo las claves de deploy (contrato §1.1): init_registry_applied.
type DeploySettings struct {
	db *sqlite.DB
}

var _ service.Marks = (*DeploySettings)(nil)

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
