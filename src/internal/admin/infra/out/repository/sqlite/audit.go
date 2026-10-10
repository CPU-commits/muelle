package sqlite

import (
	"context"
	"database/sql"
	"encoding/json/jsontext"
	"time"

	"github.com/CPU-commits/muelle/src/internal/admin/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

type AuditRepository struct {
	db *sqlite.DB
}

var _ repository.AuditRepository = (*AuditRepository)(nil)

func NewAuditRepository(db *sqlite.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) List(ctx context.Context, beforeID int64, limit int) ([]entity.AuditEvent, error) {
	query := `SELECT id, actor_type, actor_id, action, target, details, at FROM audit_events`
	args := []any{}
	if beforeID > 0 {
		query += ` WHERE id < ?`
		args = append(args, beforeID)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.Conn(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entity.AuditEvent{}
	for rows.Next() {
		var e entity.AuditEvent
		var actor sql.NullInt64
		var details string
		var at int64
		if err := rows.Scan(&e.ID, &e.ActorType, &actor, &e.Action, &e.Target, &details, &at); err != nil {
			return nil, err
		}
		if actor.Valid {
			id := actor.Int64
			e.ActorID = &id
		}
		e.Details = jsontext.Value(details)
		e.At = sqlite.Time(at)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *AuditRepository) DeleteBefore(ctx context.Context, t time.Time) (int64, error) {
	res, err := r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM audit_events WHERE at < ?`, sqlite.Millis(t))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
