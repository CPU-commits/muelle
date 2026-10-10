package sqlite

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/catalog/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/catalog/domain/repository"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

type StageRepository struct {
	db *sqlitedb.DB
}

var _ repository.StageRepository = (*StageRepository)(nil)

func NewStageRepository(db *sqlitedb.DB) *StageRepository { return &StageRepository{db: db} }

const stageSelect = `SELECT s.id, s.project_id, p.slug, s.slug, s.created_at
	FROM stages s JOIN projects p ON p.id = s.project_id`

func (r *StageRepository) Create(ctx context.Context, s *entity.Stage) error {
	res, err := r.db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO stages (project_id, slug, created_at) VALUES (?, ?, ?)`,
		s.ProjectID, s.Slug, sqlitedb.Millis(s.CreatedAt))
	if sqlitedb.IsUnique(err) {
		return srepo.ErrConflict
	}
	if err != nil {
		return err
	}
	s.ID, err = res.LastInsertId()
	return err
}

func (r *StageRepository) Get(ctx context.Context, projectSlug, slug string) (entity.Stage, error) {
	row := r.db.Conn(ctx).QueryRowContext(ctx, stageSelect+` WHERE p.slug = ? AND s.slug = ?`, projectSlug, slug)
	s, err := scanStage(row)
	return s, notFound(err)
}

func (r *StageRepository) ListByProject(ctx context.Context, projectID int64) ([]entity.Stage, error) {
	rows, err := r.db.Conn(ctx).QueryContext(ctx, stageSelect+` WHERE s.project_id = ? ORDER BY s.slug`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entity.Stage{}
	for rows.Next() {
		s, err := scanStage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *StageRepository) CountByProject(ctx context.Context, projectID int64) (int, error) {
	var n int
	err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM stages WHERE project_id = ?`, projectID).Scan(&n)
	return n, err
}

// Delete devuelve ErrNotEmpty si el stage tiene apps (clave foránea RESTRICT).
func (r *StageRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM stages WHERE id = ?`, id)
	if isForeignKey(err) {
		return srepo.ErrNotEmpty
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return srepo.ErrNotFound
	}
	return nil
}

func scanStage(sc scanner) (entity.Stage, error) {
	var s entity.Stage
	var created int64
	if err := sc.Scan(&s.ID, &s.ProjectID, &s.ProjectSlug, &s.Slug, &created); err != nil {
		return entity.Stage{}, err
	}
	s.CreatedAt = sqlitedb.Time(created)
	return s, nil
}
