// Package service tiene los adaptadores de catalog que leen tablas de otros
// módulos (§4.1 regla 1).
package service

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/catalog/domain/service"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// DomainCounter lee la tabla domains (de ingress, Fase 3).
type DomainCounter struct {
	db *sqlite.DB
}

var _ service.AppDomains = (*DomainCounter)(nil)

func NewDomainCounter(db *sqlite.DB) *DomainCounter { return &DomainCounter{db: db} }

func (c *DomainCounter) CountByApp(ctx context.Context, appID int64) (int, error) {
	var n int
	err := c.db.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM domains WHERE app_id = ?`, appID).Scan(&n)
	return n, err
}
