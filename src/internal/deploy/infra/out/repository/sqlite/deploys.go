// Package sqlite implementa los repositorios de deploy (deploys y registries,
// migración 0002) sobre la base compartida.
package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// DeployRepository implementa repository.DeployRepository.
type DeployRepository struct {
	db *sqlitedb.DB
}

var _ repository.DeployRepository = (*DeployRepository)(nil)

func NewDeployRepository(db *sqlitedb.DB) *DeployRepository { return &DeployRepository{db: db} }

const deploySelect = `SELECT id, app_id, status, trigger, images, requested_images, config_images,
	actor_type, actor_id, actor_name, superseded_by, error, created_at, started_at, finished_at FROM deploys`

// encodeImages serializa un mapa servicio → ref con las claves ordenadas;
// nil es {}.
func encodeImages(m map[string]string) (string, error) {
	if m == nil {
		m = map[string]string{}
	}
	b, err := jsonv2.Marshal(m, jsonv2.Deterministic(true))
	return string(b), err
}

func decodeImages(column, s string) (map[string]string, error) {
	m := map[string]string{}
	if err := jsonv2.Unmarshal([]byte(s), &m); err != nil {
		return nil, fmt.Errorf("deploys.%s no es un JSON de servicio → ref: %w", column, err)
	}
	return m, nil
}

type scanner interface{ Scan(dest ...any) error }

func scanDeploy(s scanner) (entity.Deploy, error) {
	var d entity.Deploy
	var status, trigger, images, requested, config string
	var actorID, superseded, started, finished sql.NullInt64
	var errText sql.NullString
	var created int64
	if err := s.Scan(&d.ID, &d.AppID, &status, &trigger, &images, &requested, &config,
		&d.ActorType, &actorID, &d.ActorName, &superseded, &errText, &created, &started, &finished); err != nil {
		return entity.Deploy{}, err
	}
	d.Status, d.Trigger = entity.Status(status), entity.Trigger(trigger)
	var err error
	if d.Images, err = decodeImages("images", images); err != nil {
		return entity.Deploy{}, err
	}
	if d.RequestedImages, err = decodeImages("requested_images", requested); err != nil {
		return entity.Deploy{}, err
	}
	if d.ConfigImages, err = decodeImages("config_images", config); err != nil {
		return entity.Deploy{}, err
	}
	d.ActorID = nullInt(actorID)
	d.SupersededBy = nullInt(superseded)
	if errText.Valid {
		d.Error = &errText.String
	}
	d.CreatedAt = sqlitedb.Time(created)
	d.StartedAt = sqlitedb.NullTime(started)
	d.FinishedAt = sqlitedb.NullTime(finished)
	return d, nil
}

func nullInt(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func (r *DeployRepository) Create(ctx context.Context, d *entity.Deploy) error {
	images, err := encodeImages(d.Images)
	if err != nil {
		return err
	}
	var actorID sql.NullInt64
	if d.ActorID != nil {
		actorID = sql.NullInt64{Int64: *d.ActorID, Valid: true}
	}
	res, err := r.db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO deploys (app_id, status, trigger, images, actor_type, actor_id, actor_name, created_at)
		 VALUES (?, 'queued', ?, ?, ?, ?, ?, ?)`,
		d.AppID, string(d.Trigger), images, d.ActorType, actorID, d.ActorName, sqlitedb.Millis(d.CreatedAt))
	if sqlitedb.IsForeignKey(err) {
		// La única clave foránea que puede fallar en este INSERT es app_id
		// (superseded_by va NULL): la app se borró entre medio.
		return srepo.ErrNotFound
	}
	if err != nil {
		return err
	}
	if d.ID, err = res.LastInsertId(); err != nil {
		return err
	}
	d.Status = entity.StatusQueued
	if d.Images == nil {
		d.Images = map[string]string{}
	}
	if d.RequestedImages == nil {
		d.RequestedImages = map[string]string{}
	}
	if d.ConfigImages == nil {
		d.ConfigImages = map[string]string{}
	}
	return nil
}

// ids lee una columna de ids (un RETURNING id), ordenados; nunca nil.
func ids(rows *sql.Rows, err error) ([]int64, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.Sort(out) // RETURNING no garantiza un orden
	return out, nil
}

func (r *DeployRepository) SupersedeQueued(ctx context.Context, appID, by int64, at time.Time) ([]int64, error) {
	return ids(r.db.Conn(ctx).QueryContext(ctx,
		`UPDATE deploys SET status = 'superseded', superseded_by = ?, finished_at = ?
		 WHERE app_id = ? AND status = 'queued' AND id != ? RETURNING id`,
		by, sqlitedb.Millis(at), appID, by))
}

func (r *DeployRepository) Get(ctx context.Context, appID, id int64) (entity.Deploy, error) {
	d, err := scanDeploy(r.db.Conn(ctx).QueryRowContext(ctx, deploySelect+` WHERE id = ? AND app_id = ?`, id, appID))
	return d, sqlitedb.NotFound(err)
}

func (r *DeployRepository) GetByID(ctx context.Context, id int64) (entity.Deploy, error) {
	d, err := scanDeploy(r.db.Conn(ctx).QueryRowContext(ctx, deploySelect+` WHERE id = ?`, id))
	return d, sqlitedb.NotFound(err)
}

func (r *DeployRepository) List(ctx context.Context, appID int64) ([]entity.Deploy, error) {
	rows, err := r.db.Conn(ctx).QueryContext(ctx, deploySelect+` WHERE app_id = ? ORDER BY id DESC`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entity.Deploy{}
	for rows.Next() {
		d, err := scanDeploy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// one es el deploy de la consulta, o nil si no hay.
func (r *DeployRepository) one(ctx context.Context, where string, args ...any) (*entity.Deploy, error) {
	d, err := scanDeploy(r.db.Conn(ctx).QueryRowContext(ctx, deploySelect+` WHERE `+where, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DeployRepository) Current(ctx context.Context, appID int64) (*entity.Deploy, error) {
	return r.one(ctx, `app_id = ? AND status = 'succeeded' ORDER BY id DESC LIMIT 1`, appID)
}

func (r *DeployRepository) PreviousSucceeded(ctx context.Context, appID, before int64) (*entity.Deploy, error) {
	return r.one(ctx, `app_id = ? AND status = 'succeeded' AND id < ? ORDER BY id DESC LIMIT 1`, appID, before)
}

func (r *DeployRepository) NextQueued(ctx context.Context, appID int64) (*entity.Deploy, error) {
	return r.one(ctx, `app_id = ? AND status = 'queued' ORDER BY id LIMIT 1`, appID)
}

func (r *DeployRepository) Running(ctx context.Context, appID int64) (*entity.Deploy, error) {
	return r.one(ctx, `app_id = ? AND status = 'running' ORDER BY id LIMIT 1`, appID)
}

func (r *DeployRepository) ActiveApps(ctx context.Context) ([]int64, error) {
	return ids(r.db.Conn(ctx).QueryContext(ctx,
		`SELECT DISTINCT app_id FROM deploys WHERE status IN ('queued', 'running')`))
}

func (r *DeployRepository) Claim(ctx context.Context, id int64, at time.Time) (bool, error) {
	res, err := r.db.Conn(ctx).ExecContext(ctx,
		`UPDATE deploys SET status = 'running', started_at = ? WHERE id = ? AND status = 'queued'`,
		sqlitedb.Millis(at), id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (r *DeployRepository) SetImages(ctx context.Context, id int64, requested, config, resolved map[string]string) error {
	var cols [3]string
	for i, m := range []map[string]string{requested, config, resolved} {
		s, err := encodeImages(m)
		if err != nil {
			return err
		}
		cols[i] = s
	}
	res, err := r.db.Conn(ctx).ExecContext(ctx,
		`UPDATE deploys SET requested_images = ?, config_images = ?, images = ? WHERE id = ?`,
		cols[0], cols[1], cols[2], id)
	return affected(res, err)
}

// affected es ErrNotFound si la sentencia no tocó ninguna fila.
func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return srepo.ErrNotFound
	}
	return nil
}

// AppendLog lee el largo en bytes (CAST AS BLOB: en un TEXT, length cuenta
// caracteres) y la marca, decide con AppendPlan y guarda los dos, en una
// transacción (la del ctx si trae una).
func (r *DeployRepository) AppendLog(ctx context.Context, id int64, lines []string, final bool, marker string) error {
	return r.db.InTx(ctx, func(ctx context.Context) error {
		var cur int
		var truncated bool
		err := r.db.Conn(ctx).QueryRowContext(ctx,
			`SELECT length(CAST(log AS BLOB)), log_truncated FROM deploys WHERE id = ?`, id).Scan(&cur, &truncated)
		if err != nil {
			return sqlitedb.NotFound(err)
		}
		add, nowTruncated := service.AppendPlan(cur, truncated, lines, final, marker)
		if add == "" && nowTruncated == truncated {
			return nil
		}
		_, err = r.db.Conn(ctx).ExecContext(ctx,
			`UPDATE deploys SET log = log || ?, log_truncated = ? WHERE id = ?`, add, nowTruncated, id)
		return err
	})
}

// ReadLog lee por bytes con substr sobre CAST(log AS BLOB) y corta el pedazo
// en su último \n.
func (r *DeployRepository) ReadLog(ctx context.Context, appID, id int64, offset, max int) (repository.LogChunk, error) {
	var c repository.LogChunk
	var status string
	var superseded sql.NullInt64
	err := r.db.Conn(ctx).QueryRowContext(ctx,
		`SELECT substr(CAST(log AS BLOB), ? + 1, ?), length(CAST(log AS BLOB)), status, superseded_by
		 FROM deploys WHERE id = ? AND app_id = ?`, offset, max, id, appID).Scan(&c.Chunk, &c.Length, &status, &superseded)
	if err != nil {
		return repository.LogChunk{}, sqlitedb.NotFound(err)
	}
	if offset > c.Length {
		return repository.LogChunk{}, service.ErrOffsetBeyond
	}
	c.Status = entity.Status(status)
	c.SupersededBy = nullInt(superseded)
	if i := bytes.LastIndexByte(c.Chunk, '\n'); i >= 0 {
		c.Chunk = c.Chunk[:i+1]
	} else {
		c.Chunk = []byte{}
	}
	return c, nil
}

func (r *DeployRepository) Finish(ctx context.Context, id int64, status entity.Status, errMsg *string, at time.Time) error {
	if !status.Final() {
		return fmt.Errorf("deploy: Finish con el estado %q, que no es final", status)
	}
	var e sql.NullString
	if errMsg != nil {
		e = sql.NullString{String: *errMsg, Valid: true}
	}
	res, err := r.db.Conn(ctx).ExecContext(ctx,
		`UPDATE deploys SET status = ?, error = ?, finished_at = ? WHERE id = ? AND status IN ('queued', 'running')`,
		string(status), e, sqlitedb.Millis(at), id)
	return affected(res, err)
}

func (r *DeployRepository) FailQueued(ctx context.Context, appID int64, reason string, at time.Time) ([]int64, error) {
	return ids(r.db.Conn(ctx).QueryContext(ctx,
		`UPDATE deploys SET status = 'failed', error = ?, finished_at = ?
		 WHERE app_id = ? AND status = 'queued' RETURNING id`,
		reason, sqlitedb.Millis(at), appID))
}

// retentionKeep son los deploys más nuevos que guarda la retención (contrato §2.3).
const retentionKeep = 20

func (r *DeployRepository) ApplyRetention(ctx context.Context, appID int64) (int, error) {
	// El actual y el succeeded anterior son los dos succeeded de id más alto.
	res, err := r.db.Conn(ctx).ExecContext(ctx,
		`DELETE FROM deploys WHERE app_id = ?1 AND status NOT IN ('queued', 'running')
		 AND id NOT IN (SELECT id FROM deploys WHERE app_id = ?1 ORDER BY id DESC LIMIT ?2)
		 AND id NOT IN (SELECT id FROM deploys WHERE app_id = ?1 AND status = 'succeeded' ORDER BY id DESC LIMIT 2)`,
		appID, retentionKeep)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}
