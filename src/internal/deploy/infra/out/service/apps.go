package service

import (
	"context"
	"database/sql"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// CatalogApps lee las apps de la tabla apps (de catalog, §4.1 regla 1) y
// escribe solo storage_node_id y last_deployed_at.
type CatalogApps struct {
	db *sqlite.DB
}

var _ service.Apps = (*CatalogApps)(nil)

func NewCatalogApps(db *sqlite.DB) *CatalogApps { return &CatalogApps{db: db} }

const appSelect = `SELECT a.id, p.slug, s.slug, a.slug, a.kind, a.spec, a.compose_yaml,
	a.deploy_timeout_s, a.storage_node_id, a.config_changed_at, a.last_deployed_at
	FROM apps a JOIN stages s ON s.id = a.stage_id JOIN projects p ON p.id = s.project_id`

func (r *CatalogApps) Get(ctx context.Context, project, stage, app string) (service.App, error) {
	return scanApp(r.db.Conn(ctx).QueryRowContext(ctx,
		appSelect+` WHERE p.slug = ? AND s.slug = ? AND a.slug = ?`, project, stage, app))
}

func (r *CatalogApps) GetByID(ctx context.Context, id int64) (service.App, error) {
	return scanApp(r.db.Conn(ctx).QueryRowContext(ctx, appSelect+` WHERE a.id = ?`, id))
}

func scanApp(row *sql.Row) (service.App, error) {
	var a service.App
	var spec, compose, node sql.NullString
	var changed int64
	var deployed sql.NullInt64
	err := row.Scan(&a.ID, &a.Project, &a.Stage, &a.Slug, &a.Kind, &spec, &compose,
		&a.DeployTimeoutS, &node, &changed, &deployed)
	if err != nil {
		return service.App{}, sqlite.NotFound(err)
	}
	if spec.Valid {
		a.Spec = []byte(spec.String)
	}
	a.ComposeYAML = compose.String
	a.StorageNodeID = node.String
	a.ConfigChangedAt = sqlite.Time(changed)
	a.LastDeployedAt = sqlite.NullTime(deployed)
	return a, nil
}

func (r *CatalogApps) SetStorageNode(ctx context.Context, appID int64, nodeID string) (string, error) {
	var node string
	err := r.db.Conn(ctx).QueryRowContext(ctx,
		`UPDATE apps SET storage_node_id = COALESCE(NULLIF(storage_node_id, ''), ?) WHERE id = ? RETURNING storage_node_id`,
		nodeID, appID).Scan(&node)
	return node, sqlite.NotFound(err)
}

func (r *CatalogApps) MarkDeployed(ctx context.Context, appID int64, at time.Time) error {
	res, err := r.db.Conn(ctx).ExecContext(ctx,
		`UPDATE apps SET last_deployed_at = MAX(COALESCE(last_deployed_at, 0), ?) WHERE id = ?`,
		sqlite.Millis(at), appID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return repository.ErrNotFound
	}
	return nil
}

// DomainTargetsReader lee los destinos de la tabla domains (de ingress, §4.1 regla 1).
type DomainTargetsReader struct {
	db *sqlite.DB
}

var _ service.DomainTargets = (*DomainTargetsReader)(nil)

func NewDomainTargets(db *sqlite.DB) *DomainTargetsReader { return &DomainTargetsReader{db: db} }

func (r *DomainTargetsReader) ByApp(ctx context.Context, appID int64) ([]service.DomainTarget, error) {
	rows, err := r.db.Conn(ctx).QueryContext(ctx,
		`SELECT hostname, target_service, target_port FROM domains WHERE app_id = ? ORDER BY hostname`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []service.DomainTarget{}
	for rows.Next() {
		var d service.DomainTarget
		if err := rows.Scan(&d.Hostname, &d.Service, &d.Port); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
