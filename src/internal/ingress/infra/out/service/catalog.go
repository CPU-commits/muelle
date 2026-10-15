package service

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// CatalogReader lee apps, stages y projects (de catalog, §4.1 regla 1).
type CatalogReader struct {
	db *sqlite.DB
}

var _ service.Catalog = (*CatalogReader)(nil)

func NewCatalogReader(db *sqlite.DB) *CatalogReader { return &CatalogReader{db: db} }

const catalogAppSelect = `SELECT a.id, p.slug, s.slug, a.slug, a.kind
	FROM apps a JOIN stages s ON s.id = a.stage_id JOIN projects p ON p.id = s.project_id`

func (r *CatalogReader) App(ctx context.Context, project, stage, app string) (service.App, error) {
	a, err := scanCatalogApp(r.db.Conn(ctx).QueryRowContext(ctx,
		catalogAppSelect+` WHERE p.slug = ? AND s.slug = ? AND a.slug = ?`, project, stage, app))
	return a, sqlite.NotFound(err)
}

func (r *CatalogReader) Apps(ctx context.Context) (map[int64]service.App, error) {
	rows, err := r.db.Conn(ctx).QueryContext(ctx, catalogAppSelect)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]service.App{}
	for rows.Next() {
		a, err := scanCatalogApp(rows)
		if err != nil {
			return nil, err
		}
		out[a.ID] = a
	}
	return out, rows.Err()
}

func (r *CatalogReader) Stages(ctx context.Context) ([]service.Stage, error) {
	rows, err := r.db.Conn(ctx).QueryContext(ctx,
		`SELECT p.slug, s.slug FROM stages s JOIN projects p ON p.id = s.project_id ORDER BY p.slug, s.slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []service.Stage{}
	for rows.Next() {
		var s service.Stage
		if err := rows.Scan(&s.Project, &s.Stage); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func scanCatalogApp(sc interface{ Scan(...any) error }) (service.App, error) {
	var a service.App
	if err := sc.Scan(&a.ID, &a.Project, &a.Stage, &a.Slug, &a.Kind); err != nil {
		return service.App{}, err
	}
	return a, nil
}
