package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestDB es igual a sqlitetest.New, que no se puede importar desde aquí (ciclo).
func newTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "muelle.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return d
}

func countProjects(t *testing.T, d *DB) int {
	t.Helper()
	var n int
	if err := d.Conn(context.Background()).QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM projects`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func insertProject(ctx context.Context, d *DB, slug string) error {
	_, err := d.Conn(ctx).ExecContext(ctx, `INSERT INTO projects (slug, name, created_at) VALUES (?, ?, 0)`, slug, slug)
	return err
}

func TestMigrateCreatesTables(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	for _, table := range []string{
		"projects", "stages", "apps", "env_vars", "users", "invites", "sessions", "api_tokens", "audit_events", "settings",
	} {
		var name string
		err := d.Conn(ctx).QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Errorf("falta la tabla %s: %v", table, err)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("segunda migración: %v", err)
	}
	var n int
	if err := d.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("schema_migrations tiene %d filas, esperaba 1", n)
	}
}

func TestPragmas(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	var fk int
	if err := d.Conn(ctx).QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign_keys=%d err=%v", fk, err)
	}
	var mode string
	if err := d.Conn(ctx).QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal_mode=%q err=%v", mode, err)
	}
	_, err := d.Conn(ctx).ExecContext(ctx, `INSERT INTO stages (project_id, slug, created_at) VALUES (999, 'x', 0)`)
	if err == nil {
		t.Fatal("foreign_keys debería rechazar un project_id inexistente")
	}
}

func TestInTxCommitAndRollback(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()

	err := d.InTx(ctx, func(ctx context.Context) error { return insertProject(ctx, d, "queda") })
	if err != nil {
		t.Fatal(err)
	}
	if n := countProjects(t, d); n != 1 {
		t.Fatalf("después del commit hay %d proyectos, esperaba 1", n)
	}

	boom := errors.New("boom")
	err = d.InTx(ctx, func(ctx context.Context) error {
		if err := insertProject(ctx, d, "no-queda"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("InTx devolvió %v, esperaba el error de fn", err)
	}
	if n := countProjects(t, d); n != 1 {
		t.Fatalf("después del rollback hay %d proyectos, esperaba 1", n)
	}
}

func TestInTxNested(t *testing.T) {
	d := newTestDB(t)
	boom := errors.New("boom")
	err := d.InTx(context.Background(), func(outer context.Context) error {
		if err := insertProject(outer, d, "externo"); err != nil {
			return err
		}
		err := d.InTx(outer, func(inner context.Context) error {
			if d.Conn(inner) != d.Conn(outer) {
				t.Error("el InTx interno debe usar la misma transacción")
			}
			// El interno ve lo que el externo todavía no confirma.
			var n int
			if err := d.Conn(inner).QueryRowContext(inner, `SELECT COUNT(*) FROM projects`).Scan(&n); err != nil {
				return err
			}
			if n != 1 {
				t.Errorf("el InTx interno ve %d proyectos, esperaba 1", n)
			}
			return insertProject(inner, d, "interno")
		})
		if err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("InTx devolvió %v", err)
	}
	if n := countProjects(t, d); n != 0 {
		t.Fatalf("el rollback del externo debió deshacer todo, quedan %d proyectos", n)
	}
}

func TestInTxRollsBackOnPanic(t *testing.T) {
	d := newTestDB(t)
	func() {
		defer func() {
			if r := recover(); r != "pánico" {
				t.Fatalf("InTx debe re-lanzar el panic, recover() = %v", r)
			}
		}()
		d.InTx(context.Background(), func(ctx context.Context) error {
			if err := insertProject(ctx, d, "no-queda"); err != nil {
				t.Fatal(err)
			}
			panic("pánico")
		})
	}()
	if n := countProjects(t, d); n != 0 {
		t.Fatalf("el panic debió hacer rollback, quedan %d proyectos", n)
	}
	// La conexión quedó libre: se puede volver a escribir.
	if err := insertProject(context.Background(), d, "despues"); err != nil {
		t.Fatal(err)
	}
}

func TestConnOutsideTx(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	if _, ok := d.Conn(ctx).(*sql.DB); !ok {
		t.Fatalf("fuera de una transacción Conn debe ser la *sql.DB, es %T", d.Conn(ctx))
	}
	if err := insertProject(ctx, d, "suelto"); err != nil {
		t.Fatal(err)
	}
	if n := countProjects(t, d); n != 1 {
		t.Fatalf("hay %d proyectos", n)
	}
}

// Una transacción de una base no se filtra a otra base con el mismo ctx.
func TestConnIgnoresTxOfOtherDB(t *testing.T) {
	a, b := newTestDB(t), newTestDB(t)
	err := a.InTx(context.Background(), func(ctx context.Context) error {
		if _, ok := b.Conn(ctx).(*sql.DB); !ok {
			t.Errorf("b.Conn dentro de una transacción de a es %T", b.Conn(ctx))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAutoincrementDoesNotReuseIDs(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	insert := func(name string) int64 {
		t.Helper()
		res, err := d.Conn(ctx).ExecContext(ctx,
			`INSERT INTO users (username, password_hash, created_at) VALUES (?, 'x', 0)`, name)
		if err != nil {
			t.Fatal(err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	first, second := insert("ana"), insert("beto")
	if _, err := d.Conn(ctx).ExecContext(ctx, `DELETE FROM users WHERE id = ?`, second); err != nil {
		t.Fatal(err)
	}
	if third := insert("carla"); third <= first || third <= second {
		t.Fatalf("ids %d, %d y luego %d: el id del borrado se reutilizó", first, second, third)
	}

	// Lo mismo vale para stages y apps, cuyos ids también van en la AAD.
	for _, table := range []string{"users", "stages", "apps"} {
		var sqlText string
		if err := d.Conn(ctx).QueryRowContext(ctx,
			`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&sqlText); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.ToUpper(sqlText), "AUTOINCREMENT") {
			t.Errorf("la tabla %s no usa AUTOINCREMENT", table)
		}
	}
}

func TestIsUniqueAndForeignKey(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	if err := insertProject(ctx, d, "auditfy"); err != nil {
		t.Fatal(err)
	}
	dup := insertProject(ctx, d, "auditfy")
	if !IsUnique(dup) || IsForeignKey(dup) {
		t.Fatalf("slug duplicado: IsUnique=%v IsForeignKey=%v err=%v", IsUnique(dup), IsForeignKey(dup), dup)
	}

	_, fk := d.Conn(ctx).ExecContext(ctx, `INSERT INTO stages (project_id, slug, created_at) VALUES (999, 'x', 0)`)
	if !IsForeignKey(fk) || IsUnique(fk) {
		t.Fatalf("project_id inexistente: IsForeignKey=%v IsUnique=%v err=%v", IsForeignKey(fk), IsUnique(fk), fk)
	}

	// Una clave primaria repetida también cuenta como único.
	if _, err := d.Conn(ctx).ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('a', 'x')`); err != nil {
		t.Fatal(err)
	}
	_, pk := d.Conn(ctx).ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('a', 'y')`)
	if !IsUnique(pk) {
		t.Fatalf("clave primaria repetida: IsUnique=false err=%v", pk)
	}

	if IsUnique(nil) || IsForeignKey(nil) || IsUnique(errors.New("x")) || IsForeignKey(sql.ErrNoRows) {
		t.Fatal("un error que no es de SQLite no es de restricción")
	}
}

func TestTimeHelpers(t *testing.T) {
	at := time.Date(2026, 10, 9, 12, 30, 0, 123_456_789, time.FixedZone("CLT", -3*3600))
	ms := Millis(at)
	if ms != at.UnixMilli() {
		t.Fatalf("Millis = %d", ms)
	}
	back := Time(ms)
	if back.Location() != time.UTC || !back.Equal(at.Truncate(time.Millisecond)) {
		t.Fatalf("Time(%d) = %v", ms, back)
	}

	if v := NullMillis(nil); v.Valid {
		t.Fatal("NullMillis(nil) debe ser NULL")
	}
	if v := NullMillis(&at); !v.Valid || v.Int64 != ms {
		t.Fatalf("NullMillis = %+v", v)
	}
	if NullTime(sql.NullInt64{}) != nil {
		t.Fatal("NullTime(NULL) debe ser nil")
	}
	if p := NullTime(sql.NullInt64{Int64: ms, Valid: true}); p == nil || !p.Equal(back) || p.Location() != time.UTC {
		t.Fatalf("NullTime = %v", p)
	}
}
