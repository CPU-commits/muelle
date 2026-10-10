package usecases

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// rowsFrom devuelve las filas de auditoría nuevas desde before.
func (f *fixture) rowsFrom(t *testing.T, before int) []auditRow {
	t.Helper()
	return f.auditRows(t)[before:]
}

func expectRow(t *testing.T, r auditRow, action, target, details string, actor *int64) {
	t.Helper()
	if r.Action != action || r.Target != target || r.Details != details {
		t.Fatalf("fila=%+v, esperaba %s %s %s", r, action, target, details)
	}
	if actor == nil && (r.ActorType != "system" || r.ActorID != nil) {
		t.Fatalf("fila=%+v, esperaba actor system", r)
	}
	if actor != nil && (r.ActorType != "user" || r.ActorID == nil || *r.ActorID != *actor) {
		t.Fatalf("fila=%+v, esperaba actor user %d", r, *actor)
	}
}

func TestLoginAudit(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")

	f.login.Execute(ctx, login("felipe", "contraseña-mala!!", "1.1.1.1"))
	f.login.Execute(ctx, login("nadie", "contraseña-mala!!", "1.1.1.1"))
	rows := f.rowsFrom(t, 0)
	if len(rows) != 2 {
		t.Fatalf("audit=%+v", rows)
	}
	expectRow(t, rows[0], "auth.login_failed", "user:felipe", `{"ip":"1.1.1.1","reason":"credentials"}`, nil)
	expectRow(t, rows[1], "auth.login_failed", "user:nadie", `{"ip":"1.1.1.1","reason":"credentials"}`, nil)

	// El fallo que bloquea deja también auth.locked; los intentos rechazados
	// mientras dura el bloqueo no dejan nada.
	for range 3 {
		f.login.Execute(ctx, login("felipe", "contraseña-mala!!", "1.1.1.1"))
	}
	before := len(f.auditRows(t))
	f.login.Execute(ctx, login("felipe", "contraseña-mala!!", "1.1.1.1"))
	rows = f.rowsFrom(t, before)
	if len(rows) != 2 {
		t.Fatalf("audit=%+v", rows)
	}
	expectRow(t, rows[0], "auth.login_failed", "user:felipe", `{"ip":"1.1.1.1","reason":"credentials"}`, nil)
	expectRow(t, rows[1], "auth.locked", "user:felipe", `{"ip":"1.1.1.1","reason":"credentials"}`, nil)
	before = len(f.auditRows(t))
	for range 10 {
		f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	}
	if rows := f.rowsFrom(t, before); len(rows) != 0 {
		t.Fatalf("los intentos rechazados no se auditan: %+v", rows)
	}

	// Login correcto y logout: el actor es el usuario.
	f.advance(time.Minute)
	before = len(f.auditRows(t))
	res, err := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := f.authSession.Execute(ctx, res.Token)
	if err := f.logout.Execute(shared.WithPrincipal(ctx, p), dto.LogoutInput{SessionHash: p.SessionHash, Username: p.Username}); err != nil {
		t.Fatal(err)
	}
	rows = f.rowsFrom(t, before)
	if len(rows) != 2 {
		t.Fatalf("audit=%+v", rows)
	}
	expectRow(t, rows[0], "auth.login", "user:felipe", `{"ip":"1.1.1.1","mfa":false}`, &u.ID)
	expectRow(t, rows[1], "auth.logout", "user:felipe", `{}`, &u.ID)
	f.expectNoSecrets(t, testPassword, "contraseña-mala!!")
}

func TestLoginAuditWithTOTP(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	secret := f.enableTOTPFor(t, u.ID)
	before := len(f.auditRows(t)) // user.totp_enable

	res, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	wrong, good := f.wrongCode(t, secret), f.code(t, secret)
	f.completeTOTP.Execute(ctx, completeTOTP(res.Token, wrong))
	if _, err := f.completeTOTP.Execute(ctx, completeTOTP(res.Token, good)); err != nil {
		t.Fatal(err)
	}
	// La contraseña correcta con 2FA pendiente todavía no es un login.
	rows := f.rowsFrom(t, before)
	if len(rows) != 2 {
		t.Fatalf("audit=%+v", rows)
	}
	expectRow(t, rows[0], "auth.login_failed", "user:felipe", `{"ip":"1.1.1.1","reason":"code"}`, nil)
	expectRow(t, rows[1], "auth.login", "user:felipe", `{"ip":"1.1.1.1","mfa":true}`, &u.ID)
	f.expectNoSecrets(t, testPassword, wrong, good, secret)
}

// Con 2FA, el 5º código malo deja auth.locked (motivo code) una sola vez.
func TestLoginAuditTOTPLockedOnlyOnTransition(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	secret := f.enableTOTPFor(t, u.ID)
	before := len(f.auditRows(t))
	for range 5 {
		res, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
		f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.wrongCode(t, secret)))
	}
	for range 5 {
		res, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
		f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.code(t, secret)))
	}
	var failed, locked int
	for _, r := range f.rowsFrom(t, before) {
		switch r.Action {
		case "auth.login_failed":
			failed++
		case "auth.locked":
			locked++
			if r.Details != `{"ip":"1.1.1.1","reason":"code"}` {
				t.Fatalf("fila=%+v", r)
			}
		default:
			t.Fatalf("fila inesperada: %+v", r)
		}
	}
	if failed != 5 || locked != 1 {
		t.Fatalf("login_failed=%d locked=%d", failed, locked)
	}
}

// Los eventos que puede provocar cualquiera sin autenticarse pasan por un
// balde global: los que sobran se descartan, con un aviso en el log a lo más
// una vez por intervalo. Los eventos de un usuario autenticado no se limitan.
func TestUnauthenticatedAuditIsThrottled(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.user(t, "felipe")
	var logs bytes.Buffer
	la := NewLoginAudit(f.rec, service.NewTokenBucket(1, 3, f.clock), slog.New(slog.NewTextHandler(&logs, nil)), f.clock)
	uc := NewLogin(f.db, f.users, f.sessions, f.hasher, f.limiter, f.lockout, la, f.clock)

	for i := range 10 {
		uc.Execute(ctx, login(fmt.Sprintf("nadie%d", i), "x", "1.1.1.1"))
	}
	if n := len(f.auditRows(t)); n != 3 {
		t.Fatalf("filas=%d, esperaba solo las 3 de la ráfaga", n)
	}
	if n := strings.Count(logs.String(), "descartados"); n != 1 {
		t.Fatalf("avisos=%d, esperaba 1: %s", n, logs.String())
	}
	if !strings.Contains(logs.String(), "descartados=1") {
		t.Fatalf("el primer aviso sale con el primer descarte: %s", logs.String())
	}

	// Pasado el intervalo, el siguiente descarte avisa con la cuenta acumulada.
	f.advance(auditDropWarnEvery + time.Second) // también recarga el balde (3 tokens)
	for i := range 4 {
		uc.Execute(ctx, login(fmt.Sprintf("otro%d", i), "x", "2.2.2.2"))
	}
	if n := strings.Count(logs.String(), "descartados"); n != 2 || !strings.Contains(logs.String(), "descartados=7") {
		t.Fatalf("avisos: %s", logs.String())
	}

	// Con el balde vacío, un login correcto se audita igual.
	before := len(f.auditRows(t))
	if _, err := uc.Execute(ctx, login("felipe", testPassword, "3.3.3.3")); err != nil {
		t.Fatal(err)
	}
	if rows := f.rowsFrom(t, before); len(rows) != 1 || rows[0].Action != "auth.login" {
		t.Fatalf("audit=%+v", rows)
	}
}

// Los fallos del código TOTP solo los provoca quien sabe la contraseña, y el
// bloqueo por usuario ya los acota: se auditan siempre, sin pasar por el
// balde. Una avalancha de contraseñas malas que lo vacía no esconde un ataque
// al 2FA.
func TestCodeFailuresAuditedWithBucketDrained(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	secret := f.enableTOTPFor(t, u.ID)
	la := NewLoginAudit(f.rec, service.NewTokenBucket(1, 3, f.clock), slog.New(slog.NewTextHandler(io.Discard, nil)), f.clock)
	loginUC := NewLogin(f.db, f.users, f.sessions, f.hasher, f.limiter, f.lockout, la, f.clock)
	complete := NewCompleteTOTP(f.db, f.users, f.sessions, f.cipher, f.totp, f.limiter, f.totpLock, f.attempts, f.replay, la, f.clock)

	// La avalancha: contraseñas malas de usuarios inventados vacían el balde.
	for i := range 10 {
		loginUC.Execute(ctx, login(fmt.Sprintf("nadie%d", i), "x", fmt.Sprintf("2001:db8:%x::1", i+1)))
	}
	before := len(f.auditRows(t))
	// El ataque al 2FA: 5 códigos malos, cada uno con su sesión pendiente y su IP.
	for i := range 5 {
		ip := fmt.Sprintf("203.0.113.%d", i+1)
		res, err := loginUC.Execute(ctx, login("felipe", testPassword, ip))
		if err != nil || !res.MFARequired {
			t.Fatalf("login %d: res=%+v err=%v", i+1, res, err)
		}
		if _, err := complete.Execute(ctx, completeFrom(res.Token, f.wrongCode(t, secret), ip)); !errors.Is(err, service.ErrInvalidCode) {
			t.Fatalf("código malo %d: got %v", i+1, err)
		}
	}
	var failed, locked int
	for _, r := range f.rowsFrom(t, before) {
		switch {
		case r.Action == "auth.login_failed" && strings.Contains(r.Details, `"reason":"code"`):
			failed++
		case r.Action == "auth.locked" && strings.Contains(r.Details, `"reason":"code"`):
			locked++
		default:
			t.Fatalf("fila inesperada: %+v", r)
		}
	}
	if failed != 5 || locked != 1 {
		t.Fatalf("con el balde vacío: login_failed(code)=%d/5 locked(code)=%d/1", failed, locked)
	}
}

// Si alguien escribe su contraseña en el campo del usuario, no queda en la
// auditoría: un nombre que no es un usuario válido se guarda como <inválido>.
func TestLoginAuditHidesInvalidUsernames(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, name := range []string{"mi contraseña secreta", "Felipe", "\xff" + strings.Repeat("ñ", 100), ""} {
		f.login.Execute(ctx, login(name, "x", "1.1.1.1"))
	}
	f.login.Execute(ctx, login("nadie", "x", "1.1.1.1"))
	rows := f.auditRows(t)
	if len(rows) != 5 {
		t.Fatalf("audit=%+v", rows)
	}
	for _, r := range rows[:4] {
		if r.Action != "auth.login_failed" || r.Target != "user:<inválido>" {
			t.Fatalf("fila=%+v", r)
		}
	}
	if rows[4].Target != "user:nadie" {
		t.Fatalf("un nombre válido que no existe se guarda tal cual: %+v", rows[4])
	}
	f.expectNoSecrets(t, "contraseña secreta")
}

// expectNoSecrets revisa que ninguna fila de auditoría contenga contraseñas,
// códigos ni secretos.
func (f *fixture) expectNoSecrets(t *testing.T, secrets ...string) {
	t.Helper()
	for _, r := range f.auditRows(t) {
		for _, s := range secrets {
			if strings.Contains(r.Target, s) || strings.Contains(r.Details, s) {
				t.Fatalf("la auditoría filtra %q: %+v", s, r)
			}
		}
	}
}
