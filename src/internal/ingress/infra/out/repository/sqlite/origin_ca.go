package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/repository"
	"github.com/CPU-commits/muelle/src/package/cipher"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// OriginCerts es la tabla origin_ca_certs (migración 0006). La clave va
// cifrada con entity.OriginKeyAAD(id): el id sale del INSERT, así que Insert
// guarda primero una clave vacía y la reemplaza en la misma transacción
// (como registries).
type OriginCerts struct {
	db     *sqlitedb.DB
	cipher cipher.Cipher
}

var _ repository.OriginCerts = (*OriginCerts)(nil)

func NewOriginCerts(db *sqlitedb.DB, c cipher.Cipher) *OriginCerts {
	return &OriginCerts{db: db, cipher: c}
}

const originCertCols = `id, hostname, zone_id, cf_cert_id, certificate, expires_at, state, created_at, retired_at,
	revoke_failures, next_revoke_at`

func (r *OriginCerts) Insert(ctx context.Context, c *entity.OriginCert) error {
	return r.db.InTx(ctx, func(ctx context.Context) error {
		if _, err := r.db.Conn(ctx).ExecContext(ctx, `UPDATE origin_ca_certs SET state = 'retired', retired_at = ?,
			private_key = x'' WHERE hostname = ? AND state = 'active'`, sqlitedb.Millis(c.CreatedAt), c.Hostname); err != nil {
			return err
		}
		id, err := r.insert(ctx, c, entity.OriginActive, nil)
		if err != nil {
			return err
		}
		ct := r.cipher.Encrypt([]byte(c.KeyPEM), entity.OriginKeyAAD(id))
		if _, err := r.db.Conn(ctx).ExecContext(ctx, `UPDATE origin_ca_certs SET private_key = ? WHERE id = ?`, ct, id); err != nil {
			return err
		}
		c.ID, c.State, c.RetiredAt = id, entity.OriginActive, nil
		return nil
	})
}

func (r *OriginCerts) InsertRetired(ctx context.Context, c *entity.OriginCert, at time.Time) error {
	id, err := r.insert(ctx, c, entity.OriginRetired, &at)
	if err != nil {
		return err
	}
	c.ID, c.State, c.RetiredAt, c.KeyPEM = id, entity.OriginRetired, &at, ""
	return nil
}

// insert guarda la fila con la clave vacía y devuelve su id.
func (r *OriginCerts) insert(ctx context.Context, c *entity.OriginCert, state entity.OriginCertState, retired *time.Time) (int64, error) {
	var id int64
	err := r.db.Conn(ctx).QueryRowContext(ctx, `INSERT INTO origin_ca_certs (hostname, zone_id, cf_cert_id, certificate,
		private_key, expires_at, state, created_at, retired_at) VALUES (?, ?, ?, ?, x'', ?, ?, ?, ?) RETURNING id`,
		c.Hostname, c.ZoneID, c.CFCertID, c.CertPEM, sqlitedb.Millis(c.ExpiresAt), string(state),
		sqlitedb.Millis(c.CreatedAt), sqlitedb.NullMillis(retired)).Scan(&id)
	return id, err
}

func (r *OriginCerts) Active(ctx context.Context) (map[string]entity.OriginCert, error) {
	list, err := r.list(ctx, `SELECT `+originCertCols+` FROM origin_ca_certs WHERE state = 'active' ORDER BY hostname`)
	if err != nil {
		return nil, err
	}
	out := make(map[string]entity.OriginCert, len(list))
	for _, c := range list {
		out[c.Hostname] = c
	}
	return out, nil
}

func (r *OriginCerts) ActiveWithKeys(ctx context.Context) (map[string]entity.OriginCert, error) {
	rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT `+originCertCols+`, private_key FROM origin_ca_certs
		WHERE state = 'active' ORDER BY hostname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]entity.OriginCert{}
	for rows.Next() {
		var ct []byte
		c, err := scanOriginCert(rows, &ct)
		if err != nil {
			return nil, err
		}
		pt, err := r.cipher.Decrypt(ct, entity.OriginKeyAAD(c.ID))
		if err != nil {
			return nil, fmt.Errorf("origin_ca_certs: descifrando la clave del certificado de %s: %w", c.Hostname, err)
		}
		c.KeyPEM = string(pt)
		out[c.Hostname] = c
	}
	return out, rows.Err()
}

func (r *OriginCerts) ActiveFor(ctx context.Context, hostname string) (*entity.OriginCert, error) {
	list, err := r.list(ctx, `SELECT `+originCertCols+` FROM origin_ca_certs WHERE state = 'active' AND hostname = ?`, hostname)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

func (r *OriginCerts) Retire(ctx context.Context, id int64, at time.Time) error {
	return affectedOne(r.db.Conn(ctx).ExecContext(ctx, `UPDATE origin_ca_certs SET state = 'retired', retired_at = ?,
		private_key = x'' WHERE id = ? AND state = 'active'`, sqlitedb.Millis(at), id))
}

func (r *OriginCerts) Retired(ctx context.Context) ([]entity.OriginCert, error) {
	return r.list(ctx, `SELECT `+originCertCols+` FROM origin_ca_certs WHERE state = 'retired' ORDER BY id`)
}

func (r *OriginCerts) RetiredDue(ctx context.Context, now, lastGoodBuild time.Time, grace time.Duration) ([]entity.OriginCert, error) {
	n := sqlitedb.Millis(now)
	lgb := int64(-1) // sin un Sync bueno todavía, ninguno se revoca
	if !lastGoodBuild.IsZero() {
		lgb = sqlitedb.Millis(lastGoodBuild)
	}
	return r.list(ctx, `SELECT `+originCertCols+` FROM origin_ca_certs WHERE state = 'retired' AND (expires_at < ?
		OR (retired_at < ? AND retired_at <= ? AND (next_revoke_at IS NULL OR next_revoke_at <= ?))) ORDER BY id`,
		n, lgb, sqlitedb.Millis(now.Add(-grace)), n)
}

func (r *OriginCerts) RevokeFailed(ctx context.Context, id int64, failures int, next time.Time) error {
	return affectedOne(r.db.Conn(ctx).ExecContext(ctx, `UPDATE origin_ca_certs SET revoke_failures = ?, next_revoke_at = ?
		WHERE id = ? AND state = 'retired'`, failures, sqlitedb.Millis(next), id))
}

func (r *OriginCerts) Delete(ctx context.Context, id int64) error {
	return affectedOne(r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM origin_ca_certs WHERE id = ?`, id))
}

func (r *OriginCerts) list(ctx context.Context, q string, args ...any) ([]entity.OriginCert, error) {
	rows, err := r.db.Conn(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entity.OriginCert{}
	for rows.Next() {
		c, err := scanOriginCert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// scanOriginCert lee las columnas de originCertCols y, después, las de
// extra (la clave cifrada en ActiveWithKeys).
func scanOriginCert(sc scanner, extra ...any) (entity.OriginCert, error) {
	var c entity.OriginCert
	var state string
	var expires, created int64
	var retired, nextRevoke sql.NullInt64
	dest := append([]any{&c.ID, &c.Hostname, &c.ZoneID, &c.CFCertID, &c.CertPEM, &expires, &state, &created, &retired,
		&c.RevokeFailures, &nextRevoke}, extra...)
	if err := sc.Scan(dest...); err != nil {
		return entity.OriginCert{}, err
	}
	c.State = entity.OriginCertState(state)
	c.ExpiresAt, c.CreatedAt = sqlitedb.Time(expires), sqlitedb.Time(created)
	c.RetiredAt, c.NextRevokeAt = sqlitedb.NullTime(retired), sqlitedb.NullTime(nextRevoke)
	return c, nil
}

// OriginHosts es la tabla origin_ca_hosts (migración 0006).
type OriginHosts struct{ db *sqlitedb.DB }

var _ repository.OriginHosts = (*OriginHosts)(nil)

func NewOriginHosts(db *sqlitedb.DB) *OriginHosts { return &OriginHosts{db: db} }

const originHostSelect = `SELECT hostname, observed, observed_at, failures, last_error_code, last_error, next_attempt_at,
	updated_at FROM origin_ca_hosts`

func (r *OriginHosts) All(ctx context.Context) (map[string]entity.OriginHost, error) {
	rows, err := r.db.Conn(ctx).QueryContext(ctx, originHostSelect+` ORDER BY hostname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]entity.OriginHost{}
	for rows.Next() {
		h, err := scanOriginHost(rows)
		if err != nil {
			return nil, err
		}
		out[h.Hostname] = h
	}
	return out, rows.Err()
}

func (r *OriginHosts) Get(ctx context.Context, hostname string) (entity.OriginHost, error) {
	h, err := scanOriginHost(r.db.Conn(ctx).QueryRowContext(ctx, originHostSelect+` WHERE hostname = ?`, hostname))
	if errors.Is(err, sql.ErrNoRows) {
		return entity.OriginHost{Hostname: hostname}, nil
	}
	return h, err
}

func (r *OriginHosts) SetObserved(ctx context.Context, hostname string, o entity.Observation, at time.Time) error {
	ms := sqlitedb.Millis(at)
	_, err := r.db.Conn(ctx).ExecContext(ctx, `INSERT INTO origin_ca_hosts (hostname, observed, observed_at, updated_at)
		VALUES (?, ?, ?, ?) ON CONFLICT (hostname) DO UPDATE SET observed = excluded.observed,
		observed_at = excluded.observed_at, updated_at = excluded.updated_at`, hostname, string(o), ms, ms)
	return err
}

func (r *OriginHosts) ForgetObserved(ctx context.Context, hostname string, at time.Time) error {
	_, err := r.db.Conn(ctx).ExecContext(ctx, `UPDATE origin_ca_hosts SET observed = NULL, observed_at = NULL, updated_at = ?
		WHERE hostname = ?`, sqlitedb.Millis(at), hostname)
	return err
}

func (r *OriginHosts) RecordFailure(ctx context.Context, hostname string, failures int, code entity.OriginErrorCode, cause string, next, at time.Time) error {
	_, err := r.db.Conn(ctx).ExecContext(ctx, `INSERT INTO origin_ca_hosts (hostname, failures, last_error_code, last_error,
		next_attempt_at, updated_at) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (hostname) DO UPDATE SET
		failures = excluded.failures, last_error_code = excluded.last_error_code, last_error = excluded.last_error,
		next_attempt_at = excluded.next_attempt_at, updated_at = excluded.updated_at`,
		hostname, failures, string(code), cause, sqlitedb.Millis(next), sqlitedb.Millis(at))
	return err
}

func (r *OriginHosts) ClearFailure(ctx context.Context, hostname string, at time.Time) error {
	_, err := r.db.Conn(ctx).ExecContext(ctx, `UPDATE origin_ca_hosts SET failures = 0, last_error_code = NULL,
		last_error = NULL, next_attempt_at = NULL, updated_at = ? WHERE hostname = ?`, sqlitedb.Millis(at), hostname)
	return err
}

func (r *OriginHosts) ResetBackoff(ctx context.Context, hostname string, at time.Time) error {
	ms := sqlitedb.Millis(at)
	_, err := r.db.Conn(ctx).ExecContext(ctx, `UPDATE origin_ca_hosts SET next_attempt_at = ?, updated_at = ?
		WHERE failures > 0 AND (? = '' OR hostname = ?)`, ms, ms, hostname, hostname)
	return err
}

func (r *OriginHosts) Delete(ctx context.Context, hostname string) error {
	_, err := r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM origin_ca_hosts WHERE hostname = ?`, hostname)
	return err
}

func (r *OriginHosts) DeleteAll(ctx context.Context) error {
	_, err := r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM origin_ca_hosts`)
	return err
}

func scanOriginHost(sc scanner) (entity.OriginHost, error) {
	var h entity.OriginHost
	var observed, code, lastErr sql.NullString
	var observedAt, next sql.NullInt64
	var updated int64
	if err := sc.Scan(&h.Hostname, &observed, &observedAt, &h.Failures, &code, &lastErr, &next, &updated); err != nil {
		return entity.OriginHost{}, err
	}
	h.Observed, h.ErrorCode, h.LastError = entity.Observation(observed.String), entity.OriginErrorCode(code.String), lastErr.String
	h.ObservedAt, h.NextAttemptAt, h.UpdatedAt = sqlitedb.NullTime(observedAt), sqlitedb.NullTime(next), sqlitedb.Time(updated)
	return h, nil
}
