package identity

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	sqliterepo "github.com/CPU-commits/muelle/src/internal/identity/infra/out/repository/sqlite"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

func newModule(t *testing.T, c clock.Clock) (*Module, *sqlitedb.DB) {
	t.Helper()
	db := sqlitetest.New(t)
	ciph, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(Deps{DB: db, Cipher: ciph, Clock: c, Audit: sqlitedb.NewAuditRecorder(db, clock.System{}, log), Log: log}), db
}

func TestEnsureSetupInvite(t *testing.T) {
	m, db := newModule(t, nil) // sin Clock usa el reloj del sistema
	ctx := context.Background()
	token, created, err := m.EnsureSetupInvite(ctx)
	if err != nil || !created || len(token) != 43 {
		t.Fatalf("token=%q created=%v err=%v", token, created, err)
	}
	sqliterepo.NewUserRepository(db).Create(ctx, entity.User{Username: "felipe", PasswordHash: "h", CreatedAt: time.Now()})
	if token, created, err := m.EnsureSetupInvite(ctx); err != nil || created || token != "" {
		t.Fatalf("con usuarios: token=%q created=%v err=%v", token, created, err)
	}
}

func TestJobsCleanupSessions(t *testing.T) {
	c := clock.NewManual(time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC))
	m, db := newModule(t, c)
	ctx := context.Background()
	u, _ := sqliterepo.NewUserRepository(db).Create(ctx, entity.User{Username: "felipe", PasswordHash: "h", CreatedAt: c.Now()})
	sessions := sqliterepo.NewSessionRepository(db)
	sessions.Create(ctx, entity.NewSession("s1", u.ID, false, c.Now(), service.SessionMax, "", ""))

	jobs := m.Jobs()
	if len(jobs) != 1 || jobs[0].Name != "sesiones vencidas" || jobs[0].Every != time.Hour || jobs[0].Run == nil {
		t.Fatalf("jobs=%+v", jobs)
	}
	if err := jobs[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Get(ctx, "s1"); err != nil {
		t.Fatalf("una sesión vigente no se borra: %v", err)
	}
	c.Advance(service.SessionIdle + time.Second)
	if err := jobs[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Get(ctx, "s1"); !errors.Is(err, sharedrepo.ErrNotFound) {
		t.Fatalf("la sesión sin uso debió borrarse, got %v", err)
	}
}

// El Authenticator del módulo es el que usa cmd/http para todos los módulos:
// un token siempre trae su alcance.
func TestAuthenticator(t *testing.T) {
	m, db := newModule(t, clock.NewManual(time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)))
	ctx := context.Background()
	var a utils.Authenticator = m.Authenticator()

	if _, err := a.AuthenticateSession(ctx, "nope"); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("got %v", err)
	}
	if _, err := a.AuthenticateToken(ctx, "mk_nope"); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("got %v", err)
	}

	res, _ := db.Conn(ctx).ExecContext(ctx, `INSERT INTO projects (slug, name, created_at) VALUES ('auditfy', 'Auditfy', 0)`)
	projectID, _ := res.LastInsertId()
	plain, hash := service.RandomToken(service.APITokenPrefix)
	if _, err := sqliterepo.NewTokenRepository(db).Create(ctx, entity.APIToken{Name: "ci", TokenHash: hash, ProjectID: projectID, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	p, err := a.AuthenticateToken(ctx, plain)
	if err != nil || p.Token == nil || p.Token.ProjectSlug != "auditfy" || p.Token.StageSlug != "" || p.Token.Name != "ci" {
		t.Fatalf("p=%+v err=%v", p, err)
	}
}
