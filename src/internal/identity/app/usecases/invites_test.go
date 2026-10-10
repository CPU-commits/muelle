package usecases

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

func itoa(id int64) string { return strconv.FormatInt(id, 10) }

func accept(token, username, password string) dto.AcceptInviteInput {
	return dto.AcceptInviteInput{Token: token, Username: username, Password: password, IP: "1.1.1.1", UserAgent: "curl"}
}

// invite guarda una invitación con el hash del token plain.
func (f *fixture) invite(t *testing.T, plain string, createdBy *int64, ttl time.Duration) {
	t.Helper()
	if err := f.invites.Create(context.Background(), entity.NewInvite(service.HashToken(plain), createdBy, f.now(), ttl)); err != nil {
		t.Fatal(err)
	}
}

func TestInvitesAndSetup(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	token, created, err := f.ensureSetup.Execute(ctx)
	if err != nil || !created || token == "" {
		t.Fatalf("token=%q created=%v err=%v", token, created, err)
	}
	// Un segundo arranque sin usuarios invalida el link anterior.
	token2, created, _ := f.ensureSetup.Execute(ctx)
	if !created || token2 == token {
		t.Fatalf("token2=%q created=%v", token2, created)
	}
	if _, err := f.acceptInvite.Execute(ctx, accept(token, "felipe", testPassword)); !errors.Is(err, service.ErrInviteInvalid) {
		t.Fatalf("el link viejo debió invalidarse, got %v", err)
	}
	res, err := f.acceptInvite.Execute(ctx, accept(token2, "felipe", testPassword))
	if err != nil || res.Token == "" || res.UserID == 0 || res.Username != "felipe" || res.MFARequired {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if _, err := f.authSession.Execute(ctx, res.Token); err != nil {
		t.Fatalf("la sesión del nuevo usuario debe servir: %v", err)
	}
	if _, created, _ := f.ensureSetup.Execute(ctx); created {
		t.Fatal("con usuarios no se crea link de setup")
	}

	inv, err := f.createInvite.Execute(ctx, dto.CreateInviteInput{CreatedBy: res.UserID, CreatorUsername: "felipe"})
	if err != nil || inv.Token == "" || !inv.ExpiresAt.Equal(f.now().Add(service.InviteTTL)) {
		t.Fatalf("inv=%+v err=%v", inv, err)
	}
	if _, err := f.acceptInvite.Execute(ctx, accept(inv.Token, "ana", "corta")); err == nil {
		t.Fatal("contraseña corta debió fallar")
	}
	ana, err := f.acceptInvite.Execute(ctx, accept(inv.Token, "ana", testPassword))
	if err != nil {
		t.Fatal(err)
	}
	if u := f.reload(t, ana.UserID); u.CreatedBy == nil || *u.CreatedBy != res.UserID {
		t.Fatalf("ana la creó felipe: %+v", u)
	}
}

func TestAcceptInvite(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	felipe := f.user(t, "felipe")
	f.invite(t, "inv1", &felipe.ID, 24*time.Hour)

	res, err := f.acceptInvite.Execute(ctx, accept("inv1", "ana", testPassword))
	if err != nil || res.Username != "ana" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	ana := f.reload(t, res.UserID)
	if ana.CreatedBy == nil || *ana.CreatedBy != felipe.ID {
		t.Fatalf("ana=%+v", ana)
	}
	if ok, _ := f.hasher.Verify(ana.PasswordHash, testPassword); !ok {
		t.Fatal("la contraseña se guarda con argon2id")
	}
	if _, err := f.acceptInvite.Execute(ctx, accept("inv1", "otra", testPassword)); !errors.Is(err, service.ErrInviteInvalid) {
		t.Fatalf("invitación usada: esperaba ErrInviteInvalid, got %v", err)
	}
	if _, err := f.acceptInvite.Execute(ctx, accept("no-existe", "otra", testPassword)); !errors.Is(err, service.ErrInviteInvalid) {
		t.Fatalf("esperaba ErrInviteInvalid, got %v", err)
	}
}

func TestAcceptInviteExpiredOrNameTaken(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.user(t, "felipe")
	f.invite(t, "vieja", nil, time.Hour)
	f.invite(t, "nueva", nil, 48*time.Hour)

	// Nombre tomado: falla y la invitación sigue disponible, sin sesión ni auditoría.
	if _, err := f.acceptInvite.Execute(ctx, accept("nueva", "felipe", testPassword)); !errors.Is(err, sharedrepo.ErrConflict) {
		t.Fatalf("esperaba ErrConflict, got %v", err)
	}
	if n := f.countSessions(t); n != 0 || len(f.auditRows(t)) != 0 {
		t.Fatalf("el fallo no deja sesiones (%d) ni auditoría (%+v)", n, f.auditRows(t))
	}
	if _, err := f.acceptInvite.Execute(ctx, accept("nueva", "ana", testPassword)); err != nil {
		t.Fatalf("la invitación no debió consumirse: %v", err)
	}

	f.advance(2 * time.Hour)
	if _, err := f.acceptInvite.Execute(ctx, accept("vieja", "beto", testPassword)); !errors.Is(err, service.ErrInviteInvalid) {
		t.Fatalf("vencida: esperaba ErrInviteInvalid, got %v", err)
	}
}

func TestAcceptInviteValidates(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.invite(t, "inv", nil, time.Hour)
	cases := []struct {
		username, password, field string
	}{
		{"A", testPassword, "username"},
		{"ana", "corta", "password"},
	}
	for _, c := range cases {
		_, err := f.acceptInvite.Execute(ctx, accept("inv", c.username, c.password))
		if ve, ok := errors.AsType[*validate.Error](err); !ok || ve.Field != c.field {
			t.Errorf("%s/%s: esperaba un error de %s, got %v", c.username, c.password, c.field, err)
		}
	}
	if _, err := f.acceptInvite.Execute(ctx, accept("inv", "ana", testPassword)); err != nil {
		t.Fatalf("la invitación sigue disponible: %v", err)
	}
}

func TestAcceptInviteAudit(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.invite(t, "inv", nil, time.Hour)
	res, err := f.acceptInvite.Execute(ctx, accept("inv", "ana", testPassword))
	if err != nil {
		t.Fatal(err)
	}
	// El actor de user.create es el usuario recién creado.
	rows := f.auditRows(t)
	if len(rows) != 1 || rows[0].Action != "user.create" || rows[0].Target != "user:ana" ||
		rows[0].ActorType != "user" || rows[0].ActorID == nil || *rows[0].ActorID != res.UserID || rows[0].Details != "{}" {
		t.Fatalf("audit=%+v", rows)
	}
}

func TestSetup(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	token, _, _ := f.ensureSetup.Execute(ctx)

	if _, err := f.setup.Execute(ctx, accept(token, "felipe", "corta")); err == nil {
		t.Fatal("contraseña corta debió fallar")
	}
	res, err := f.setup.Execute(ctx, accept(token, "felipe", testPassword))
	if err != nil || res.Username != "felipe" || res.Token == "" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if rows := f.auditRows(t); len(rows) != 1 || rows[0].Action != "user.create" || *rows[0].ActorID != res.UserID {
		t.Fatalf("audit=%+v", rows)
	}

	// Con un usuario creado, setup ya no funciona: ni con otro link de
	// sistema, ni con un cuerpo inválido.
	f.invite(t, "otra", nil, time.Hour)
	if _, err := f.setup.Execute(ctx, accept("otra", "otro", testPassword)); !errors.Is(err, service.ErrAlreadySetUp) {
		t.Fatalf("esperaba ErrAlreadySetUp, got %v", err)
	}
	if _, err := f.setup.Execute(ctx, accept("otra", "otro", "corta")); !errors.Is(err, service.ErrAlreadySetUp) {
		t.Fatalf("esperaba ErrAlreadySetUp, got %v", err)
	}
	if n, _ := f.users.Count(ctx); n != 1 {
		t.Fatalf("users=%d", n)
	}
}

func TestCreateInviteAudit(t *testing.T) {
	f := newFixture(t)
	felipe := f.user(t, "felipe")
	ctx := withUser(felipe)
	if _, err := f.createInvite.Execute(ctx, dto.CreateInviteInput{CreatedBy: felipe.ID, CreatorUsername: "felipe"}); err != nil {
		t.Fatal(err)
	}
	rows := f.auditRows(t)
	if len(rows) != 1 || rows[0].Action != "invite.create" || rows[0].Target != "user:felipe" || *rows[0].ActorID != felipe.ID {
		t.Fatalf("audit=%+v", rows)
	}
}

func (f *fixture) countSessions(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.db.Conn(context.Background()).QueryRowContext(context.Background(), `SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
