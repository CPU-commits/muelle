// Package sqlitetest abre bases SQLite reales y migradas para los tests.
package sqlitetest

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// New abre una base nueva en t.TempDir(), corre las migraciones y la cierra al terminar el test.
func New(t testing.TB) *sqlite.DB {
	t.Helper()
	d, err := sqlite.Open(filepath.Join(t.TempDir(), "muelle.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return d
}
