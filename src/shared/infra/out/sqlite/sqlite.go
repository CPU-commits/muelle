// Package sqlite es la base compartida de todos los módulos: abre SQLite,
// aplica las migraciones embebidas, maneja las transacciones (implementa
// repository.Transactor) y guarda la auditoría.
//
// Los repositorios de los módulos usan Conn(ctx) para cada consulta: así
// participan en la transacción de InTx si el ctx trae una. Dentro de InTx
// nunca se usa otra conexión para escribir, porque la transacción tiene el
// lock de escritura y la otra conexión esperaría hasta el busy_timeout.
package sqlite

import (
	"cmp"
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"time"

	msqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/CPU-commits/muelle/src/shared/domain/repository"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Executor es lo común entre *sql.DB y *sql.Tx.
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type DB struct {
	db *sql.DB
}

var _ repository.Transactor = (*DB)(nil)

// txKey guarda en el ctx la transacción en curso de una base concreta.
type txKey struct{ d *DB }

// Open abre la base en path con WAL, claves foráneas, busy_timeout de 5 s y
// transacciones IMMEDIATE (los escritores esperan en vez de fallar con SQLITE_BUSY).
func Open(path string) (*DB, error) {
	dsn := "file:" + path +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: abriendo %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: abriendo %s: %w", path, err)
	}
	return &DB{db: db}, nil
}

func (d *DB) Close() error { return d.db.Close() }

// Migrate aplica en orden las migraciones embebidas que aún no se aplicaron.
// Cada archivo se llama NNNN_descripcion.sql y corre en su propia transacción.
func (d *DB) Migrate(ctx context.Context) error {
	if _, err := d.db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("sqlite: creando schema_migrations: %w", err)
	}
	var current int
	if err := d.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	type migration struct {
		version int
		name    string
	}
	var migs []migration
	for _, e := range entries {
		prefix, _, ok := strings.Cut(e.Name(), "_")
		v, err := strconv.Atoi(prefix)
		if !ok || err != nil {
			return fmt.Errorf("sqlite: la migración %s no empieza con NNNN_", e.Name())
		}
		migs = append(migs, migration{v, e.Name()})
	}
	slices.SortFunc(migs, func(a, b migration) int { return cmp.Compare(a.version, b.version) })

	for _, m := range migs {
		if m.version <= current {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + m.name)
		if err != nil {
			return err
		}
		tx, err := d.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("sqlite: migración %s: %w", m.name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, m.version, Millis(time.Now())); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// InTx ejecuta fn en una transacción. Las consultas hechas con Conn(ctx) del
// ctx que recibe fn participan en ella. Si fn devuelve error (o hace panic)
// se hace rollback; el error se devuelve tal cual y el panic sigue su curso.
// Si ctx ya trae una transacción de esta base, fn corre dentro de ella.
func (d *DB) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{d}).(*sql.Tx); ok {
		return fn(ctx)
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: iniciando la transacción: %w", err)
	}
	// Después de un Commit exitoso, Rollback no hace nada (sql.ErrTxDone).
	defer tx.Rollback()
	if err := fn(context.WithValue(ctx, txKey{d}, tx)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: confirmando la transacción: %w", err)
	}
	return nil
}

// Conn devuelve la transacción en curso del ctx si hay una; si no, la base.
func (d *DB) Conn(ctx context.Context) Executor {
	if tx, ok := ctx.Value(txKey{d}).(*sql.Tx); ok {
		return tx
	}
	return d.db
}

// Millis convierte t a milisegundos Unix, el formato de todas las marcas de tiempo.
func Millis(t time.Time) int64 { return t.UnixMilli() }

// Time convierte milisegundos Unix a time.Time en UTC.
func Time(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

// NullMillis convierte un tiempo opcional a una columna que admite NULL.
func NullMillis(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: Millis(*t), Valid: true}
}

// NullTime convierte una columna que admite NULL a un tiempo opcional.
func NullTime(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := Time(v.Int64)
	return &t
}

// IsUnique dice si err es una violación de UNIQUE o de PRIMARY KEY.
func IsUnique(err error) bool {
	code := errorCode(err)
	return code == sqlite3.SQLITE_CONSTRAINT_UNIQUE || code == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY
}

// IsForeignKey dice si err es una violación de una clave foránea.
func IsForeignKey(err error) bool {
	return errorCode(err) == sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY
}

// errorCode devuelve el código extendido de SQLite de err, o 0 si no es de SQLite.
func errorCode(err error) int {
	var se *msqlite.Error
	if errors.As(err, &se) {
		return se.Code()
	}
	return 0
}
