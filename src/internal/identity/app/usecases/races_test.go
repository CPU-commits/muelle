package usecases

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// Estos tests provocan carreras de forma determinista: un gate detiene una
// llamada (al hasher o al TOTP reales) en el medio de un caso de uso,
// mientras el test cambia el estado por debajo.

type gate struct {
	entered chan struct{}
	release chan struct{}
}

func newGate() *gate { return &gate{entered: make(chan struct{}, 8), release: make(chan struct{})} }

func (g *gate) pass() {
	g.entered <- struct{}{}
	<-g.release
}

type gatedHasher struct {
	service.PasswordHasher
	g *gate
}

func (h gatedHasher) Verify(hash, password string) (bool, error) {
	h.g.pass()
	return h.PasswordHasher.Verify(hash, password)
}

type gatedTOTP struct {
	service.TOTP
	g *gate
}

func (t gatedTOTP) Validate(code, secret string, at time.Time) (int64, bool) {
	t.g.pass()
	return t.TOTP.Validate(code, secret, at)
}

type countingHasher struct {
	service.PasswordHasher
	hashes *atomic.Int32
}

func (h countingHasher) Hash(password string) (string, error) {
	h.hashes.Add(1)
	return h.PasswordHasher.Hash(password)
}

func (f *fixture) countFullSessions(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.db.Conn(context.Background()).QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM sessions WHERE mfa_pending = 0`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Un intento que pasó el chequeo de bloqueo y esperaba al hasher no abre
// sesión si mientras tanto otros intentos bloquearon el par (usuario, IP).
// La respuesta es "bloqueado" sea la contraseña correcta o no, y el bloqueo
// no cambia, para no revelar cuál era.
func TestQueuedLoginRechecksLockout(t *testing.T) {
	for _, password := range []string{testPassword, "contraseña-mala!!"} {
		f := newFixture(t)
		ctx := context.Background()
		f.user(t, "felipe")
		g := newGate()
		uc := NewLogin(f.db, f.users, f.sessions, gatedHasher{f.hasher, g}, f.limiter, f.lockout, f.clock)

		done := make(chan error, 1)
		go func() {
			_, err := uc.Execute(ctx, login("felipe", password, "1.1.1.1"))
			done <- err
		}()
		<-g.entered // pasó el chequeo de bloqueo y está en el hasher
		for range 5 {
			f.lockout.Fail("felipe", "1.1.1.1") // otros intentos bloquean el par
		}
		g.release <- struct{}{}
		if err := <-done; !errors.Is(err, service.ErrLocked) {
			t.Fatalf("%q: esperaba ErrLocked, got %v", password, err)
		}
		if n := f.countSessions(t); n != 0 {
			t.Fatalf("%q: no debió abrirse ninguna sesión: %d", password, n)
		}
		// El bloqueo sigue siendo de 1 min: el intento en espera no lo extendió.
		f.advance(service.LockoutFor(5))
		if _, err := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1")); err != nil {
			t.Fatalf("%q: al minuto entra: %v", password, err)
		}
	}
}

// Tampoco el estado que queda puede revelar la contraseña: con 4 fallos, un
// candidato espera en el hasher mientras un 5º intento malo bloquea el par
// 1 min. Sea el candidato correcto o no, después responde igual (si el malo
// extendiera el bloqueo a 2 min, una prueba al minuto revelaría cuál era).
func TestQueuedLoginLeavesSameLockState(t *testing.T) {
	run := func(candidate string) []error {
		f := newFixture(t)
		ctx := context.Background()
		f.user(t, "felipe")
		for range 4 {
			f.login.Execute(ctx, login("felipe", "contraseña-mala!!", "1.1.1.1"))
		}
		g := newGate()
		uc := NewLogin(f.db, f.users, f.sessions, gatedHasher{f.hasher, g}, f.limiter, f.lockout, f.clock)
		done := make(chan error, 1)
		go func() {
			_, err := uc.Execute(ctx, login("felipe", candidate, "1.1.1.1"))
			done <- err
		}()
		<-g.entered
		if _, err := f.login.Execute(ctx, login("felipe", "otra-mala-más!!", "1.1.1.1")); !errors.Is(err, service.ErrInvalidCredentials) {
			t.Fatalf("5º fallo: %v", err)
		}
		g.release <- struct{}{}
		var out []error
		out = append(out, <-done)
		// Sondas: justo antes y justo después del minuto, y otra mala después.
		f.advance(service.LockoutFor(5) - time.Second)
		_, err := f.login.Execute(ctx, login("felipe", "sonda-incorrecta!", "1.1.1.1"))
		out = append(out, err)
		f.advance(time.Second)
		_, err = f.login.Execute(ctx, login("felipe", "sonda-incorrecta!", "1.1.1.1"))
		out = append(out, err)
		_, err = f.login.Execute(ctx, login("felipe", "sonda-incorrecta!", "1.1.1.1"))
		out = append(out, err)
		return out
	}
	good, bad := run(testPassword), run("contraseña-mala!!")
	for i := range good {
		if !errors.Is(good[i], bad[i]) || !errors.Is(bad[i], good[i]) {
			t.Fatalf("respuesta %d: correcta=%v incorrecta=%v", i, good, bad)
		}
	}
	if !errors.Is(good[0], service.ErrLocked) || !errors.Is(good[2], service.ErrInvalidCredentials) {
		t.Fatalf("respuestas=%v", good)
	}
}

// Dos CompleteTOTP con la misma sesión pendiente y un código válido: solo
// uno abre la sesión completa.
func TestConcurrentCompleteTOTPConsumesOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	secret := f.enableTOTPFor(t, u.ID)
	res, err := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	if err != nil || !res.MFARequired {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	g := newGate()
	uc := NewCompleteTOTP(f.db, f.users, f.sessions, f.cipher, gatedTOTP{f.totp, g}, f.limiter, f.lockout, f.replay, f.clock)
	code := f.code(t, secret)

	errs := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := uc.Execute(ctx, completeTOTP(res.Token, code))
			errs <- err
		}()
	}
	<-g.entered
	<-g.entered // los dos ya revisaron la sesión y validan el código
	close(g.release)

	var ok, unauth int
	for range 2 {
		switch err := <-errs; {
		case err == nil:
			ok++
		case errors.Is(err, shared.ErrUnauthenticated):
			unauth++
		default:
			t.Fatalf("error inesperado: %v", err)
		}
	}
	if ok != 1 || unauth != 1 || f.countFullSessions(t) != 1 {
		t.Fatalf("ok=%d unauth=%d sesiones completas=%d", ok, unauth, f.countFullSessions(t))
	}
}

// Dos sesiones pendientes canjeadas a la vez con el mismo código: el código
// solo vale una vez, así que solo una abre la sesión completa.
func TestTOTPReplayInParallel(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	secret := f.enableTOTPFor(t, u.ID)
	a, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	b, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	g := newGate()
	uc := NewCompleteTOTP(f.db, f.users, f.sessions, f.cipher, gatedTOTP{f.totp, g}, f.limiter, f.lockout, f.replay, f.clock)
	code := f.code(t, secret)

	errs := make(chan error, 2)
	for _, pending := range []string{a.Token, b.Token} {
		go func() {
			_, err := uc.Execute(ctx, completeTOTP(pending, code))
			errs <- err
		}()
	}
	<-g.entered
	<-g.entered // las dos validaron el código y esperan
	close(g.release)

	var ok, invalid int
	for range 2 {
		switch err := <-errs; {
		case err == nil:
			ok++
		case errors.Is(err, service.ErrInvalidCode):
			invalid++
		default:
			t.Fatalf("error inesperado: %v", err)
		}
	}
	if ok != 1 || invalid != 1 || f.countFullSessions(t) != 1 {
		t.Fatalf("ok=%d inválidos=%d sesiones completas=%d", ok, invalid, f.countFullSessions(t))
	}
}

// Si un admin reinicia el 2FA mientras se valida el código, no se abre la sesión.
func TestCompleteTOTPResetDuringValidation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	secret := f.enableTOTPFor(t, u.ID)
	res, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	g := newGate()
	uc := NewCompleteTOTP(f.db, f.users, f.sessions, f.cipher, gatedTOTP{f.totp, g}, f.limiter, f.lockout, f.replay, f.clock)

	done := make(chan error, 1)
	go func() {
		_, err := uc.Execute(ctx, completeTOTP(res.Token, f.code(t, secret)))
		done <- err
	}()
	<-g.entered
	if err := f.resetTOTP.Execute(ctx, "felipe"); err != nil {
		t.Fatal(err)
	}
	close(g.release)
	if err := <-done; !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("esperaba ErrUnauthenticated, got %v", err)
	}
	if n := f.countFullSessions(t); n != 0 {
		t.Fatalf("sesiones completas=%d", n)
	}
}

// Una invitación inexistente o vencida se rechaza antes de calcular el hash:
// el endpoint es público y el hasher tiene solo 2 lugares.
func TestAcceptInviteChecksTokenBeforeHashing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var hashes atomic.Int32
	uc := NewAcceptInvite(f.db, f.users, f.invites, f.sessions, countingHasher{f.hasher, &hashes}, f.rec, f.limiter, f.clock)

	if _, err := uc.Execute(ctx, accept("inventado", "ana", testPassword)); !errors.Is(err, service.ErrInviteInvalid) {
		t.Fatalf("got %v", err)
	}
	f.invite(t, "vieja", nil, time.Hour)
	f.advance(2 * time.Hour)
	if _, err := uc.Execute(ctx, accept("vieja", "ana", testPassword)); !errors.Is(err, service.ErrInviteInvalid) {
		t.Fatalf("got %v", err)
	}
	if n := hashes.Load(); n != 0 {
		t.Fatalf("%d hashes para invitaciones inválidas", n)
	}
	f.invite(t, "buena", nil, time.Hour)
	if _, err := uc.Execute(ctx, accept("buena", "ana", testPassword)); err != nil {
		t.Fatal(err)
	}
	if n := hashes.Load(); n != 1 {
		t.Fatalf("hashes=%d", n)
	}
}

// Aceptar invitaciones y el setup cuentan contra el mismo límite por IP que el login.
func TestAcceptInviteRateLimited(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for range service.LoginMaxPerIP {
		f.acceptInvite.Execute(ctx, dto.AcceptInviteInput{Token: "x", Username: "ana", Password: testPassword, IP: "6.6.6.6"})
	}
	if _, err := f.acceptInvite.Execute(ctx, dto.AcceptInviteInput{Token: "x", Username: "ana", Password: testPassword, IP: "6.6.6.6"}); !errors.Is(err, service.ErrRateLimited) {
		t.Fatalf("invitación: esperaba ErrRateLimited, got %v", err)
	}
	if _, err := f.setup.Execute(ctx, dto.AcceptInviteInput{Token: "x", Username: "ana", Password: testPassword, IP: "6.6.6.6"}); !errors.Is(err, service.ErrRateLimited) {
		t.Fatalf("setup: esperaba ErrRateLimited, got %v", err)
	}
	if _, err := f.login.Execute(ctx, login("ana", testPassword, "6.6.6.6")); !errors.Is(err, service.ErrRateLimited) {
		t.Fatalf("login: esperaba ErrRateLimited, got %v", err)
	}
}

// Un usuario ya borrado no puede borrar a otro con un request que estaba en
// curso: así dos usuarios que se borran a la vez no dejan la base vacía.
func TestDeleteUserRequiresLiveActor(t *testing.T) {
	f := newFixture(t)
	admin := f.user(t, "admin")
	ana := f.user(t, "ana")

	if err := f.deleteUser.Execute(withUser(admin), dto.DeleteUserInput{Username: "ana", ActorID: admin.ID}); err != nil {
		t.Fatal(err)
	}
	if err := f.deleteUser.Execute(withUser(ana), dto.DeleteUserInput{Username: "admin", ActorID: ana.ID}); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("esperaba ErrUnauthenticated, got %v", err)
	}
	if n, _ := f.users.Count(context.Background()); n != 1 {
		t.Fatalf("users=%d", n)
	}
}

func TestConcurrentCrossDeletionKeepsOneUser(t *testing.T) {
	f := newFixture(t)
	admin := f.user(t, "admin")
	ana := f.user(t, "ana")

	errs := make(chan error, 2)
	go func() {
		errs <- f.deleteUser.Execute(withUser(admin), dto.DeleteUserInput{Username: "ana", ActorID: admin.ID})
	}()
	go func() {
		errs <- f.deleteUser.Execute(withUser(ana), dto.DeleteUserInput{Username: "admin", ActorID: ana.ID})
	}()
	var ok, unauth int
	for range 2 {
		switch err := <-errs; {
		case err == nil:
			ok++
		case errors.Is(err, shared.ErrUnauthenticated):
			unauth++
		default:
			t.Fatalf("error inesperado: %v", err)
		}
	}
	if n, _ := f.users.Count(context.Background()); ok != 1 || unauth != 1 || n != 1 {
		t.Fatalf("ok=%d unauth=%d users=%d", ok, unauth, n)
	}
}

// Si se genera un secreto nuevo mientras se confirma el anterior, se activa
// el que se validó y el nuevo queda pendiente (no se pierde).
func TestEnableTOTPDoesNotDropNewerPending(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	a, _ := f.beginTOTP.Execute(ctx, u.ID)
	g := newGate()
	uc := NewEnableTOTP(f.db, f.users, f.cipher, gatedTOTP{f.totp, g}, f.replay, f.rec, f.clock)

	done := make(chan error, 1)
	go func() { done <- uc.Execute(ctx, dto.EnableTOTPInput{UserID: u.ID, Code: f.code(t, a.Secret)}) }()
	<-g.entered // validando el código de A
	bc := make(chan dto.TOTPSetupOutput, 1)
	go func() {
		b, err := f.beginTOTP.Execute(ctx, u.ID)
		if err != nil {
			t.Error(err)
		}
		bc <- b
	}()
	// B intenta guardar su secreto mientras se valida A. No hay forma de ver
	// que B espera el lock de SQLite, así que se le da un momento: con el
	// arreglo el test pasa sea cual sea el orden, y sin él (B escribe antes de
	// que se active A) falla.
	time.Sleep(50 * time.Millisecond)
	close(g.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	b := <-bc

	cur := f.reload(t, u.ID)
	active, err := f.cipher.Decrypt(cur.TOTPSecret, service.TOTPSecretAAD(u.ID))
	if err != nil || string(active) != a.Secret {
		t.Fatalf("activo: %q err=%v", active, err)
	}
	pending, err := f.cipher.Decrypt(cur.TOTPPending, service.TOTPPendingAAD(u.ID))
	if err != nil || string(pending) != b.Secret {
		t.Fatalf("el secreto nuevo debió quedar pendiente: %q err=%v", pending, err)
	}
	if err := f.enableTOTP.Execute(ctx, dto.EnableTOTPInput{UserID: u.ID, Code: f.code(t, b.Secret)}); err != nil {
		t.Fatalf("el secreto nuevo se puede confirmar: %v", err)
	}
}
