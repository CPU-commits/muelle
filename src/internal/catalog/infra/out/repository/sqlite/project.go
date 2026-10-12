// Package sqlite implementa los repositorios del catálogo sobre SQLite.
// Todas las consultas usan db.Conn(ctx), así participan en el InTx del ctx.
package sqlite

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/catalog/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/catalog/domain/repository"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

type ProjectRepository struct {
	db *sqlitedb.DB
}

var _ repository.ProjectRepository = (*ProjectRepository)(nil)

func NewProjectRepository(db *sqlitedb.DB) *ProjectRepository { return &ProjectRepository{db: db} }

// scanner lo cumplen *sql.Row y *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

func (r *ProjectRepository) Create(ctx context.Context, p *entity.Project) error {
	res, err := r.db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO projects (slug, name, created_at) VALUES (?, ?, ?)`,
		p.Slug, p.Name, sqlitedb.Millis(p.CreatedAt))
	if sqlitedb.IsUnique(err) {
		return srepo.ErrConflict
	}
	if err != nil {
		return err
	}
	p.ID, err = res.LastInsertId()
	return err
}

func (r *ProjectRepository) GetBySlug(ctx context.Context, slug string) (entity.Project, error) {
	row := r.db.Conn(ctx).QueryRowContext(ctx,
		`SELECT id, slug, name, created_at FROM projects WHERE slug = ?`, slug)
	p, err := scanProject(row)
	return p, sqlitedb.NotFound(err)
}

func (r *ProjectRepository) List(ctx context.Context) ([]entity.Project, error) {
	rows, err := r.db.Conn(ctx).QueryContext(ctx,
		`SELECT id, slug, name, created_at FROM projects ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entity.Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *ProjectRepository) Update(ctx context.Context, p entity.Project) error {
	res, err := r.db.Conn(ctx).ExecContext(ctx, `UPDATE projects SET name = ? WHERE id = ?`, p.Name, p.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return srepo.ErrNotFound
	}
	return nil
}

// Delete devuelve ErrNotEmpty si el proyecto tiene stages: la clave foránea
// RESTRICT lo garantiza aunque el caso de uso no lo haya comprobado.
func (r *ProjectRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	if sqlitedb.IsForeignKey(err) {
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

func scanProject(sc scanner) (entity.Project, error) {
	var p entity.Project
	var created int64
	if err := sc.Scan(&p.ID, &p.Slug, &p.Name, &created); err != nil {
		return entity.Project{}, err
	}
	p.CreatedAt = sqlitedb.Time(created)
	return p, nil
}
