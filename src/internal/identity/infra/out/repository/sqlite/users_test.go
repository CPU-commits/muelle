package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
)

func TestUsers(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	st := r.users

	if n, _ := st.Count(ctx); n != 0 {
		t.Fatalf("count=%d", n)
	}
	felipe, err := st.Create(ctx, entity.User{Username: "felipe", PasswordHash: "hash1", CreatedAt: t0})
	if err != nil || felipe.ID == 0 || felipe.CreatedBy != nil || felipe.HasTOTP() || !felipe.CreatedAt.Equal(t0) {
		t.Fatalf("felipe=%+v err=%v", felipe, err)
	}
	ana, err := st.Create(ctx, entity.User{Username: "ana", PasswordHash: "hash2", CreatedBy: &felipe.ID, CreatedAt: t0})
	if err != nil || ana.CreatedBy == nil || *ana.CreatedBy != felipe.ID || ana.PasswordHash != "hash2" {
		t.Fatalf("ana=%+v err=%v", ana, err)
	}
	if _, err := st.Create(ctx, entity.User{Username: "ana", PasswordHash: "x", CreatedAt: t0}); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("esperaba ErrConflict, got %v", err)
	}
	if n, _ := st.Count(ctx); n != 2 {
		t.Fatalf("count=%d", n)
	}
	list, _ := st.List(ctx)
	if len(list) != 2 || list[0].Username != "ana" || list[1].Username != "felipe" {
		t.Fatalf("list=%+v", list)
	}
	if _, err := st.GetByUsername(ctx, "nadie"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
	if _, err := st.GetByID(ctx, 999); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
	if got, _ := st.GetByUsername(ctx, "felipe"); got.ID != felipe.ID {
		t.Fatalf("got=%+v", got)
	}

	// Fallos de login y bloqueo.
	if n, _ := st.IncrementFailedLogins(ctx, ana.ID); n != 1 {
		t.Fatalf("n=%d", n)
	}
	if n, _ := st.IncrementFailedLogins(ctx, ana.ID); n != 2 {
		t.Fatalf("n=%d", n)
	}
	if _, err := st.IncrementFailedLogins(ctx, 999); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("usuario inexistente: esperaba ErrNotFound, got %v", err)
	}
	if err := st.SetLockedUntil(ctx, ana.ID, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	ana, _ = st.GetByID(ctx, ana.ID)
	if ana.FailedLogins != 2 || ana.LockedUntil == nil || !ana.LockedUntil.Equal(t0.Add(time.Minute)) {
		t.Fatalf("ana=%+v", ana)
	}
	if err := st.ResetLoginFailures(ctx, ana.ID); err != nil {
		t.Fatal(err)
	}
	ana, _ = st.GetByID(ctx, ana.ID)
	if ana.FailedLogins != 0 || ana.LockedUntil != nil {
		t.Fatalf("ana=%+v", ana)
	}

	// TOTP: pendiente → activo → borrado.
	if err := st.ActivateTOTP(ctx, ana.ID, []byte("activo")); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("sin pendiente: esperaba ErrNotFound, got %v", err)
	}
	if err := st.SetTOTPPending(ctx, ana.ID, []byte("pendiente")); err != nil {
		t.Fatal(err)
	}
	ana, _ = st.GetByID(ctx, ana.ID)
	if !ana.HasPendingTOTP() || string(ana.TOTPPending) != "pendiente" || ana.HasTOTP() {
		t.Fatalf("ana=%+v", ana)
	}
	if err := st.ActivateTOTP(ctx, ana.ID, []byte("activo")); err != nil {
		t.Fatal(err)
	}
	ana, _ = st.GetByID(ctx, ana.ID)
	if !ana.HasTOTP() || string(ana.TOTPSecret) != "activo" || ana.TOTPPending != nil {
		t.Fatalf("ana=%+v", ana)
	}
	if err := st.ClearTOTP(ctx, ana.ID); err != nil {
		t.Fatal(err)
	}
	ana, _ = st.GetByID(ctx, ana.ID)
	if ana.HasTOTP() || ana.HasPendingTOTP() {
		t.Fatal("ClearTOTP no borró los secretos")
	}
	if err := st.ClearTOTP(ctx, 999); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}

	// Borrar a felipe deja created_by de ana en NULL.
	if err := st.Delete(ctx, felipe.ID); err != nil {
		t.Fatal(err)
	}
	ana, _ = st.GetByID(ctx, ana.ID)
	if ana.CreatedBy != nil {
		t.Fatal("created_by debió quedar NULL")
	}
	if err := st.Delete(ctx, felipe.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
}

func TestUsersInTxRollback(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	boom := errors.New("boom")
	err := r.db.InTx(ctx, func(ctx context.Context) error {
		if _, err := r.users.Create(ctx, entity.User{Username: "felipe", PasswordHash: "h", CreatedAt: t0}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	if n, _ := r.users.Count(ctx); n != 0 {
		t.Fatalf("el usuario debió deshacerse con el rollback: count=%d", n)
	}
}
