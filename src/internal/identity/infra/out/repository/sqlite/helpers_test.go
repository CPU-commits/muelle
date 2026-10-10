package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

var t0 = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

type repos struct {
	db       *sqlitedb.DB
	users    *UserRepository
	sessions *SessionRepository
	invites  *InviteRepository
	tokens   *TokenRepository
}

func newRepos(t *testing.T) repos {
	t.Helper()
	db := sqlitetest.New(t)
	return repos{db: db, users: NewUserRepository(db), sessions: NewSessionRepository(db),
		invites: NewInviteRepository(db), tokens: NewTokenRepository(db)}
}

func (r repos) user(t *testing.T, name string, createdBy *int64) entity.User {
	t.Helper()
	u, err := r.users.Create(context.Background(), entity.User{Username: name, PasswordHash: "h", CreatedBy: createdBy, CreatedAt: t0})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// exec corre SQL directo: identity no puede importar catalog, así que los
// proyectos y stages de los tests se crean a mano.
func exec(t *testing.T, db *sqlitedb.DB, query string, args ...any) int64 {
	t.Helper()
	res, err := db.Conn(context.Background()).ExecContext(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// seedStage crea el proyecto auditfy con el stage staging y devuelve sus ids.
func seedStage(t *testing.T, db *sqlitedb.DB) (projectID, stageID int64) {
	t.Helper()
	projectID = exec(t, db, `INSERT INTO projects (slug, name, created_at) VALUES ('auditfy', 'Auditfy', 0)`)
	stageID = exec(t, db, `INSERT INTO stages (project_id, slug, created_at) VALUES (?, 'staging', 0)`, projectID)
	return projectID, stageID
}
