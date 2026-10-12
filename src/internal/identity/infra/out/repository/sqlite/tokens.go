package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

type TokenRepository struct {
	db *sqlitedb.DB
}

var _ repository.TokenRepository = (*TokenRepository)(nil)

func NewTokenRepository(db *sqlitedb.DB) *TokenRepository { return &TokenRepository{db: db} }

// tokenSelect lee los slugs del alcance de las tablas de catalog (solo
// lectura) y el username de quien lo creó (NULL si se borró: created_by es
// ON DELETE SET NULL, el token no se borra con su creador). Las claves
// foráneas ON DELETE CASCADE borran un token con su proyecto o su stage, pero
// si la base quedó inconsistente (por ejemplo, un
// borrado con foreign_keys=OFF) el token no puede ampliar su alcance: sin su
// proyecto no aparece (JOIN), y con un stage que no existe o es de otro
// proyecto el slug llega NULL y scanToken lo rechaza (errBrokenScope).
const tokenSelect = `SELECT t.id, t.name, t.token_hash, t.project_id, p.slug, t.stage_id, s.slug,
	t.created_by, u.username, t.created_at, t.last_used_at, t.expires_at
	FROM api_tokens t JOIN projects p ON p.id = t.project_id
	LEFT JOIN stages s ON s.id = t.stage_id AND s.project_id = t.project_id
	LEFT JOIN users u ON u.id = t.created_by`

// errBrokenScope: el token tiene stage, pero el stage no existe o es de otro
// proyecto. Sin su slug, Principal.Allows lo trataría como de todo el proyecto.
var errBrokenScope = errors.New("identity: token con un stage que no existe")

// scopeNotFound traduce un token inexistente o con el alcance roto a ErrNotFound.
func scopeNotFound(err error) error {
	if errors.Is(err, errBrokenScope) {
		return sharedrepo.ErrNotFound
	}
	return sqlitedb.NotFound(err)
}

func (r *TokenRepository) Create(ctx context.Context, t entity.APIToken) (entity.APIToken, error) {
	res, err := r.db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO api_tokens (name, token_hash, project_id, stage_id, created_by, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.Name, t.TokenHash, t.ProjectID, nullInt(t.StageID), nullInt(t.CreatedBy),
		sqlitedb.Millis(t.CreatedAt), sqlitedb.NullMillis(t.ExpiresAt))
	switch {
	case sqlitedb.IsUnique(err):
		return entity.APIToken{}, sharedrepo.ErrConflict
	case sqlitedb.IsForeignKey(err):
		return entity.APIToken{}, sharedrepo.ErrNotFound
	case err != nil:
		return entity.APIToken{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return entity.APIToken{}, err
	}
	saved, err := scanToken(r.db.Conn(ctx).QueryRowContext(ctx, tokenSelect+` WHERE t.id = ?`, id))
	return saved, scopeNotFound(err)
}

func (r *TokenRepository) GetByHash(ctx context.Context, tokenHash string) (entity.APIToken, error) {
	t, err := scanToken(r.db.Conn(ctx).QueryRowContext(ctx, tokenSelect+` WHERE t.token_hash = ?`, tokenHash))
	return t, scopeNotFound(err)
}

func (r *TokenRepository) List(ctx context.Context) ([]entity.APIToken, error) {
	rows, err := r.db.Conn(ctx).QueryContext(ctx, tokenSelect+` ORDER BY t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []entity.APIToken{}
	for rows.Next() {
		t, err := scanToken(rows)
		if errors.Is(err, errBrokenScope) {
			continue // no sirve para autenticar, y mostrarlo sin stage engañaría
		}
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *TokenRepository) Delete(ctx context.Context, id int64) error {
	return affectedOne(r.db.Conn(ctx).ExecContext(ctx, `DELETE FROM api_tokens WHERE id = ?`, id))
}

func (r *TokenRepository) Touch(ctx context.Context, id int64, at time.Time) error {
	_, err := r.db.Conn(ctx).ExecContext(ctx,
		`UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, sqlitedb.Millis(at), id)
	return err
}

func scanToken(sc scanner) (entity.APIToken, error) {
	var t entity.APIToken
	var stageID, createdBy, used, expires sql.NullInt64
	var stageSlug, createdByName sql.NullString
	var created int64
	err := sc.Scan(&t.ID, &t.Name, &t.TokenHash, &t.ProjectID, &t.ProjectSlug, &stageID, &stageSlug,
		&createdBy, &createdByName, &created, &used, &expires)
	if err != nil {
		return entity.APIToken{}, err
	}
	if stageID.Valid && !stageSlug.Valid {
		return entity.APIToken{}, errBrokenScope
	}
	t.StageSlug = stageSlug.String
	t.StageID = fromNullInt(stageID)
	t.CreatedBy = fromNullInt(createdBy)
	t.CreatedByName = createdByName.String
	t.CreatedAt = sqlitedb.Time(created)
	t.LastUsedAt = sqlitedb.NullTime(used)
	t.ExpiresAt = sqlitedb.NullTime(expires)
	return t, nil
}
