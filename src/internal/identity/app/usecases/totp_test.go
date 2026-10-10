package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/package/cipher"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

func completeTOTP(pendingToken, code string) dto.CompleteTOTPInput {
	return dto.CompleteTOTPInput{SessionHash: service.HashToken(pendingToken), Code: code, IP: "1.1.1.1"}
}

// enableTOTP configura el 2FA de userID y devuelve el secreto.
func (f *fixture) enableTOTPFor(t *testing.T, userID int64) string {
	t.Helper()
	ctx := context.Background()
	setup, err := f.beginTOTP.Execute(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.enableTOTP.Execute(ctx, dto.EnableTOTPInput{UserID: userID, Code: f.code(t, setup.Secret)}); err != nil {
		t.Fatal(err)
	}
	// El código de la activación queda usado: el próximo login necesita el del paso siguiente.
	f.advance(service.TOTPPeriod)
	return setup.Secret
}

// Un código TOTP no se acepta dos veces (RFC 6238 §5.2), tampoco en otra
// sesión pendiente; el del período siguiente sí.
func TestTOTPCodeCannotBeReused(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	secret := f.enableTOTPFor(t, u.ID)
	code := f.code(t, secret)

	first, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	if _, err := f.completeTOTP.Execute(ctx, completeTOTP(first.Token, code)); err != nil {
		t.Fatal(err)
	}
	second, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	for f.la.bucket.Allow() { // con el balde vacío, el código repetido se audita igual
	}
	before := len(f.auditRows(t))
	if _, err := f.completeTOTP.Execute(ctx, completeTOTP(second.Token, code)); !errors.Is(err, service.ErrInvalidCode) {
		t.Fatalf("el mismo código otra vez: esperaba ErrInvalidCode, got %v", err)
	}
	if rows := f.rowsFrom(t, before); len(rows) != 1 {
		t.Fatalf("el código repetido deja un auth.login_failed: %+v", rows)
	} else {
		expectRow(t, rows[0], "auth.login_failed", "user:felipe", `{"ip":"1.1.1.1","reason":"code"}`, nil)
	}
	f.advance(service.TOTPPeriod)
	if _, err := f.completeTOTP.Execute(ctx, completeTOTP(second.Token, f.code(t, secret))); err != nil {
		t.Fatalf("el código del período siguiente sirve (y la sesión pendiente sigue): %v", err)
	}
}

// El código con que se activó el 2FA tampoco sirve para entrar.
func TestTOTPEnableCodeCannotLogIn(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	setup, _ := f.beginTOTP.Execute(ctx, u.ID)
	code := f.code(t, setup.Secret)
	if err := f.enableTOTP.Execute(ctx, dto.EnableTOTPInput{UserID: u.ID, Code: code}); err != nil {
		t.Fatal(err)
	}
	res, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	if _, err := f.completeTOTP.Execute(ctx, completeTOTP(res.Token, code)); !errors.Is(err, service.ErrInvalidCode) {
		t.Fatalf("esperaba ErrInvalidCode, got %v", err)
	}
	f.advance(service.TOTPPeriod)
	if _, err := f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.code(t, setup.Secret))); err != nil {
		t.Fatal(err)
	}
}

func TestTOTPFlow(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")

	if err := f.enableTOTP.Execute(ctx, dto.EnableTOTPInput{UserID: u.ID, Code: "123456"}); !errors.Is(err, service.ErrNoPendingTOTP) {
		t.Fatalf("esperaba ErrNoPendingTOTP, got %v", err)
	}
	setup, err := f.beginTOTP.Execute(ctx, u.ID)
	if err != nil || setup.Secret == "" || setup.URL == "" {
		t.Fatalf("setup=%+v err=%v", setup, err)
	}
	if err := f.enableTOTP.Execute(ctx, dto.EnableTOTPInput{UserID: u.ID, Code: f.wrongCode(t, setup.Secret)}); !errors.Is(err, service.ErrInvalidCode) {
		t.Fatalf("esperaba ErrInvalidCode, got %v", err)
	}
	if err := f.enableTOTP.Execute(ctx, dto.EnableTOTPInput{UserID: u.ID, Code: f.code(t, setup.Secret)}); err != nil {
		t.Fatal(err)
	}
	secret := setup.Secret
	f.advance(service.TOTPPeriod) // el código de la activación ya no sirve para entrar

	// Con TOTP activo el login queda pendiente hasta el código.
	res, err := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	if err != nil || !res.MFARequired || !res.ExpiresAt.Equal(f.now().Add(service.MFAPendingTTL)) {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	p, err := f.authSession.Execute(ctx, res.Token)
	if err != nil || !p.MFAPending {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	if _, err := f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.wrongCode(t, secret))); !errors.Is(err, service.ErrInvalidCode) {
		t.Fatalf("esperaba ErrInvalidCode, got %v", err)
	}
	full, err := f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.code(t, secret)))
	if err != nil || full.MFARequired || full.Token == res.Token || !full.ExpiresAt.Equal(f.now().Add(service.SessionMax)) {
		t.Fatalf("full=%+v err=%v", full, err)
	}
	if _, err := f.authSession.Execute(ctx, res.Token); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatal("la sesión pendiente debe dejar de existir")
	}
	p, _ = f.authSession.Execute(ctx, full.Token)
	if p.MFAPending || p.UserID != u.ID {
		t.Fatalf("p=%+v", p)
	}
	// Una sesión completa no sirve para CompleteTOTP.
	if _, err := f.completeTOTP.Execute(ctx, completeTOTP(full.Token, f.code(t, secret))); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("sesión completa: got %v", err)
	}

	// Una sesión pendiente vence a los 5 min.
	res, _ = f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	f.advance(service.MFAPendingTTL)
	if _, err := f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.code(t, secret))); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("pendiente vencida: got %v", err)
	}
}

// Los códigos TOTP malos se acumulan entre logins, por usuario: una
// contraseña correcta no reinicia esa cuenta, así no se puede probar códigos
// sin fin abriendo sesiones pendientes nuevas. El paso de la contraseña no
// se bloquea por los códigos; el del código sí.
func TestTOTPFailuresLockAcrossLogins(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	secret := f.enableTOTPFor(t, u.ID)

	for i := 1; i <= 5; i++ {
		res, err := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
		if err != nil {
			t.Fatalf("login %d: %v", i, err)
		}
		if _, err := f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.wrongCode(t, secret))); !errors.Is(err, service.ErrInvalidCode) {
			t.Fatalf("código %d: got %v", i, err)
		}
	}
	res, err := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	if err != nil || !res.MFARequired {
		t.Fatalf("la contraseña sigue sirviendo: res=%+v err=%v", res, err)
	}
	if _, err := f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.code(t, secret))); !errors.Is(err, service.ErrLocked) {
		t.Fatalf("el código queda bloqueado: got %v", err)
	}

	// Pasado el bloqueo, la contraseña correcta sola no reinicia los fallos:
	// un código malo más (el 6º fallo) bloquea 2 min.
	f.advance(service.LockoutFor(5))
	res, _ = f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	if _, err := f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.wrongCode(t, secret))); !errors.Is(err, service.ErrInvalidCode) {
		t.Fatalf("6º fallo: got %v", err)
	}
	if _, err := f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.code(t, secret))); !errors.Is(err, service.ErrLocked) {
		t.Fatalf("la contraseña sola no debió reiniciar los fallos: got %v", err)
	}

	// El código correcto sí los reinicia: después, un código malo no bloquea.
	f.advance(service.LockoutFor(6))
	if _, err := f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.code(t, secret))); err != nil {
		t.Fatal(err)
	}
	f.advance(service.TOTPPeriod) // un código ya usado no se acepta de nuevo
	res, _ = f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.wrongCode(t, secret)))
	if _, err := f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.code(t, secret))); err != nil {
		t.Fatalf("tras el código correcto, un fallo no bloquea: %v", err)
	}
}

// El secreto pendiente y el activo usan AAD distintas (users:totp_pending:<id>
// y users:totp:<id>).
func TestTOTPSecretsUseTheirAAD(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")

	setup, _ := f.beginTOTP.Execute(ctx, u.ID)
	pending := f.reload(t, u.ID).TOTPPending
	if got, err := f.cipher.Decrypt(pending, "users:totp_pending:"+itoa(u.ID)); err != nil || string(got) != setup.Secret {
		t.Fatalf("pendiente: got=%q err=%v", got, err)
	}
	if _, err := f.cipher.Decrypt(pending, "users:totp:"+itoa(u.ID)); !errors.Is(err, cipher.ErrDecrypt) {
		t.Fatalf("el pendiente no debe descifrar como activo, got %v", err)
	}

	if err := f.enableTOTP.Execute(ctx, dto.EnableTOTPInput{UserID: u.ID, Code: f.code(t, setup.Secret)}); err != nil {
		t.Fatal(err)
	}
	active := f.reload(t, u.ID)
	if active.TOTPPending != nil {
		t.Fatal("el pendiente debió borrarse")
	}
	if got, err := f.cipher.Decrypt(active.TOTPSecret, "users:totp:"+itoa(u.ID)); err != nil || string(got) != setup.Secret {
		t.Fatalf("activo: got=%q err=%v", got, err)
	}
}

// Si un admin reinicia el 2FA mientras el login espera el código, la sesión
// pendiente deja de servir.
func TestCompleteTOTPAfterReset(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	secret := f.enableTOTPFor(t, u.ID)
	res, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))

	if err := f.resetTOTP.Execute(ctx, "felipe"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.code(t, secret))); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("esperaba ErrUnauthenticated, got %v", err)
	}
	// Sin 2FA, el login vuelve a ser de un paso.
	res, err := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	if err != nil || res.MFARequired {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

// Reiniciar el 2FA de un usuario (un admin, para quien perdió el teléfono)
// también borra en memoria sus fallos de código y sus códigos usados: con el
// secreto nuevo entra enseguida, aunque el anterior lo hubiera bloqueado.
func TestResetUserTOTPClearsCodeFailuresAndReplay(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	secret := f.enableTOTPFor(t, u.ID)
	res, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	used := service.TOTPStep(f.now())
	if _, err := f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.code(t, secret))); err != nil {
		t.Fatal(err)
	}
	f.advance(service.TOTPPeriod)
	for i := range 5 {
		res, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
		if _, err := f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.wrongCode(t, secret))); !errors.Is(err, service.ErrInvalidCode) {
			t.Fatalf("código malo %d: %v", i+1, err)
		}
	}
	if !f.totpLock.Locked(u.ID) {
		t.Fatal("5 códigos malos bloquean el 2FA del usuario")
	}

	if err := f.resetTOTP.Execute(ctx, "felipe"); err != nil {
		t.Fatal(err)
	}
	if f.totpLock.Locked(u.ID) {
		t.Fatal("el reinicio del 2FA borra el bloqueo por códigos")
	}
	if !f.replay.Use(u.ID, used) {
		t.Fatal("el reinicio del 2FA borra los códigos usados del usuario")
	}

	// Con el secreto nuevo entra enseguida, y un código malo no lo bloquea.
	other := f.enableTOTPFor(t, u.ID)
	res, _ = f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	if _, err := f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.wrongCode(t, other))); !errors.Is(err, service.ErrInvalidCode) {
		t.Fatalf("un código malo después del reinicio: %v", err)
	}
	if _, err := f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.code(t, other))); err != nil {
		t.Fatalf("con el secreto nuevo entra: %v", err)
	}
}

func TestRequire2FA(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	res, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))

	p, _ := f.authSession.Execute(ctx, res.Token)
	if p.NeedsTOTPSetup {
		t.Fatal("sin require_2fa no se exige")
	}
	f.setRequire2FA(t, true)
	p, _ = f.authSession.Execute(ctx, res.Token)
	if !p.NeedsTOTPSetup {
		t.Fatal("con require_2fa y sin TOTP debe exigirlo")
	}
	me, err := f.me.Execute(ctx, p)
	if err != nil || me.Username != "felipe" || me.TOTPEnabled || !me.NeedsTOTPSetup {
		t.Fatalf("me=%+v err=%v", me, err)
	}

	f.enableTOTPFor(t, u.ID)
	p, _ = f.authSession.Execute(ctx, res.Token)
	if p.NeedsTOTPSetup {
		t.Fatal("con TOTP configurado ya no se exige")
	}
	me, _ = f.me.Execute(ctx, p)
	if !me.TOTPEnabled || me.NeedsTOTPSetup {
		t.Fatalf("me=%+v", me)
	}
}

func TestEnableTOTPAudit(t *testing.T) {
	f := newFixture(t)
	u := f.user(t, "felipe")
	ctx := shared.WithPrincipal(context.Background(), shared.Principal{UserID: u.ID, Username: "felipe"})
	setup, _ := f.beginTOTP.Execute(ctx, u.ID)
	if err := f.enableTOTP.Execute(ctx, dto.EnableTOTPInput{UserID: u.ID, Code: f.code(t, setup.Secret)}); err != nil {
		t.Fatal(err)
	}
	rows := f.auditRows(t)
	if len(rows) != 1 || rows[0].Action != "user.totp_enable" || rows[0].Target != "user:felipe" ||
		rows[0].ActorType != "user" || *rows[0].ActorID != u.ID {
		t.Fatalf("audit=%+v", rows)
	}
}
