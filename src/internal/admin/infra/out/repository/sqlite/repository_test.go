package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	shared "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

func TestAudit(t *testing.T) {
	db := sqlitetest.New(t)
	clk := clock.NewManual(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	rec := sqlite.NewAuditRecorder(db, clk, nil)
	repo := NewAuditRepository(db)
	ctx := context.Background()

	rec.Record(entity.WithPrincipal(ctx, entity.Principal{UserID: 7}), "project.create", "project:a", nil)
	clk.Advance(time.Hour)
	rec.Record(entity.WithPrincipal(ctx, entity.Principal{Token: &entity.TokenScope{ID: 3}}),
		"env.replace", "app:a/s/x", map[string][]string{"keys": {"A"}})

	list, err := repo.List(ctx, 0, 10)
	if err != nil || len(list) != 2 || list[0].Action != "env.replace" || string(list[0].Details) != `{"keys":["A"]}` {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if list[0].ActorType != "token" || list[0].ActorID == nil || *list[0].ActorID != 3 {
		t.Fatalf("evento 0=%+v", list[0])
	}
	if string(list[1].Details) != "{}" || list[1].ActorID == nil || *list[1].ActorID != 7 || list[1].ActorType != "user" {
		t.Fatalf("evento 1=%+v", list[1])
	}
	if !list[1].At.Equal(clk.Now().Add(-time.Hour)) {
		t.Fatalf("at=%v", list[1].At)
	}

	older, _ := repo.List(ctx, list[0].ID, 10)
	if len(older) != 1 || older[0].ID != list[1].ID {
		t.Fatalf("older=%+v", older)
	}
	if one, _ := repo.List(ctx, 0, 1); len(one) != 1 || one[0].ID != list[0].ID {
		t.Fatalf("limit=%+v", one)
	}

	n, err := repo.DeleteBefore(ctx, clk.Now().Add(-time.Minute))
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if left, _ := repo.List(ctx, 0, 10); len(left) != 1 || left[0].ID != list[0].ID {
		t.Fatalf("left=%+v", left)
	}
}

func TestAuditEmptyListIsNotNil(t *testing.T) {
	list, err := NewAuditRepository(sqlitetest.New(t)).List(context.Background(), 0, 10)
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("list=%v err=%v", list, err)
	}
}

func TestSettings(t *testing.T) {
	repo := NewSettingsRepository(sqlitetest.New(t))
	ctx := context.Background()

	if _, _, err := repo.Get(ctx, "require_2fa"); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
	for _, v := range []string{"true", "false"} {
		if err := repo.Set(ctx, "require_2fa", []byte(v), false); err != nil {
			t.Fatal(err)
		}
	}
	v, enc, err := repo.Get(ctx, "require_2fa")
	if err != nil || enc || string(v) != "false" {
		t.Fatalf("v=%q enc=%v err=%v", v, enc, err)
	}

	// Cifrado: se guarda el blob tal cual y la marca encrypted.
	if err := repo.Set(ctx, "x", []byte{0, 1, 2, 255}, true); err != nil {
		t.Fatal(err)
	}
	v, enc, err = repo.Get(ctx, "x")
	if err != nil || !enc || string(v) != "\x00\x01\x02\xff" {
		t.Fatalf("v=%q enc=%v err=%v", v, enc, err)
	}
	// Volver a sin cifrar limpia la marca.
	if err := repo.Set(ctx, "x", []byte("plano"), false); err != nil {
		t.Fatal(err)
	}
	if _, enc, _ := repo.Get(ctx, "x"); enc {
		t.Fatal("encrypted debe volver a 0")
	}
}

func TestSettingsInsertDoesNotOverwrite(t *testing.T) {
	repo := NewSettingsRepository(sqlitetest.New(t))
	ctx := context.Background()
	if err := repo.Insert(ctx, "k", []byte{1, 2}, true); err != nil {
		t.Fatal(err)
	}
	if err := repo.Insert(ctx, "k", []byte("otro"), false); !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	v, enc, err := repo.Get(ctx, "k")
	if err != nil || !enc || string(v) != "\x01\x02" {
		t.Fatalf("v=%q enc=%v err=%v", v, enc, err)
	}
}

func TestSettingsPlainValueIsText(t *testing.T) {
	db := sqlitetest.New(t)
	NewSettingsRepository(db).Set(context.Background(), "require_2fa", []byte("true"), false)
	var n int
	err := db.Conn(context.Background()).QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM settings WHERE key = 'require_2fa' AND value = 'true'`).Scan(&n)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v: el valor sin cifrar debe ser texto", n, err)
	}
}
