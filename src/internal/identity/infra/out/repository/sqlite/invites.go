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

type InviteRepository struct {
	db *sqlitedb.DB
}

var _ repository.InviteRepository = (*InviteRepository)(nil)

func NewInviteRepository(db *sqlitedb.DB) *InviteRepository { return &InviteRepository{db: db} }

func (r *InviteRepository) Create(ctx context.Context, inv entity.Invite) error {
	_, err := r.db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO invites (token_hash, created_by, expires_at, used_at) VALUES (?, ?, ?, ?)`,
		inv.TokenHash, nullInt(inv.CreatedBy), sqlitedb.Millis(inv.ExpiresAt), sqlitedb.NullMillis(inv.UsedAt))
	if sqlitedb.IsUnique(err) {
		return sharedrepo.ErrConflict
	}
	return err
}

func (r *InviteRepository) GetByTokenHash(ctx context.Context, tokenHash string) (entity.Invite, error) {
	var inv entity.Invite
	var createdBy, used sql.NullInt64
	var expires int64
	err := r.db.Conn(ctx).QueryRowContext(ctx,
		`SELECT id, token_hash, created_by, expires_at, used_at FROM invites WHERE token_hash = ?`, tokenHash).
		Scan(&inv.ID, &inv.TokenHash, &createdBy, &expires, &used)
	if err != nil {
		return entity.Invite{}, notFound(err)
	}
	inv.CreatedBy = fromNullInt(createdBy)
	inv.ExpiresAt = sqlitedb.Time(expires)
	inv.UsedAt = sqlitedb.NullTime(used)
	return inv, nil
}

func (r *InviteRepository) MarkUsed(ctx context.Context, id int64, at time.Time) error {
	return affectedOne(r.db.Conn(ctx).ExecContext(ctx,
		`UPDATE invites SET used_at = ? WHERE id = ? AND used_at IS NULL`, sqlitedb.Millis(at), id))
}

func (r *InviteRepository) DeleteUnusedSystem(ctx context.Context) error {
	_, err := r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM invites WHERE created_by IS NULL AND used_at IS NULL`)
	return err
}
