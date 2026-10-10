package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
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
	return reposOn(sqlitetest.New(t))
}

func reposOn(db *sqlitedb.DB) repos {
	return repos{db: db, users: NewUserRepository(db), sessions: NewSessionRepository(db),
		invites: NewInviteRepository(db), tokens: NewTokenRepository(db)}
}

// newReposAt abre la base en una ruta conocida, para poder corromperla desde
// otra conexión (rawExec).
func newReposAt(t *testing.T) (repos, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "muelle.db")
	db, err := sqlitedb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return reposOn(db), path
}

// rawExec corre SQL en una conexión aparte sin claves foráneas (en SQLite
// vienen apagadas si no se activan), para dejar filas huérfanas como las que
// quedarían si alguien borra con foreign_keys=OFF.
func rawExec(t *testing.T, path, query string, args ...any) {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
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
