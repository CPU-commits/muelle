package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/admin/admintest"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

func TestTOTPStatus(t *testing.T) {
	db := sqlitetest.New(t)
	s := NewTOTPStatus(db)
	ctx := context.Background()
	with := admintest.InsertUser(t, db, "con-totp", true)
	without := admintest.InsertUser(t, db, "sin-totp", false)

	for id, want := range map[int64]bool{with: true, without: false, 999: false} {
		got, err := s.HasTOTP(ctx, id)
		if err != nil || got != want {
			t.Errorf("HasTOTP(%d)=%v err=%v, esperaba %v", id, got, err, want)
		}
	}
}

func TestBackupStore(t *testing.T) {
	db := sqlitetest.New(t)
	ctx := context.Background()
	admintest.InsertUser(t, db, "ana", false)
	dir := filepath.Join(t.TempDir(), "backups")
	s := NewBackupStore(db, dir)

	if names, err := s.List(ctx); err != nil || len(names) != 0 {
		t.Fatalf("names=%v err=%v", names, err)
	}
	for _, n := range []string{"muelle-2026-10-02.db", "muelle-2026-10-01.db"} {
		if err := s.Create(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	names, err := s.List(ctx)
	if err != nil || len(names) != 2 || names[0] != "muelle-2026-10-01.db" || names[1] != "muelle-2026-10-02.db" {
		t.Fatalf("names=%v err=%v", names, err)
	}
	// Un archivo ya existente no se pisa.
	if err := s.Create(ctx, "muelle-2026-10-01.db"); err == nil {
		t.Fatal("debía fallar al repetir el nombre")
	}
	if names, _ := s.List(ctx); len(names) != 2 {
		t.Fatalf("names=%v: no debe quedar un temporal visible", names)
	}

	// La copia es una base válida con los datos.
	cp, err := sqlite.Open(filepath.Join(dir, names[1]))
	if err != nil {
		t.Fatal(err)
	}
	defer cp.Close()
	var n int
	if err := cp.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE username = 'ana'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("la copia no tiene al usuario: n=%d err=%v", n, err)
	}

	if err := s.Remove(ctx, names[0]); err != nil {
		t.Fatal(err)
	}
	if names, _ := s.List(ctx); len(names) != 1 {
		t.Fatalf("names=%v", names)
	}
	if s.Remove(ctx, "../x.db") == nil || s.Create(ctx, "../x.db") == nil {
		t.Fatal("debe rechazar nombres con ruta")
	}
}
