package swarmlock

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Mientras alguien tiene el candado, nadie más lo toma; al soltarlo, sí.
func TestLockExcludesOthers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "swarm.lock")
	unlock, err := lockAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if tryLock(t, path) {
		t.Fatal("otro tomó el candado mientras estaba tomado")
	}
	unlock()
	if !tryLock(t, path) {
		t.Fatal("el candado no se soltó")
	}
}

// El segundo espera al primero, y lo toma apenas se suelta.
func TestLockWaitsForHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "swarm.lock")
	unlock, err := lockAt(path)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan func(), 1)
	go func() {
		u, err := lockAt(path)
		if err != nil {
			t.Error(err)
			u = func() {}
		}
		got <- u
	}()
	select {
	case u := <-got:
		u()
		t.Fatal("el segundo no esperó al primero")
	case <-time.After(200 * time.Millisecond):
	}
	unlock()
	select {
	case u := <-got:
		u()
	case <-time.After(5 * time.Second):
		t.Fatal("el segundo no tomó el candado al soltarse")
	}
}

// Lock usa un archivo fijo en el directorio temporal: así lo comparten los
// binarios de test de todos los paquetes de un mismo go test.
func TestLockUsesSharedFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	unlock := Lock()
	path := filepath.Join(dir, fileName)
	if _, err := os.Stat(path); err != nil {
		unlock()
		t.Fatalf("el candado no está en %s: %v", path, err)
	}
	if tryLock(t, path) {
		unlock()
		t.Fatal("Lock no tomó el archivo compartido")
	}
	unlock()
}

// tryLock intenta tomar el candado de path sin esperar, y lo suelta si pudo.
func tryLock(t *testing.T, path string) bool {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	return true
}
