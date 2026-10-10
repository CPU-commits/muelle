package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// ScopeResolver implementa service.ScopeResolver leyendo directo las tablas
// projects y stages de catalog (solo lectura).
type ScopeResolver struct {
	db *sqlitedb.DB
}

var _ service.ScopeResolver = (*ScopeResolver)(nil)

func NewScopeResolver(db *sqlitedb.DB) *ScopeResolver { return &ScopeResolver{db: db} }

func (r *ScopeResolver) Resolve(ctx context.Context, project, stage string) (int64, *int64, error) {
	conn := r.db.Conn(ctx)
	var projectID int64
	err := conn.QueryRowContext(ctx, `SELECT id FROM projects WHERE slug = ?`, project).Scan(&projectID)
	if err != nil {
		return 0, nil, notFound(err)
	}
	if stage == "" {
		return projectID, nil, nil
	}
	var stageID int64
	err = conn.QueryRowContext(ctx, `SELECT id FROM stages WHERE project_id = ? AND slug = ?`, projectID, stage).Scan(&stageID)
	if err != nil {
		return 0, nil, notFound(err)
	}
	return projectID, &stageID, nil
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return repository.ErrNotFound
	}
	return err
}
