package usecases

import (
	"context"
	"errors"
	"fmt"
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
	// ActorName() es el autor de la auditoría de cada petición con esta sesión.
	if err != nil || p.UserID != u.ID || p.Username != "felipe" || p.ActorName() != "felipe" || p.MFAPending || p.IsToken() ||
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
	if err := f.logout.Execute(ctx, dto.LogoutInput{SessionHash: p.SessionHash, Username: p.Username}); err != nil {
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
	// El login correcto reinició los fallos: 4 más no bloquean.
	for range 4 {
		f.login.Execute(ctx, login("felipe", "contraseña-mala!!", "1.1.1.1"))
	}
	if _, err := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1")); err != nil {
		t.Fatalf("el login correcto debe reiniciar los fallos: %v", err)
	}
}

// El calendario del bloqueo se respeta de punta a punta: 5 fallos, 1 min; el
// 6º, 2 min; el 7º, 4 min.
func TestLoginLockoutSchedule(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.user(t, "felipe")
	for range 4 {
		f.login.Execute(ctx, login("felipe", "contraseña-mala!!", "1.1.1.1"))
	}
	for n := 5; n <= 7; n++ {
		if _, err := f.login.Execute(ctx, login("felipe", "contraseña-mala!!", "1.1.1.1")); !errors.Is(err, service.ErrInvalidCredentials) {
			t.Fatalf("fallo %d: got %v", n, err)
		}
		d := service.LockoutFor(n)
		f.advance(d - time.Second)
		if _, err := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1")); !errors.Is(err, service.ErrLocked) {
			t.Fatalf("con %d fallos sigue bloqueado hasta %v: got %v", n, d, err)
		}
		f.advance(time.Second)
	}
}

// El bloqueo es por usuario e IP: un atacante que se equivoca desde su IP
// solo se bloquea a sí mismo, y el usuario entra desde la suya.
func TestAttackerLocksOnlyItself(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.user(t, "admin")
	for range 5 {
		f.login.Execute(ctx, login("admin", "contraseña-mala!!", "203.0.113.9"))
	}
	if _, err := f.login.Execute(ctx, login("admin", testPassword, "203.0.113.9")); !errors.Is(err, service.ErrLocked) {
		t.Fatalf("el atacante queda bloqueado: got %v", err)
	}
	if _, err := f.login.Execute(ctx, login("admin", testPassword, "198.51.100.1")); err != nil {
		t.Fatalf("el admin entra desde otra IP: %v", err)
	}
}

// Una IPv6 cuenta por su /64: rotar direcciones del mismo /64 no ayuda.
func TestLoginLockoutGroupsIPv6By64(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.user(t, "admin")
	for i := range 5 {
		f.login.Execute(ctx, login("admin", "contraseña-mala!!", fmt.Sprintf("2001:db8:1:2::%x", i+1)))
	}
	if _, err := f.login.Execute(ctx, login("admin", testPassword, "2001:db8:1:2:ffff::1")); !errors.Is(err, service.ErrLocked) {
		t.Fatalf("otra dirección del mismo /64: got %v", err)
	}
	if _, err := f.login.Execute(ctx, login("admin", testPassword, "2001:db8:1:3::1")); err != nil {
		t.Fatalf("otro /64 entra: %v", err)
	}
}

// Un usuario que no existe recibe exactamente las mismas respuestas que uno
// que existe con una contraseña mala: 423 locked no revela qué usuarios hay.
func TestUnknownAndKnownUsersGetSameResponses(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.user(t, "felipe")
	sequence := func(username, ip string) []error {
		var out []error
		for range 7 {
			_, err := f.login.Execute(ctx, login(username, "contraseña-mala!!", ip))
			out = append(out, err)
		}
		return out
	}
	known, unknown := sequence("felipe", "1.1.1.1"), sequence("nadie", "2.2.2.2")
	for i := range known {
		if !errors.Is(unknown[i], known[i]) || !errors.Is(known[i], unknown[i]) {
			t.Fatalf("intento %d: existe=%v, no existe=%v", i+1, known[i], unknown[i])
		}
	}
	if !errors.Is(known[4], service.ErrInvalidCredentials) || !errors.Is(known[5], service.ErrLocked) {
		t.Fatalf("secuencia=%v", known)
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
