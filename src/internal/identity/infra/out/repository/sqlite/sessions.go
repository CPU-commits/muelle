package sqlite

import (
	"context"
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

type SessionRepository struct {
	db *sqlitedb.DB
}

var _ repository.SessionRepository = (*SessionRepository)(nil)

func NewSessionRepository(db *sqlitedb.DB) *SessionRepository { return &SessionRepository{db: db} }

func (r *SessionRepository) Create(ctx context.Context, s entity.Session) error {
	_, err := r.db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO sessions (id_hash, user_id, mfa_pending, created_at, last_seen_at, expires_at, user_agent, ip)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.IDHash, s.UserID, s.MFAPending, sqlitedb.Millis(s.CreatedAt), sqlitedb.Millis(s.LastSeenAt),
		sqlitedb.Millis(s.ExpiresAt), s.UserAgent, s.IP)
	if sqlitedb.IsUnique(err) {
		return sharedrepo.ErrConflict
	}
	return err
}

func (r *SessionRepository) Get(ctx context.Context, idHash string) (entity.Session, error) {
	var s entity.Session
	var created, seen, expires int64
	err := r.db.Conn(ctx).QueryRowContext(ctx,
		`SELECT id_hash, user_id, mfa_pending, created_at, last_seen_at, expires_at, user_agent, ip
		 FROM sessions WHERE id_hash = ?`, idHash).
		Scan(&s.IDHash, &s.UserID, &s.MFAPending, &created, &seen, &expires, &s.UserAgent, &s.IP)
	if err != nil {
		return entity.Session{}, notFound(err)
	}
	s.CreatedAt, s.LastSeenAt, s.ExpiresAt = sqlitedb.Time(created), sqlitedb.Time(seen), sqlitedb.Time(expires)
	return s, nil
}

func (r *SessionRepository) Touch(ctx context.Context, idHash string, at time.Time) error {
	_, err := r.db.Conn(ctx).ExecContext(ctx,
		`UPDATE sessions SET last_seen_at = ? WHERE id_hash = ?`, sqlitedb.Millis(at), idHash)
	return err
}

func (r *SessionRepository) Delete(ctx context.Context, idHash string) error {
	_, err := r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM sessions WHERE id_hash = ?`, idHash)
	return err
}

func (r *SessionRepository) DeleteExpired(ctx context.Context, now time.Time, idle time.Duration) (int64, error) {
	res, err := r.db.Conn(ctx).ExecContext(ctx,
		`DELETE FROM sessions WHERE expires_at <= ? OR (mfa_pending = 0 AND last_seen_at < ?)`,
		sqlitedb.Millis(now), sqlitedb.Millis(now.Add(-idle)))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
