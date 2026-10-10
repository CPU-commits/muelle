package usecases

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	sqliterepo "github.com/CPU-commits/muelle/src/internal/identity/infra/out/repository/sqlite"
	outservice "github.com/CPU-commits/muelle/src/internal/identity/infra/out/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

const testPassword = "una-contraseña-larga"

// fixture arma todos los casos de uso con repositorios reales, un reloj
// manual y cifrado de verdad.
type fixture struct {
	db       *sqlitedb.DB
	path     string
	clock    *clock.Manual
	cipher   *crypto.Cipher
	users    *sqliterepo.UserRepository
	sessions *sqliterepo.SessionRepository
	invites  *sqliterepo.InviteRepository
	tokens   *sqliterepo.TokenRepository
	hasher   *outservice.Argon2Hasher
	totp     outservice.TOTP
	rec      *sqlitedb.AuditRecorder
	limiter  *service.LoginLimiter
	lockout  *service.LoginLockout
	replay   *service.TOTPReplayGuard

	login        *Login
	completeTOTP *CompleteTOTP
	logout       *Logout
	authSession  *AuthenticateSession
	authToken    *AuthenticateToken
	setup        *Setup
	acceptInvite *AcceptInvite
	ensureSetup  *EnsureSetupInvite
	createInvite *CreateInvite
	me           *Me
	beginTOTP    *BeginTOTPSetup
	enableTOTP   *EnableTOTP
	listUsers    *ListUsers
	deleteUser   *DeleteUser
	resetTOTP    *ResetUserTOTP
	createToken  *CreateToken
	listTokens   *ListTokens
	deleteToken  *DeleteToken
	cleanup      *CleanupSessions
}

func newFixture(t *testing.T) *fixture {
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
	c := clock.NewManual(time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC))
	ciph, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{
		db: db, path: path, clock: c, cipher: ciph,
		users: sqliterepo.NewUserRepository(db), sessions: sqliterepo.NewSessionRepository(db),
		invites: sqliterepo.NewInviteRepository(db), tokens: sqliterepo.NewTokenRepository(db),
		hasher: outservice.NewArgon2Hasher(), totp: outservice.NewTOTP(),
	}
	rec := sqlitedb.NewAuditRecorder(db, c, slog.New(slog.NewTextHandler(io.Discard, nil)))
	settings := outservice.NewSettingsReader(db)
	scopes := outservice.NewScopeResolver(db)
	limiter := service.NewLoginLimiter(service.LoginMaxPerIP, service.LoginWindow, c)

	lockout := service.NewLoginLockout(c)
	replay := service.NewTOTPReplayGuard(c)
	f.rec, f.limiter, f.lockout, f.replay = rec, limiter, lockout, replay
	f.login = NewLogin(db, f.users, f.sessions, f.hasher, limiter, lockout, c)
	f.completeTOTP = NewCompleteTOTP(db, f.users, f.sessions, ciph, f.totp, limiter, lockout, replay, c)
	f.logout = NewLogout(f.sessions)
	f.authSession = NewAuthenticateSession(f.users, f.sessions, settings, c)
	f.authToken = NewAuthenticateToken(f.tokens, c)
	f.setup = NewSetup(db, f.users, f.invites, f.sessions, f.hasher, rec, limiter, c)
	f.acceptInvite = NewAcceptInvite(db, f.users, f.invites, f.sessions, f.hasher, rec, limiter, c)
	f.ensureSetup = NewEnsureSetupInvite(db, f.users, f.invites, c)
	f.createInvite = NewCreateInvite(db, f.users, f.invites, rec, c)
	f.me = NewMe(f.users)
	f.beginTOTP = NewBeginTOTPSetup(f.users, ciph, f.totp)
	f.enableTOTP = NewEnableTOTP(db, f.users, ciph, f.totp, replay, rec, c)
	f.listUsers = NewListUsers(f.users)
	f.deleteUser = NewDeleteUser(db, f.users, rec)
	f.resetTOTP = NewResetUserTOTP(f.users, rec)
	f.createToken = NewCreateToken(db, f.users, f.tokens, scopes, rec, c)
	f.listTokens = NewListTokens(f.tokens)
	f.deleteToken = NewDeleteToken(f.tokens, rec)
	f.cleanup = NewCleanupSessions(f.sessions, c)
	return f
}

func (f *fixture) advance(d time.Duration) { f.clock.Advance(d) }

func (f *fixture) now() time.Time { return f.clock.Now() }

// user crea un usuario directo en el repositorio, con testPassword.
func (f *fixture) user(t *testing.T, name string) entity.User {
	t.Helper()
	h, err := f.hasher.Hash(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	u, err := f.users.Create(context.Background(), entity.User{Username: name, PasswordHash: h, CreatedAt: f.now()})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func (f *fixture) reload(t *testing.T, id int64) entity.User {
	t.Helper()
	u, err := f.users.GetByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// exec corre SQL directo: los proyectos, stages y settings son de otros
// módulos, así que los tests los crean a mano.
func (f *fixture) exec(t *testing.T, query string, args ...any) int64 {
	t.Helper()
	res, err := f.db.Conn(context.Background()).ExecContext(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// rawExec corre SQL en una conexión aparte sin claves foráneas, para dejar
// filas huérfanas como las de un borrado con foreign_keys=OFF.
func (f *fixture) rawExec(t *testing.T, query string, args ...any) {
	t.Helper()
	raw, err := sql.Open("sqlite", "file:"+f.path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func (f *fixture) project(t *testing.T, slug string) int64 {
	t.Helper()
	return f.exec(t, `INSERT INTO projects (slug, name, created_at) VALUES (?, ?, 0)`, slug, slug)
}

func (f *fixture) stage(t *testing.T, projectID int64, slug string) int64 {
	t.Helper()
	return f.exec(t, `INSERT INTO stages (project_id, slug, created_at) VALUES (?, ?, 0)`, projectID, slug)
}

func (f *fixture) setRequire2FA(t *testing.T, on bool) {
	t.Helper()
	f.exec(t, `INSERT INTO settings (key, value) VALUES ('require_2fa', ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, fmt.Sprint(on))
}

type auditRow struct {
	ActorType string
	ActorID   *int64
	Action    string
	Target    string
	Details   string
}

func (f *fixture) auditRows(t *testing.T) []auditRow {
	t.Helper()
	rows, err := f.db.Conn(context.Background()).QueryContext(context.Background(),
		`SELECT actor_type, actor_id, action, target, details FROM audit_events ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []auditRow
	for rows.Next() {
		var r auditRow
		var id sql.NullInt64
		if err := rows.Scan(&r.ActorType, &id, &r.Action, &r.Target, &r.Details); err != nil {
			t.Fatal(err)
		}
		if id.Valid {
			r.ActorID = &id.Int64
		}
		out = append(out, r)
	}
	return out
}

// code devuelve el código TOTP válido ahora.
func (f *fixture) code(t *testing.T, secret string) string {
	t.Helper()
	c, err := totp.GenerateCode(secret, f.now())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// wrongCode devuelve un código que no es válido ahora (ni en los períodos vecinos).
func (f *fixture) wrongCode(t *testing.T, secret string) string {
	t.Helper()
	valid := map[string]bool{}
	for _, d := range []time.Duration{-30 * time.Second, 0, 30 * time.Second} {
		c, _ := totp.GenerateCode(secret, f.now().Add(d))
		valid[c] = true
	}
	for i := 0; ; i++ {
		if c := fmt.Sprintf("%06d", i); !valid[c] {
			return c
		}
	}
}
