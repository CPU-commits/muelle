// Package swarmlock serializa los tests de integración que usan el Swarm real
// (build tag integration): go test corre los binarios de los paquetes a la
// vez, y con varios paquetes creando y borrando overlays al mismo tiempo, el
// allocator de redes de Swarm se confunde ("could not find network allocator
// state for network …") y conectar un contenedor a una overlay attachable
// falla con "attaching to network failed … context deadline exceeded". Con
// el candado, `go test -tags integration ./...` corre esos paquetes de a uno,
// y los demás siguen en paralelo.
//
// Cada paquete que toca el Swarm lo toma en su TestMain, antes de tocar
// Docker:
//
//	func TestMain(m *testing.M) {
//		unlock := swarmlock.Lock()
//		code := m.Run()
//		unlock()
//		os.Exit(code)
//	}
//
// Es un flock(2) sobre un archivo del directorio temporal: el kernel lo suelta
// solo si el proceso muere (un -timeout que corta el test, por ejemplo).
// Esperar el candado cuenta para el -timeout del paquete.
package swarmlock

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// fileName es el archivo del candado en os.TempDir().
const fileName = "muelle-it-swarm.lock"

// Lock espera el candado del Swarm y lo toma; devuelve la función que lo
// suelta. Si no puede tomarlo (el archivo es de otro usuario, por ejemplo),
// lo dice en stderr y sigue sin candado: los tests corren igual, como antes.
func Lock() (unlock func()) {
	path := filepath.Join(os.TempDir(), fileName)
	start := time.Now()
	unlock, err := lockAt(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "swarmlock: sigo sin el candado del Swarm: %v\n", err)
		return func() {}
	}
	if d := time.Since(start); d >= time.Second {
		fmt.Fprintf(os.Stderr, "swarmlock: esperé %s a que otro paquete soltara el Swarm\n", d.Round(time.Second))
	}
	return unlock
}

// lockAt toma el flock exclusivo de path (lo crea con 0600 si no existe),
// esperando a quien lo tenga.
func lockAt(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("flock %s: %w", path, err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
