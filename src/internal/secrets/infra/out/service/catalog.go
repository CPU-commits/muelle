// Package service tiene los adaptadores de los puertos de secrets hacia
// catalog. Leen y actualizan directo las tablas projects, stages y apps,
// sin importar el módulo catalog (§4.1 del spec).
package service

import (
	"context"
	"time"

	"github.com/CPU-commits/muelle/src/internal/secrets/domain/entity"
	ports "github.com/CPU-commits/muelle/src/internal/secrets/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// OwnerResolver traduce slugs a ids de stages y apps.
type OwnerResolver struct {
	db *sqlitedb.DB
}

var _ ports.OwnerResolver = (*OwnerResolver)(nil)

func NewOwnerResolver(db *sqlitedb.DB) *OwnerResolver { return &OwnerResolver{db: db} }

func (r *OwnerResolver) Stage(ctx context.Context, project, stage string) (entity.Owner, error) {
	var id int64
	err := r.db.Conn(ctx).QueryRowContext(ctx,
		`SELECT s.id FROM stages s JOIN projects p ON p.id = s.project_id
		 WHERE p.slug = ? AND s.slug = ?`, project, stage).Scan(&id)
	if err != nil {
		return entity.Owner{}, sqlitedb.NotFound(err)
	}
	return entity.StageOwner(id), nil
}

func (r *OwnerResolver) App(ctx context.Context, project, stage, app string) (entity.Owner, error) {
	var id int64
	err := r.db.Conn(ctx).QueryRowContext(ctx,
		`SELECT a.id FROM apps a JOIN stages s ON s.id = a.stage_id JOIN projects p ON p.id = s.project_id
		 WHERE p.slug = ? AND s.slug = ? AND a.slug = ?`, project, stage, app).Scan(&id)
	if err != nil {
		return entity.Owner{}, sqlitedb.NotFound(err)
	}
	return entity.AppOwner(id), nil
}

func (r *OwnerResolver) StageOfApp(ctx context.Context, appID int64) (entity.Owner, error) {
	var stageID int64
	err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT stage_id FROM apps WHERE id = ?`, appID).Scan(&stageID)
	if err != nil {
		return entity.Owner{}, sqlitedb.NotFound(err)
	}
	return entity.StageOwner(stageID), nil
}

// ChangeMarker actualiza apps.config_changed_at.
type ChangeMarker struct {
	db *sqlitedb.DB
}

var _ ports.ChangeMarker = (*ChangeMarker)(nil)

func NewChangeMarker(db *sqlitedb.DB) *ChangeMarker { return &ChangeMarker{db: db} }

// Mark marca la app, o todas las apps del stage. Una app inexistente da
// repository.ErrNotFound; un stage sin apps no tiene nada que marcar.
func (m *ChangeMarker) Mark(ctx context.Context, o entity.Owner, at time.Time) error {
	if err := o.Check(); err != nil {
		return err
	}
	conn := m.db.Conn(ctx)
	if o.Kind == entity.OwnerStage {
		_, err := conn.ExecContext(ctx, `UPDATE apps SET config_changed_at = ? WHERE stage_id = ?`, sqlitedb.Millis(at), o.ID)
		return err
	}
	res, err := conn.ExecContext(ctx, `UPDATE apps SET config_changed_at = ? WHERE id = ?`, sqlitedb.Millis(at), o.ID)
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
