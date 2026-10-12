package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/CPU-commits/muelle/src/internal/admin/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/service"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// BackupStore guarda las copias como archivos en dir, hechas con VACUUM INTO.
// Las copias tienen contraseñas hasheadas, secretos cifrados y datos TOTP:
// el directorio es 0700 y los archivos 0600.
//
// El directorio tiene que estar en un sistema de archivos con enlaces duros
// (Create publica con os.Link). Un volumen local de Docker (el valor por
// defecto) los soporta; algunos montajes de red o FAT no.
type BackupStore struct {
	db  *sqlite.DB
	dir string
	mu  sync.Mutex // un Create a la vez: así limpiar temporales viejos no borra uno en curso
}

var _ service.BackupStore = (*BackupStore)(nil)

func NewBackupStore(db *sqlite.DB, dir string) *BackupStore {
	return &BackupStore{db: db, dir: dir}
}

// Create genera una copia consistente de la base en uso. Escribe a un archivo
// temporal único, lo sincroniza a disco y lo publica con un enlace duro, que
// falla si name ya existe: nunca pisa una copia ni deja una a medias, ni
// siquiera con dos Create concurrentes del mismo nombre.
func (s *BackupStore) Create(ctx context.Context, name string) error {
	if filepath.Base(name) != name {
		return fmt.Errorf("backup: nombre inválido %q", name)
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	// MkdirAll no cambia los permisos de un directorio que ya existía.
	if err := os.Chmod(s.dir, 0o700); err != nil {
		return err
	}
	final := filepath.Join(s.dir, name)

	s.mu.Lock()
	defer s.mu.Unlock()
	// Un crash durante VACUUM INTO deja un temporal con una copia completa
	// (secretos incluidos) que List no ve: se borra antes de seguir.
	stale, err := filepath.Glob(filepath.Join(s.dir, "muelle-*.db.*.tmp"))
	if err != nil {
		return err
	}
	for _, p := range stale {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	f, err := os.CreateTemp(s.dir, name+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	f.Close()
	defer os.Remove(tmp) // también tras publicar: el enlace duro sigue vivo
	// VACUUM INTO exige que el destino no exista.
	if err := os.Remove(tmp); err != nil {
		return err
	}
	// VACUUM INTO no puede correr dentro de una transacción: Conn(ctx) sin tx es la base.
	if _, err := s.db.Conn(ctx).ExecContext(ctx, `VACUUM INTO ?`, tmp); err != nil {
		return fmt.Errorf("backup: VACUUM INTO %s: %w", name, err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	if err := syncFile(tmp); err != nil {
		return err
	}
	if err := os.Link(tmp, final); err != nil {
		return fmt.Errorf("backup: publicando %s: %w", name, err)
	}
	return syncFile(s.dir)
}

func syncFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// List devuelve solo las copias diarias (muelle-AAAA-MM-DD.db): un archivo
// ajeno en el directorio no cuenta para la retención.
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
		if _, ok := entity.ParseBackupName(e.Name()); ok && !e.IsDir() {
			names = append(names, e.Name())
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
