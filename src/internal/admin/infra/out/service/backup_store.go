package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/CPU-commits/muelle/src/internal/admin/domain/service"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// BackupStore guarda las copias como archivos en Dir, hechas con VACUUM INTO.
type BackupStore struct {
	db  *sqlite.DB
	dir string
}

var _ service.BackupStore = (*BackupStore)(nil)

func NewBackupStore(db *sqlite.DB, dir string) *BackupStore {
	return &BackupStore{db: db, dir: dir}
}

// Create genera una copia consistente de la base en uso. Escribe a un archivo
// temporal y lo renombra, así un fallo no deja una copia a medias con el nombre final.
func (s *BackupStore) Create(ctx context.Context, name string) error {
	if filepath.Base(name) != name {
		return fmt.Errorf("backup: nombre inválido %q", name)
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	final := filepath.Join(s.dir, name)
	if _, err := os.Stat(final); err == nil {
		return fmt.Errorf("backup: %s ya existe", name)
	}
	tmp := final + ".tmp"
	os.Remove(tmp) // un resto de un intento anterior haría fallar a VACUUM INTO
	// VACUUM INTO no puede correr dentro de una transacción: Conn(ctx) sin tx es la base.
	if _, err := s.db.Conn(ctx).ExecContext(ctx, `VACUUM INTO ?`, tmp); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("backup: VACUUM INTO %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func (s *BackupStore) List(ctx context.Context) ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && strings.HasPrefix(n, "muelle-") && strings.HasSuffix(n, ".db") {
			names = append(names, n)
		}
	}
	slices.Sort(names) // AAAA-MM-DD ordena cronológicamente
	return names, nil
}

func (s *BackupStore) Remove(ctx context.Context, name string) error {
	if filepath.Base(name) != name {
		return fmt.Errorf("backup: nombre inválido %q", name)
	}
	return os.Remove(filepath.Join(s.dir, name))
}
