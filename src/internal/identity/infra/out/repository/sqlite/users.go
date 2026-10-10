package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

type UserRepository struct {
	db *sqlitedb.DB
}

var _ repository.UserRepository = (*UserRepository)(nil)

func NewUserRepository(db *sqlitedb.DB) *UserRepository { return &UserRepository{db: db} }

const userSelect = `SELECT id, username, password_hash, totp_secret, totp_pending, created_by, created_at,
	failed_logins, locked_until FROM users`

func (r *UserRepository) Create(ctx context.Context, u entity.User) (entity.User, error) {
	res, err := r.db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO users (username, password_hash, created_by, created_at) VALUES (?, ?, ?, ?)`,
		u.Username, u.PasswordHash, nullInt(u.CreatedBy), sqlitedb.Millis(u.CreatedAt))
	if sqlitedb.IsUnique(err) {
		return entity.User{}, sharedrepo.ErrConflict
	}
	if err != nil {
		return entity.User{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return entity.User{}, err
	}
	return r.GetByID(ctx, id)
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (entity.User, error) {
	u, err := scanUser(r.db.Conn(ctx).QueryRowContext(ctx, userSelect+` WHERE id = ?`, id))
	return u, notFound(err)
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (entity.User, error) {
	u, err := scanUser(r.db.Conn(ctx).QueryRowContext(ctx, userSelect+` WHERE username = ?`, username))
	return u, notFound(err)
}

func (r *UserRepository) List(ctx context.Context) ([]entity.User, error) {
	rows, err := r.db.Conn(ctx).QueryContext(ctx, userSelect+` ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entity.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r *UserRepository) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// Delete borra el usuario; sus sesiones e invitaciones se borran en cascada.
func (r *UserRepository) Delete(ctx context.Context, id int64) error {
	return affectedOne(r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id))
}

func (r *UserRepository) IncrementFailedLogins(ctx context.Context, id int64) (int, error) {
	var n int
	err := r.db.Conn(ctx).QueryRowContext(ctx,
		`UPDATE users SET failed_logins = failed_logins + 1 WHERE id = ? RETURNING failed_logins`, id).Scan(&n)
	return n, notFound(err)
}

func (r *UserRepository) SetLockedUntil(ctx context.Context, id int64, until time.Time) error {
	return affectedOne(r.db.Conn(ctx).ExecContext(ctx,
		`UPDATE users SET locked_until = ? WHERE id = ?`, sqlitedb.Millis(until), id))
}

func (r *UserRepository) ResetLoginFailures(ctx context.Context, id int64) error {
	return affectedOne(r.db.Conn(ctx).ExecContext(ctx,
		`UPDATE users SET failed_logins = 0, locked_until = NULL WHERE id = ?`, id))
}

func (r *UserRepository) SetTOTPPending(ctx context.Context, id int64, encrypted []byte) error {
	return affectedOne(r.db.Conn(ctx).ExecContext(ctx,
		`UPDATE users SET totp_pending = ? WHERE id = ?`, encrypted, id))
}

func (r *UserRepository) ActivateTOTP(ctx context.Context, id int64, encrypted []byte) error {
	return affectedOne(r.db.Conn(ctx).ExecContext(ctx,
		`UPDATE users SET totp_secret = ?, totp_pending = NULL WHERE id = ? AND totp_pending IS NOT NULL`, encrypted, id))
}

func (r *UserRepository) ClearTOTP(ctx context.Context, id int64) error {
	return affectedOne(r.db.Conn(ctx).ExecContext(ctx,
		`UPDATE users SET totp_secret = NULL, totp_pending = NULL WHERE id = ?`, id))
}

func scanUser(sc scanner) (entity.User, error) {
	var u entity.User
	var createdBy, locked sql.NullInt64
	var created int64
	err := sc.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.TOTPSecret, &u.TOTPPending, &createdBy, &created,
		&u.FailedLogins, &locked)
	if err != nil {
		return entity.User{}, err
	}
	u.CreatedBy = fromNullInt(createdBy)
	u.CreatedAt = sqlitedb.Time(created)
	u.LockedUntil = sqlitedb.NullTime(locked)
	return u, nil
}
