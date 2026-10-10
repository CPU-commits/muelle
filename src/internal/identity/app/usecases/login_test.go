package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

func login(name, password, ip string) dto.LoginInput {
	return dto.LoginInput{Username: name, Password: password, IP: ip, UserAgent: "curl"}
}

func TestLoginAndAuthenticateSession(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")

	res, err := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	if err != nil || res.MFARequired || res.Token == "" || res.UserID != u.ID || res.Username != "felipe" ||
		!res.ExpiresAt.Equal(f.now().Add(service.SessionMax)) {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	p, err := f.authSession.Execute(ctx, res.Token)
	if err != nil || p.UserID != u.ID || p.Username != "felipe" || p.MFAPending || p.IsToken() ||
		p.SessionHash != service.HashToken(res.Token) {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	ss, _ := f.sessions.Get(ctx, p.SessionHash)
	if ss.IP != "1.1.1.1" || ss.UserAgent != "curl" {
		t.Fatalf("la sesión guarda IP y user agent: %+v", ss)
	}
	if _, err := f.authSession.Execute(ctx, "token-falso"); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("esperaba ErrUnauthenticated, got %v", err)
	}
	if err := f.logout.Execute(ctx, p.SessionHash); err != nil {
		t.Fatal(err)
	}
	if _, err := f.authSession.Execute(ctx, res.Token); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("tras logout: esperaba ErrUnauthenticated, got %v", err)
	}
}

func TestSessionIdleAndMaxExpiry(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.user(t, "felipe")
	res, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))

	// Usarla cada 11 h la mantiene viva hasta las 165 h...
	for i := 1; i <= 15; i++ {
		f.advance(11 * time.Hour)
		if _, err := f.authSession.Execute(ctx, res.Token); err != nil {
			t.Fatalf("a las %d h: %v", 11*i, err)
		}
	}
	// ...pero a las 176 h ya pasó el máximo de 7 días (168 h).
	f.advance(11 * time.Hour)
	if _, err := f.authSession.Execute(ctx, res.Token); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("pasado el máximo: esperaba ErrUnauthenticated, got %v", err)
	}
	// La sesión vencida se borra al detectarla.
	if _, err := f.sessions.Get(ctx, service.HashToken(res.Token)); !errors.Is(err, sharedrepo.ErrNotFound) {
		t.Fatalf("la sesión vencida debió borrarse, got %v", err)
	}

	res2, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	f.advance(12*time.Hour + time.Second)
	if _, err := f.authSession.Execute(ctx, res2.Token); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("12 h sin uso: esperaba ErrUnauthenticated, got %v", err)
	}
}

func TestAuthenticateSessionTouchesOncePerMinute(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.user(t, "felipe")
	res, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	hash := service.HashToken(res.Token)
	start := f.now()

	f.advance(30 * time.Second)
	f.authSession.Execute(ctx, res.Token)
	if ss, _ := f.sessions.Get(ctx, hash); !ss.LastSeenAt.Equal(start) {
		t.Fatalf("antes del minuto no se escribe: last_seen=%v", ss.LastSeenAt)
	}
	f.advance(30 * time.Second)
	f.authSession.Execute(ctx, res.Token)
	if ss, _ := f.sessions.Get(ctx, hash); !ss.LastSeenAt.Equal(f.now()) {
		t.Fatalf("al minuto se actualiza: last_seen=%v", ss.LastSeenAt)
	}
}

func TestAuthenticateSessionOfDeletedUser(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	res, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	f.users.Delete(ctx, u.ID)
	if _, err := f.authSession.Execute(ctx, res.Token); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("esperaba ErrUnauthenticated, got %v", err)
	}
}

func TestLoginFailuresLockAndReset(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.user(t, "felipe")

	if _, err := f.login.Execute(ctx, login("nadie", testPassword, "9.9.9.9")); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("usuario inexistente: got %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := f.login.Execute(ctx, login("felipe", "contraseña-mala!!", "1.1.1.1")); !errors.Is(err, service.ErrInvalidCredentials) {
			t.Fatalf("intento %d: got %v", i, err)
		}
	}
	// Tras 5 fallos queda bloqueado 1 min, aunque la contraseña sea correcta.
	if _, err := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1")); !errors.Is(err, service.ErrLocked) {
		t.Fatalf("esperaba ErrLocked, got %v", err)
	}
	f.advance(time.Minute)
	if _, err := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1")); err != nil {
		t.Fatalf("tras el bloqueo: %v", err)
	}
	u, _ := f.users.GetByUsername(ctx, "felipe")
	if u.FailedLogins != 0 || u.LockedUntil != nil {
		t.Fatalf("el login correcto debe resetear los fallos: %+v", u)
	}
}

// Los fallos se guardan aunque el login devuelva un error: InTx deshace todo
// cuando fn devuelve un error, así que el contador se confirma aparte.
func TestFailedLoginsPersist(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")

	for n := 1; n <= 7; n++ {
		f.login.Execute(ctx, login("felipe", "contraseña-mala!!", "1.1.1.1"))
		got := f.reload(t, u.ID)
		if got.FailedLogins != n {
			t.Fatalf("tras %d fallos, failed_logins=%d", n, got.FailedLogins)
		}
		if want := service.LockoutFor(n); want > 0 {
			if got.LockedUntil == nil || !got.LockedUntil.Equal(f.now().Add(want)) {
				t.Fatalf("tras %d fallos, locked_until=%v, esperaba ahora + %v", n, got.LockedUntil, want)
			}
			// Mientras dura el bloqueo ni se verifica la contraseña; se espera a que pase.
			f.advance(want)
		}
	}
	if got := f.reload(t, u.ID); got.FailedLogins != 7 {
		t.Fatalf("failed_logins=%d", got.FailedLogins)
	}
}

// Un cliente que corta la conexión cancela el ctx del request: el fallo
// igual se guarda.
func TestLoginFailureSurvivesCanceledContext(t *testing.T) {
	f := newFixture(t)
	u := f.user(t, "felipe")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for range 5 {
		if err := f.login.failures.register(ctx, u.ID); err != nil {
			t.Fatal(err)
		}
	}
	got := f.reload(t, u.ID)
	if got.FailedLogins != 5 || !got.LockedAt(f.now()) {
		t.Fatalf("u=%+v", got)
	}
}

func TestIPRateLimit(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		f.login.Execute(ctx, login("nadie", "x", "6.6.6.6"))
	}
	if _, err := f.login.Execute(ctx, login("nadie", "x", "6.6.6.6")); !errors.Is(err, service.ErrRateLimited) {
		t.Fatalf("esperaba ErrRateLimited, got %v", err)
	}
	if _, err := f.login.Execute(ctx, login("nadie", "x", "7.7.7.7")); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("otra IP no se ve afectada: got %v", err)
	}
}

func TestCleanupSessions(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	f.sessions.Create(ctx, entity.NewSession("vieja", u.ID, false, f.now(), service.SessionMax, "", ""))
	f.advance(6 * time.Hour)
	f.sessions.Create(ctx, entity.NewSession("nueva", u.ID, false, f.now(), service.SessionMax, "", ""))
	f.advance(6*time.Hour + time.Second)

	n, err := f.cleanup.Execute(ctx)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if _, err := f.sessions.Get(ctx, "nueva"); err != nil {
		t.Fatalf("la sesión usada hace 6 h sigue: %v", err)
	}
}
