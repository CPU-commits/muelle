package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// withUser es el ctx de una petición hecha por u (para la auditoría).
func withUser(u entity.User) context.Context {
	return shared.WithPrincipal(context.Background(), shared.Principal{UserID: u.ID, Username: u.Username})
}

func TestListUsers(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if users, err := f.listUsers.Execute(ctx); err != nil || users == nil || len(users) != 0 {
		t.Fatalf("sin usuarios: users=%#v err=%v", users, err)
	}
	admin := f.user(t, "admin")
	f.user(t, "ana")
	f.enableTOTPFor(t, admin.ID)

	users, err := f.listUsers.Execute(ctx)
	if err != nil || len(users) != 2 {
		t.Fatalf("users=%+v err=%v", users, err)
	}
	if users[0].Username != "admin" || !users[0].TOTPEnabled || !users[0].CreatedAt.Equal(admin.CreatedAt) ||
		users[1].Username != "ana" || users[1].TOTPEnabled {
		t.Fatalf("users=%+v", users)
	}
}

func TestDeleteUserAndResetTOTP(t *testing.T) {
	f := newFixture(t)
	admin := f.user(t, "admin")
	ana := f.user(t, "ana")
	f.enableTOTPFor(t, ana.ID)
	ctx := withUser(admin)
	before := len(f.auditRows(t)) // user.totp_enable de la preparación

	if err := f.deleteUser.Execute(ctx, dto.DeleteUserInput{Username: "admin", ActorUsername: "admin"}); !errors.Is(err, service.ErrCannotDeleteSelf) {
		t.Fatalf("esperaba ErrCannotDeleteSelf, got %v", err)
	}
	if err := f.resetTOTP.Execute(ctx, "ana"); err != nil {
		t.Fatal(err)
	}
	if f.reload(t, ana.ID).HasTOTP() {
		t.Fatal("el 2FA de ana debió borrarse")
	}
	if err := f.resetTOTP.Execute(ctx, "nadie"); !errors.Is(err, sharedrepo.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
	if err := f.deleteUser.Execute(ctx, dto.DeleteUserInput{Username: "ana", ActorUsername: "admin"}); err != nil {
		t.Fatal(err)
	}
	if err := f.deleteUser.Execute(ctx, dto.DeleteUserInput{Username: "ana", ActorUsername: "admin"}); !errors.Is(err, sharedrepo.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}

	var actions []string
	for _, r := range f.auditRows(t)[before:] {
		if r.ActorType != "user" || *r.ActorID != admin.ID || r.Target != "user:ana" {
			t.Fatalf("audit=%+v", r)
		}
		actions = append(actions, r.Action)
	}
	if len(actions) != 2 || actions[0] != "user.totp_reset" || actions[1] != "user.delete" {
		t.Fatalf("actions=%v", actions)
	}
}

func TestDeleteUserDeletesSessions(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.user(t, "admin")
	f.user(t, "ana")
	res, _ := f.login.Execute(ctx, login("ana", testPassword, "1.1.1.1"))
	if err := f.deleteUser.Execute(ctx, dto.DeleteUserInput{Username: "ana", ActorUsername: "admin"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.authSession.Execute(ctx, res.Token); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("la sesión de un usuario borrado no sirve, got %v", err)
	}
}
