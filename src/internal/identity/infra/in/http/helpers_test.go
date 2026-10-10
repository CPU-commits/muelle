package http_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/identity"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	sqliterepo "github.com/CPU-commits/muelle/src/internal/identity/infra/out/repository/sqlite"
	outservice "github.com/CPU-commits/muelle/src/internal/identity/infra/out/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

const testPassword = "una-contraseña-larga"

// env es el módulo identity armado de verdad detrás del servidor de cmd/http.
type env struct {
	t     *testing.T
	db    *sqlitedb.DB
	clock *clock.Manual
	mod   *identity.Module
	c     *httptestutil.Client
	users *sqliterepo.UserRepository
}

func newEnv(t *testing.T) *env {
	return newEnvWith(t, nil)
}

// newEnvWith registra además las rutas de extra (para simular rutas de otros módulos).
func newEnvWith(t *testing.T, extra func(k *utils.Kit)) *env {
	t.Helper()
	db := sqlitetest.New(t)
	c := clock.NewManual(time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC))
	ciph, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(t.Output(), nil))
	m := identity.New(identity.Deps{DB: db, Cipher: ciph, Clock: c, Audit: sqlitedb.NewAuditRecorder(db, c, log), Log: log})
	client := httptestutil.New(t, m.Authenticator(), func(k *utils.Kit) {
		m.RegisterRoutes(k)
		if extra != nil {
			extra(k)
		}
	})
	return &env{t: t, db: db, clock: c, mod: m, c: client, users: sqliterepo.NewUserRepository(db)}
}

func (e *env) do(method, path string, body any, opts ...httptestutil.Opt) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.c.Do(method, path, body, opts...)
}

// user crea un usuario directo en el repositorio, con testPassword.
func (e *env) user(name string) entity.User {
	e.t.Helper()
	h, err := outservice.NewArgon2Hasher().Hash(testPassword)
	if err != nil {
		e.t.Fatal(err)
	}
	u, err := e.users.Create(context.Background(), entity.User{Username: name, PasswordHash: h, CreatedAt: e.clock.Now()})
	if err != nil {
		e.t.Fatal(err)
	}
	return u
}

// login crea el usuario "admin" (si no existe), inicia sesión y devuelve el token de la cookie.
func (e *env) login() string {
	e.t.Helper()
	if _, err := e.users.GetByUsername(context.Background(), "admin"); err != nil {
		e.user("admin")
	}
	w := e.do("POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": testPassword})
	httptestutil.Expect(e.t, w, http.StatusOK)
	return httptestutil.SessionFrom(e.t, w)
}

func (e *env) exec(query string, args ...any) int64 {
	e.t.Helper()
	res, err := e.db.Conn(context.Background()).ExecContext(context.Background(), query, args...)
	if err != nil {
		e.t.Fatalf("%s: %v", query, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func (e *env) code(secret string) string {
	e.t.Helper()
	c, err := totp.GenerateCode(secret, e.clock.Now())
	if err != nil {
		e.t.Fatal(err)
	}
	return c
}

// wrongCode devuelve un código que no es válido ahora (ni en los períodos vecinos).
func (e *env) wrongCode(secret string) string {
	valid := map[string]bool{}
	for _, d := range []time.Duration{-30 * time.Second, 0, 30 * time.Second} {
		c, _ := totp.GenerateCode(secret, e.clock.Now().Add(d))
		valid[c] = true
	}
	for i := 0; ; i++ {
		if c := fmt.Sprintf("%06d", i); !valid[c] {
			return c
		}
	}
}

func expectCode(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	httptestutil.Expect(t, w, status)
	if got := httptestutil.ErrorCode(t, w); got != code {
		t.Fatalf("code=%s, esperaba %s: %s", got, code, w.Body.String())
	}
}
