package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
)

func TestInvites(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	felipe := r.user(t, "felipe", nil)

	if err := r.invites.Create(ctx, entity.NewInvite("inv1", &felipe.ID, t0, 24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := r.invites.Create(ctx, entity.NewInvite("inv1", nil, t0, time.Hour)); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("hash repetido: esperaba ErrConflict, got %v", err)
	}
	inv, err := r.invites.GetByTokenHash(ctx, "inv1")
	if err != nil || inv.ID == 0 || inv.TokenHash != "inv1" || inv.CreatedBy == nil || *inv.CreatedBy != felipe.ID ||
		!inv.ExpiresAt.Equal(t0.Add(24*time.Hour)) || inv.UsedAt != nil {
		t.Fatalf("inv=%+v err=%v", inv, err)
	}
	if _, err := r.invites.GetByTokenHash(ctx, "no-existe"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}

	// Se marca como usada una sola vez.
	if err := r.invites.MarkUsed(ctx, inv.ID, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	inv, _ = r.invites.GetByTokenHash(ctx, "inv1")
	if inv.UsedAt == nil || !inv.UsedAt.Equal(t0.Add(time.Minute)) {
		t.Fatalf("used_at=%v", inv.UsedAt)
	}
	if err := r.invites.MarkUsed(ctx, inv.ID, t0.Add(2*time.Minute)); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("ya usada: esperaba ErrNotFound, got %v", err)
	}
	if err := r.invites.MarkUsed(ctx, 999, t0); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("inexistente: esperaba ErrNotFound, got %v", err)
	}
}

// Un creador que ya no existe (lo borraron mientras tanto) no es un error 500.
func TestCreateInviteWithMissingCreator(t *testing.T) {
	r := newRepos(t)
	missing := int64(999)
	if err := r.invites.Create(context.Background(), entity.NewInvite("inv", &missing, t0, time.Hour)); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
}

func TestDeleteUnusedSystemInvites(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	felipe := r.user(t, "felipe", nil)
	r.invites.Create(ctx, entity.NewInvite("sys", nil, t0, time.Hour))
	r.invites.Create(ctx, entity.NewInvite("sys-usada", nil, t0, time.Hour))
	r.invites.Create(ctx, entity.NewInvite("user", &felipe.ID, t0, time.Hour))
	used, _ := r.invites.GetByTokenHash(ctx, "sys-usada")
	r.invites.MarkUsed(ctx, used.ID, t0)

	if err := r.invites.DeleteUnusedSystem(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := r.invites.GetByTokenHash(ctx, "sys"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("la invitación del sistema debió borrarse, got %v", err)
	}
	if _, err := r.invites.GetByTokenHash(ctx, "sys-usada"); err != nil {
		t.Fatalf("una invitación ya usada queda como registro: %v", err)
	}
	if _, err := r.invites.GetByTokenHash(ctx, "user"); err != nil {
		t.Fatalf("la invitación de un usuario no se toca: %v", err)
	}
}

func TestDeleteUserCascadesInvites(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	felipe := r.user(t, "felipe", nil)
	r.invites.Create(ctx, entity.NewInvite("user", &felipe.ID, t0, time.Hour))
	r.users.Delete(ctx, felipe.ID)
	if _, err := r.invites.GetByTokenHash(ctx, "user"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("la invitación debió borrarse con su creador, got %v", err)
	}
}
