// Package sqlite implementa los repositorios de ingress sobre la base
// compartida. Todas las consultas usan Conn(ctx), así participan en la
// transacción del ctx si hay una.
package sqlite

import (
	"context"
	"database/sql"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// Domains es la tabla domains (migración 0003).
type Domains struct {
	db *sqlitedb.DB
}

var _ repository.Domains = (*Domains)(nil)

func NewDomains(db *sqlitedb.DB) *Domains { return &Domains{db: db} }

const domainSelect = `SELECT id, app_id, hostname, target_service, target_port, proxied, tls_mode,
	cf_zone_id, cf_record_id, cf_record_id_aaaa, status, last_error_code, last_error,
	cert_issuer, cert_expires_at, created_at, updated_at FROM domains`

func (r *Domains) Create(ctx context.Context, d *entity.Domain) error {
	var id int64
	err := r.db.Conn(ctx).QueryRowContext(ctx, `INSERT INTO domains (app_id, hostname, target_service, target_port,
		proxied, tls_mode, cf_zone_id, cf_record_id, cf_record_id_aaaa, status, last_error_code, last_error,
		cert_issuer, cert_expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		d.AppID, d.Hostname, d.TargetService, d.TargetPort, d.Proxied, string(d.TLSMode),
		null(d.CFZoneID), null(d.CFRecordID), null(d.CFRecordIDAAAA), string(d.Status),
		null(string(d.ErrorCode)), null(d.LastError), null(d.CertIssuer), sqlitedb.NullMillis(d.CertExpiresAt),
		sqlitedb.Millis(d.CreatedAt), sqlitedb.Millis(d.UpdatedAt)).Scan(&id)
	if sqlitedb.IsUnique(err) {
		return service.ErrHostnameTaken
	}
	if err != nil {
		return err
	}
	d.ID = id
	return nil
}

func (r *Domains) Get(ctx context.Context, hostname string) (entity.Domain, error) {
	d, err := scanDomain(r.db.Conn(ctx).QueryRowContext(ctx, domainSelect+` WHERE hostname = ?`, hostname))
	return d, sqlitedb.NotFound(err)
}

func (r *Domains) ListByApp(ctx context.Context, appID int64) ([]entity.Domain, error) {
	return r.list(ctx, domainSelect+` WHERE app_id = ? ORDER BY hostname`, appID)
}

func (r *Domains) List(ctx context.Context) ([]entity.Domain, error) {
	return r.list(ctx, domainSelect+` ORDER BY hostname`)
}

func (r *Domains) list(ctx context.Context, q string, args ...any) ([]entity.Domain, error) {
	rows, err := r.db.Conn(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entity.Domain{}
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *Domains) Update(ctx context.Context, d entity.Domain) error {
	return affectedOne(r.db.Conn(ctx).ExecContext(ctx, `UPDATE domains SET target_service = ?, target_port = ?,
		proxied = ?, tls_mode = ?, cf_zone_id = ?, cf_record_id = ?, cf_record_id_aaaa = ?, status = ?,
		last_error_code = ?, last_error = ?, cert_issuer = ?, cert_expires_at = ?, updated_at = ?
		WHERE id = ?`,
		d.TargetService, d.TargetPort, d.Proxied, string(d.TLSMode),
		null(d.CFZoneID), null(d.CFRecordID), null(d.CFRecordIDAAAA), string(d.Status),
		null(string(d.ErrorCode)), null(d.LastError), null(d.CertIssuer), sqlitedb.NullMillis(d.CertExpiresAt),
		sqlitedb.Millis(d.UpdatedAt), d.ID))
}

func (r *Domains) Delete(ctx context.Context, id int64) error {
	return affectedOne(r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM domains WHERE id = ?`, id))
}

// scanner es lo común entre *sql.Row y *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanDomain(sc scanner) (entity.Domain, error) {
	var d entity.Domain
	var tls, status string
	var zone, rec, rec6, code, lastErr, issuer sql.NullString
	var expires sql.NullInt64
	var created, updated int64
	err := sc.Scan(&d.ID, &d.AppID, &d.Hostname, &d.TargetService, &d.TargetPort, &d.Proxied, &tls,
		&zone, &rec, &rec6, &status, &code, &lastErr, &issuer, &expires, &created, &updated)
	if err != nil {
		return entity.Domain{}, err
	}
	d.TLSMode, d.Status = entity.TLSMode(tls), entity.Status(status)
	d.CFZoneID, d.CFRecordID, d.CFRecordIDAAAA = zone.String, rec.String, rec6.String
	d.ErrorCode, d.LastError, d.CertIssuer = entity.ErrorCode(code.String), lastErr.String, issuer.String
	d.CertExpiresAt = sqlitedb.NullTime(expires)
	d.CreatedAt, d.UpdatedAt = sqlitedb.Time(created), sqlitedb.Time(updated)
	return d, nil
}

// null es "" como NULL.
func null(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

// affectedOne devuelve ErrNotFound si la sentencia no tocó ninguna fila.
func affectedOne(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sharedrepo.ErrNotFound
	}
	return nil
}
