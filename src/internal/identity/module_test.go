package identity

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	sqliterepo "github.com/CPU-commits/muelle/src/internal/identity/infra/out/repository/sqlite"
	outservice "github.com/CPU-commits/muelle/src/internal/identity/infra/out/service"
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

const testPassword = "una-contraseña-larga"

// moduleUser crea un usuario con testPassword directo en el repositorio.
func moduleUser(t *testing.T, db *sqlitedb.DB, name string, at time.Time) entity.User {
	t.Helper()
	h, err := outservice.NewArgon2Hasher().Hash(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	u, err := sqliterepo.NewUserRepository(db).Create(context.Background(), entity.User{Username: name, PasswordHash: h, CreatedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// enableTOTP activa el 2FA de userID con los casos de uso del módulo y deja
// el reloj en el paso siguiente (el código de la activación queda usado).
func enableTOTP(t *testing.T, m *Module, c *clock.Manual, userID int64) string {
	t.Helper()
	ctx := context.Background()
	setup, err := m.routes.BeginTOTPSetup.Execute(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	code, _ := totp.GenerateCode(setup.Secret, c.Now())
	if err := m.routes.EnableTOTP.Execute(ctx, dto.EnableTOTPInput{UserID: userID, Code: code}); err != nil {
		t.Fatal(err)
	}
	c.Advance(service.TOTPPeriod)
	return setup.Secret
}

// codes devuelve el código TOTP válido en at y uno que no lo es (ni en los
// períodos vecinos).
func codes(secret string, at time.Time) (good, wrong string) {
	valid := map[string]bool{}
	for _, d := range []time.Duration{-service.TOTPPeriod, 0, service.TOTPPeriod} {
		c, _ := totp.GenerateCode(secret, at.Add(d))
		valid[c] = true
	}
	good, _ = totp.GenerateCode(secret, at)
	for i := 0; ; i++ {
		if c := fmt.Sprintf("%06d", i); !valid[c] {
			return good, c
		}
	}
}

// El reinicio del 2FA y el paso del código comparten el bloqueo por usuario
// y los códigos usados: reiniciarle el 2FA a un usuario bloqueado por
// códigos malos lo deja entrar enseguida con el secreto nuevo.
func TestResetUserTOTPSharesCodeLockWithLogin(t *testing.T) {
	c := clock.NewManual(time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC))
	m, db := newModule(t, c)
	ctx := context.Background()
	u := moduleUser(t, db, "felipe", c.Now())
	secret := enableTOTP(t, m, c, u.ID)
	pending := func() string {
		res, err := m.routes.Login.Execute(ctx, dto.LoginInput{Username: "felipe", Password: testPassword, IP: "1.1.1.1"})
		if err != nil || !res.MFARequired {
			t.Fatalf("login: res=%+v err=%v", res, err)
		}
		return service.HashToken(res.Token)
	}
	complete := func(session, code string) error {
		_, err := m.routes.CompleteTOTP.Execute(ctx, dto.CompleteTOTPInput{SessionHash: session, Code: code, IP: "1.1.1.1"})
		return err
	}
	good, wrong := codes(secret, c.Now())
	session := pending()
	for i := range 5 {
		if err := complete(session, wrong); !errors.Is(err, service.ErrInvalidCode) {
			t.Fatalf("código malo %d: %v", i+1, err)
		}
		if i == 3 {
			session = pending() // la 5ª, con otra sesión: la primera se agota a los 5
		}
	}
	if err := complete(pending(), good); !errors.Is(err, service.ErrLocked) {
		t.Fatalf("5 códigos malos bloquean el 2FA: %v", err)
	}

	if err := m.routes.ResetUserTOTP.Execute(ctx, "felipe"); err != nil {
		t.Fatal(err)
	}
	other := enableTOTP(t, m, c, u.ID)
	code, _ := codes(other, c.Now())
	if err := complete(pending(), code); err != nil {
		t.Fatalf("después del reinicio entra enseguida con el secreto nuevo: %v", err)
	}
}
