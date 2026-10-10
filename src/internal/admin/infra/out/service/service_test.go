package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	if entries, _ := os.ReadDir(dir); len(entries) != 2 {
		t.Fatalf("entries=%v: no debe quedar un temporal", entries)
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

func TestBackupStorePermissions(t *testing.T) {
	db := sqlitetest.New(t)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "backups")
	// El directorio ya existía con permisos abiertos.
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil { // por si el umask lo recortó
		t.Fatal(err)
	}
	s := NewBackupStore(db, dir)
	if err := s.Create(ctx, "muelle-2026-10-01.db"); err != nil {
		t.Fatal(err)
	}
	di, err := os.Stat(dir)
	if err != nil || di.Mode().Perm() != 0o700 {
		t.Fatalf("dir: %v err=%v", di.Mode(), err)
	}
	fi, err := os.Stat(filepath.Join(dir, "muelle-2026-10-01.db"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("archivo: %v err=%v", fi.Mode(), err)
	}
	// Un directorio nuevo también queda 0700.
	dir2 := filepath.Join(t.TempDir(), "nuevo", "backups")
	if err := NewBackupStore(db, dir2).Create(ctx, "muelle-2026-10-01.db"); err != nil {
		t.Fatal(err)
	}
	if di, _ := os.Stat(dir2); di.Mode().Perm() != 0o700 {
		t.Fatalf("dir nuevo: %v", di.Mode())
	}
}

func TestBackupStoreCancelledLeavesNothing(t *testing.T) {
	db := sqlitetest.New(t)
	ctx := context.Background()
	// Unos MB de datos para que VACUUM INTO tarde más que el plazo.
	if _, err := db.Conn(ctx).ExecContext(ctx, `CREATE TABLE relleno (v BLOB)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Conn(ctx).ExecContext(ctx,
		`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < 6000)
		 INSERT INTO relleno SELECT randomblob(2000) FROM n`); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "backups")
	s := NewBackupStore(db, dir)

	cctx, cancel := context.WithTimeout(ctx, 5*time.Millisecond)
	defer cancel()
	if err := s.Create(cctx, "muelle-2026-10-01.db"); err == nil {
		t.Fatal("VACUUM INTO terminó antes del plazo: no se probó la limpieza")
	}
	if names, err := s.List(ctx); err != nil || len(names) != 0 {
		t.Fatalf("names=%v err=%v", names, err)
	}
	left, _ := os.ReadDir(dir)
	if len(left) != 0 {
		t.Fatalf("quedaron archivos: %v", left)
	}
}

func TestBackupStoreConcurrentSameName(t *testing.T) {
	db := sqlitetest.New(t)
	ctx := context.Background()
	admintest.InsertUser(t, db, "ana", false)
	dir := filepath.Join(t.TempDir(), "backups")
	s := NewBackupStore(db, dir)

	const n = 4
	errs := make(chan error, n)
	for range n {
		go func() { errs <- s.Create(ctx, "muelle-2026-10-01.db") }()
	}
	ok := 0
	for range n {
		if <-errs == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("%d Create exitosos, esperaba 1", ok)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "muelle-2026-10-01.db" {
		t.Fatalf("entries=%v", entries)
	}
	cp, err := sqlite.Open(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	defer cp.Close()
	var c int
	if err := cp.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&c); err != nil || c != 1 {
		t.Fatalf("copia inválida: c=%d err=%v", c, err)
	}
}

func TestBackupStoreIgnoresStrayFiles(t *testing.T) {
	db := sqlitetest.New(t)
	ctx := context.Background()
	dir := t.TempDir()
	stray := filepath.Join(dir, "muelle-manual-antes-de-migrar.db")
	if err := os.WriteFile(stray, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewBackupStore(db, dir)
	if err := s.Create(ctx, "muelle-2026-10-01.db"); err != nil {
		t.Fatal(err)
	}
	names, err := s.List(ctx)
	if err != nil || len(names) != 1 || names[0] != "muelle-2026-10-01.db" {
		t.Fatalf("names=%v err=%v", names, err)
	}
}
