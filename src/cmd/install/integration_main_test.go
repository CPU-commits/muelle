//go:build integration

package install

import (
	"os"
	"testing"

	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest/swarmlock"
)

// TestMain toma el candado del Swarm (swarmlock) antes de correr los tests:
// los paquetes de integración que usan el Swarm corren de a uno, así uno no
// rompe las overlays del otro.
func TestMain(m *testing.M) {
	unlock := swarmlock.Lock()
	code := m.Run()
	unlock()
	os.Exit(code)
}
