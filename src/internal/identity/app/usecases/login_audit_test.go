package usecases

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
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

	// Bloqueado: cada intento rechazado sin verificar queda como auth.locked.
	for range 4 {
		f.login.Execute(ctx, login("felipe", "contraseña-mala!!", "1.1.1.1"))
	}
	before := len(f.auditRows(t))
	f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	rows = f.rowsFrom(t, before)
	if len(rows) != 1 {
		t.Fatalf("audit=%+v", rows)
	}
	expectRow(t, rows[0], "auth.locked", "user:felipe", `{"ip":"1.1.1.1"}`, nil)

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

// El nombre del login puede ser cualquier texto: en la auditoría va como
// UTF-8 válido y recortado a 64 bytes.
func TestLoginAuditSanitizesUsername(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	name := "\xff" + strings.Repeat("ñ", 100)
	f.login.Execute(ctx, login(name, "x", "1.1.1.1"))
	rows := f.auditRows(t)
	if len(rows) != 1 || rows[0].Action != "auth.login_failed" {
		t.Fatalf("audit=%+v", rows)
	}
	target, ok := strings.CutPrefix(rows[0].Target, "user:")
	if !ok || !utf8.ValidString(target) || len(target) > 64 || !strings.HasPrefix(target, "�ñ") {
		t.Fatalf("target=%q (%d bytes)", rows[0].Target, len(target))
	}
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
