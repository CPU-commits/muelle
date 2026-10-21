package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sqlite3 "modernc.org/sqlite/lib"

	"github.com/CPU-commits/muelle/src/shared/domain/repository"
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
		"deploys", "registries", // 0002, Fase 2
		"domains", // 0003, Fase 3
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
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	if n != len(entries) || n < 3 {
		t.Fatalf("schema_migrations tiene %d filas, esperaba una por migración (%d)", n, len(entries))
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

// Un InTx anidado no tiene savepoints: si el interno devuelve un error y el
// externo lo ignora, lo que escribió el interno se confirma igual.
func TestInTxNestedSwallowedErrorStillCommits(t *testing.T) {
	d := newTestDB(t)
	boom := errors.New("boom")
	err := d.InTx(context.Background(), func(outer context.Context) error {
		if err := insertProject(outer, d, "externo"); err != nil {
			return err
		}
		innerErr := d.InTx(outer, func(inner context.Context) error {
			if err := insertProject(inner, d, "interno"); err != nil {
				return err
			}
			return boom
		})
		if !errors.Is(innerErr, boom) {
			t.Errorf("el InTx interno devolvió %v", innerErr)
		}
		return nil // el externo se traga el error del interno
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := countProjects(t, d); n != 2 {
		t.Fatalf("hay %d proyectos: sin savepoints, lo del interno se confirma con el externo", n)
	}
}

// El ctx de fn sigue apuntando a su transacción aunque sobreviva a InTx (ej.
// una goroutine con context.WithoutCancel): sus consultas fallan con
// sql.ErrTxDone en vez de escribir en silencio fuera de la transacción.
func TestFnContextAfterTxFinishes(t *testing.T) {
	d := newTestDB(t)
	boom := errors.New("boom")
	for _, fail := range []bool{false, true} {
		var leaked context.Context
		err := d.InTx(context.Background(), func(ctx context.Context) error {
			leaked = context.WithoutCancel(ctx)
			if fail {
				return boom
			}
			return nil
		})
		if fail != errors.Is(err, boom) {
			t.Fatalf("fail=%v: InTx devolvió %v", fail, err)
		}

		before := countProjects(t, d)
		done := make(chan error)
		go func() { done <- insertProject(leaked, d, fmt.Sprintf("goroutine-%v", fail)) }()
		if err := <-done; !errors.Is(err, sql.ErrTxDone) {
			t.Fatalf("fail=%v: escribir con el ctx de una transacción terminada devolvió %v, esperaba sql.ErrTxDone", fail, err)
		}
		// Un InTx con ese ctx tampoco abre otra transacción por su cuenta.
		err = d.InTx(leaked, func(ctx context.Context) error {
			return insertProject(ctx, d, fmt.Sprintf("tx-%v", fail))
		})
		if !errors.Is(err, sql.ErrTxDone) {
			t.Fatalf("fail=%v: InTx con el ctx de una transacción terminada devolvió %v, esperaba sql.ErrTxDone", fail, err)
		}
		if n := countProjects(t, d); n != before {
			t.Fatalf("fail=%v: hay %d proyectos, esperaba %d: algo se escribió fuera de la transacción", fail, n, before)
		}
	}
}

// Un InTx anidado en una goroutine cuya transacción externa termina a mitad de
// camino falla con sql.ErrTxDone: lo que viene después nunca se confirma solo.
func TestNestedInTxInGoroutineAfterOuterEnds(t *testing.T) {
	d := newTestDB(t)
	started, proceed := make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	err := d.InTx(context.Background(), func(outer context.Context) error {
		go func() {
			result <- d.InTx(outer, func(inner context.Context) error {
				// Esto todavía es parte de la transacción externa.
				if err := insertProject(inner, d, "antes"); err != nil {
					return err
				}
				close(started)
				<-proceed // mientras tanto, la externa se confirma
				return insertProject(inner, d, "despues")
			})
		}()
		<-started
		return insertProject(outer, d, "externo")
	})
	if err != nil {
		t.Fatal(err)
	}
	close(proceed)
	if err := <-result; !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("el InTx de la goroutine devolvió %v, esperaba sql.ErrTxDone", err)
	}
	var n int
	if err := d.Conn(context.Background()).QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM projects WHERE slug = 'despues'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("lo que la goroutine escribió después de terminar la transacción se confirmó solo")
	}
	// Sin savepoints, lo de antes quedó con la externa.
	if n := countProjects(t, d); n != 2 {
		t.Fatalf("hay %d proyectos, esperaba 2 (externo y antes)", n)
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

	// Lo mismo vale para stages y apps, cuyos ids también van en la AAD, y
	// para api_tokens, cuyo id queda en la auditoría (actor_id) y en DELETE /tokens/{id}.
	for _, table := range []string{"users", "stages", "apps", "api_tokens"} {
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

// ON DELETE RESTRICT no da SQLITE_CONSTRAINT_FOREIGNKEY (787) sino
// SQLITE_CONSTRAINT_TRIGGER (1811) con el mensaje de clave foránea.
func TestIsForeignKeyRestrict(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	if err := insertProject(ctx, d, "auditfy"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Conn(ctx).ExecContext(ctx,
		`INSERT INTO stages (project_id, slug, created_at) SELECT id, 'staging', 0 FROM projects`); err != nil {
		t.Fatal(err)
	}
	_, err := d.Conn(ctx).ExecContext(ctx, `DELETE FROM projects WHERE slug = 'auditfy'`)
	if err == nil {
		t.Fatal("RESTRICT debió impedir borrar un proyecto con stages")
	}
	if errorCode(err) != sqlite3.SQLITE_CONSTRAINT_TRIGGER {
		t.Fatalf("borrar con RESTRICT: código %d, esperaba 1811 (si cambió, revisar IsForeignKey)", errorCode(err))
	}
	if !IsForeignKey(err) || IsUnique(err) {
		t.Fatalf("borrar con RESTRICT: IsForeignKey=%v IsUnique=%v err=%v (código %d)", IsForeignKey(err), IsUnique(err), err, errorCode(err))
	}

	// Otro error de trigger (1811) no es de clave foránea.
	if _, err := d.Conn(ctx).ExecContext(ctx,
		`CREATE TRIGGER settings_ro BEFORE INSERT ON settings BEGIN SELECT RAISE(ABORT, 'prohibido'); END`); err != nil {
		t.Fatal(err)
	}
	_, err = d.Conn(ctx).ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('a', 'x')`)
	if errorCode(err) != sqlite3.SQLITE_CONSTRAINT_TRIGGER || IsForeignKey(err) {
		t.Fatalf("un RAISE de un trigger no es de clave foránea: IsForeignKey=%v err=%v (código %d)", IsForeignKey(err), err, errorCode(err))
	}
}

func TestNotFound(t *testing.T) {
	if NotFound(nil) != nil {
		t.Fatal("NotFound(nil) debe ser nil")
	}
	other := errors.New("disco lleno")
	if NotFound(other) != other {
		t.Fatal("NotFound debe devolver tal cual un error que no es sql.ErrNoRows")
	}
	if !errors.Is(NotFound(fmt.Errorf("leyendo: %w", sql.ErrNoRows)), repository.ErrNotFound) {
		t.Fatal("un sql.ErrNoRows envuelto también es ErrNotFound")
	}

	d := newTestDB(t)
	ctx := context.Background()
	var slug string
	err := NotFound(d.Conn(ctx).QueryRowContext(ctx, `SELECT slug FROM projects WHERE id = 42`).Scan(&slug))
	if !errors.Is(err, repository.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("NotFound(sin filas) = %v, esperaba repository.ErrNotFound", err)
	}
}

// La base guarda hashes de contraseñas y secretos cifrados: solo la lee su dueño.
func TestOpenFileMode(t *testing.T) {
	dir := t.TempDir()
	check := func(name, path string) {
		t.Helper()
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("%s: permisos %o, esperaba 600", name, perm)
		}
	}

	fresh := filepath.Join(dir, "nueva.db")
	d, err := Open(fresh)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	check("base nueva", fresh)
	if err := d.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	check("base nueva después de escribir", fresh)
	// En modo WAL, SQLite crea el -wal con los permisos de la base.
	check("WAL", fresh+"-wal")

	existing := filepath.Join(dir, "existente.db")
	if err := os.WriteFile(existing, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(existing, 0o644); err != nil {
		t.Fatal(err)
	}
	d2, err := Open(existing)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	check("base existente", existing)

	// Un -wal o -shm que ya existe con datos (ej. de un muelle que se cayó)
	// también queda en 0600. Se simula copiando los archivos con la base
	// abierta. Vacíos no sirven: SQLite ya corrige los permisos de un archivo
	// de largo 0 al abrirlo.
	live := filepath.Join(dir, "viva.db")
	d3, err := Open(live)
	if err != nil {
		t.Fatal(err)
	}
	defer d3.Close()
	if err := d3.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := insertProject(context.Background(), d3, "en-el-wal"); err != nil {
		t.Fatal(err)
	}
	crashed := filepath.Join(dir, "caida.db")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		data, err := os.ReadFile(live + suffix)
		if err != nil {
			t.Fatal(err)
		}
		if suffix != "" && len(data) == 0 {
			t.Fatalf("%s está vacío: la prueba no sirve", suffix)
		}
		if err := os.WriteFile(crashed+suffix, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(crashed+suffix, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	d4, err := Open(crashed)
	if err != nil {
		t.Fatal(err)
	}
	defer d4.Close()
	for _, suffix := range []string{"", "-wal", "-shm"} {
		check("caída"+suffix, crashed+suffix)
	}
	if n := countProjects(t, d4); n != 1 {
		t.Fatalf("la base recuperada tiene %d proyectos, esperaba el del WAL", n)
	}
}

// path va dentro de una URI file:, así que "?" o "#" cambiarían sus opciones
// (ej. "?_pragma=foreign_keys(0)"), SQLite decodifica los "%XX" (a%41.db
// abriría aA.db, que quedaría sin el chmod), y ":memory:" no es un archivo.
func TestOpenRejectsSpecialPaths(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{
		":memory:",
		filepath.Join(dir, "muelle.db?_pragma=foreign_keys(0)"),
		filepath.Join(dir, "muelle#x.db"),
		filepath.Join(dir, "a%41.db"),
		filepath.Join(dir, "100%.db"),
	} {
		d, err := Open(p)
		if err == nil {
			d.Close()
			t.Errorf("Open(%q) debió fallar", p)
			continue
		}
		if !strings.HasPrefix(err.Error(), "sqlite: ") || !strings.Contains(err.Error(), p) {
			t.Errorf("Open(%q): error %q, esperaba uno que nombre la ruta", p, err)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("Open no debe crear archivos para una ruta rechazada: %v", entries)
	}
	if _, err := os.Stat(":memory:"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Open creó el archivo :memory:: %v", err)
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
