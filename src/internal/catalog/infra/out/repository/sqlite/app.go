package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/CPU-commits/muelle/src/internal/catalog/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/catalog/domain/repository"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

type AppRepository struct {
	db *sqlitedb.DB
}

var _ repository.AppRepository = (*AppRepository)(nil)

func NewAppRepository(db *sqlitedb.DB) *AppRepository { return &AppRepository{db: db} }

const appSelect = `SELECT a.id, a.stage_id, p.slug, s.slug, a.slug, a.kind, a.spec, a.compose_yaml,
	a.deploy_timeout_s, a.config_changed_at, a.last_deployed_at, a.created_at
	FROM apps a JOIN stages s ON s.id = a.stage_id JOIN projects p ON p.id = s.project_id`

// specColumns devuelve spec y compose_yaml como columnas: la que no
// corresponde al tipo va NULL (el CHECK de la tabla lo exige).
func specColumns(a entity.App) (spec, compose sql.NullString, err error) {
	switch a.Kind {
	case entity.KindService:
		return sql.NullString{String: string(a.Spec), Valid: true}, sql.NullString{}, nil
	case entity.KindCompose:
		return sql.NullString{}, sql.NullString{String: a.ComposeYAML, Valid: true}, nil
	}
	return spec, compose, fmt.Errorf("catalog: kind inválido %q", a.Kind)
}

func (r *AppRepository) Create(ctx context.Context, a *entity.App) error {
	spec, compose, err := specColumns(*a)
	if err != nil {
		return err
	}
	res, err := r.db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO apps (stage_id, slug, kind, spec, compose_yaml, deploy_timeout_s, config_changed_at, last_deployed_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.StageID, a.Slug, string(a.Kind), spec, compose, a.DeployTimeoutS,
		sqlitedb.Millis(a.ConfigChangedAt), sqlitedb.NullMillis(a.LastDeployedAt), sqlitedb.Millis(a.CreatedAt))
	if sqlitedb.IsForeignKey(err) {
		// Mapear cualquier error de clave foránea a ErrNotFound es correcto solo
		// mientras el insert tenga una única FK (stage_id).
		return srepo.ErrNotFound // el stage se borró en paralelo
	}
	if sqlitedb.IsUnique(err) {
		return srepo.ErrConflict
	}
	if err != nil {
		return err
	}
	a.ID, err = res.LastInsertId()
	return err
}

func (r *AppRepository) Get(ctx context.Context, projectSlug, stageSlug, slug string) (entity.App, error) {
	row := r.db.Conn(ctx).QueryRowContext(ctx,
		appSelect+` WHERE p.slug = ? AND s.slug = ? AND a.slug = ?`, projectSlug, stageSlug, slug)
	a, err := scanApp(row)
	return a, sqlitedb.NotFound(err)
}

func (r *AppRepository) ListByStage(ctx context.Context, stageID int64) ([]entity.App, error) {
	rows, err := r.db.Conn(ctx).QueryContext(ctx, appSelect+` WHERE a.stage_id = ? ORDER BY a.slug`, stageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entity.App{}
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *AppRepository) CountByStage(ctx context.Context, stageID int64) (int, error) {
	var n int
	err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM apps WHERE stage_id = ?`, stageID).Scan(&n)
	return n, err
}

func (r *AppRepository) Update(ctx context.Context, a entity.App) error {
	spec, compose, err := specColumns(a)
	if err != nil {
		return err
	}
	res, err := r.db.Conn(ctx).ExecContext(ctx,
		`UPDATE apps SET spec = ?, compose_yaml = ?, deploy_timeout_s = ?, config_changed_at = ? WHERE id = ?`,
		spec, compose, a.DeployTimeoutS, sqlitedb.Millis(a.ConfigChangedAt), a.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return srepo.ErrNotFound
	}
	return nil
}

// Delete borra la app y, en cascada, sus variables.
// Delete borra la app. Con dominios (domains.app_id es RESTRICT) devuelve ErrNotEmpty.
func (r *AppRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM apps WHERE id = ?`, id)
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

func scanApp(sc scanner) (entity.App, error) {
	var a entity.App
	var kind string
	var spec, compose sql.NullString
	var changed, created int64
	var deployed sql.NullInt64
	err := sc.Scan(&a.ID, &a.StageID, &a.ProjectSlug, &a.StageSlug, &a.Slug, &kind, &spec, &compose,
		&a.DeployTimeoutS, &changed, &deployed, &created)
	if err != nil {
		return entity.App{}, err
	}
	a.Kind = entity.Kind(kind)
	if spec.Valid {
		a.Spec = []byte(spec.String)
	}
	a.ComposeYAML = compose.String
	a.ConfigChangedAt = sqlitedb.Time(changed)
	a.LastDeployedAt = sqlitedb.NullTime(deployed)
	a.CreatedAt = sqlitedb.Time(created)
	return a, nil
}
