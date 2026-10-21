//go:build integration

package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest/swarmlock"
)

// staleAfter es la edad desde la que algo it2 se considera de una corrida
// anterior que no limpió: más que el -timeout de 25 min de estos tests, así
// nunca se toca lo de una corrida que sigue viva.
const staleAfter = 30 * time.Minute

// TestMain barre lo que dejó una corrida anterior cortada por un -timeout
// (un panic que no corre los t.Cleanup) antes de correr los tests. Solo toca
// lo de esta línea (it2) con más de 30 min:
//   - los stacks cuyos servicios se llaman it2… y llevan muelle.project = it2…;
//   - las redes it2… con la etiqueta muelle.test=it2 (las crea itIngress);
//   - los volúmenes it2… cuya com.docker.stack.namespace empieza con it2.
//
// Si Docker no está, no hace nada (los tests se saltan solos). Antes toma el
// candado del Swarm (swarmlock): los paquetes de integración que usan el Swarm
// corren de a uno.
func TestMain(m *testing.M) {
	unlock := swarmlock.Lock()
	sweepStale()
	code := m.Run()
	unlock()
	os.Exit(code)
}

func sweepStale() {
	c, err := docker.New(docker.Options{})
	if err != nil {
		return
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	info, err := c.Info(ctx)
	if err != nil || info.SwarmState != docker.SwarmActive || !info.IsManager {
		return
	}
	old := time.Now().Add(-staleAfter)

	services, err := c.ListServices(ctx, nil)
	if err != nil {
		return
	}
	stacks := map[string]bool{}
	for _, s := range services {
		stack := s.Labels[naming.LabelStackNamespace]
		if strings.HasPrefix(s.Name, itPrefix) && strings.HasPrefix(s.Labels[naming.LabelProject], itPrefix) &&
			strings.HasPrefix(stack, itPrefix) && s.CreatedAt.Before(old) {
			stacks[stack] = true
		}
	}
	for stack := range stacks {
		fmt.Fprintf(os.Stderr, "it2: borrando el stack %s de una corrida anterior\n", stack)
		_, _ = c.RemoveStack(ctx, stack)
	}
	for stack := range stacks {
		deadline := time.Now().Add(90 * time.Second)
		for time.Now().Before(deadline) {
			out, err := exec.CommandContext(ctx, "docker", "ps", "-aq", "--filter", "label="+naming.LabelStackNamespace+"="+stack).Output()
			if err != nil || len(bytes.TrimSpace(out)) == 0 {
				break
			}
			time.Sleep(2 * time.Second)
		}
	}

	if nets, err := c.ListNetworks(ctx, map[string]string{itLabel: "it2"}); err == nil {
		for _, n := range nets {
			if !strings.HasPrefix(n.Name, itPrefix) {
				continue
			}
			created, ok := createdAt(ctx, "network", n.Name, "{{json .Created}}")
			if !ok || !created.Before(old) {
				continue
			}
			fmt.Fprintf(os.Stderr, "it2: borrando la red %s de una corrida anterior\n", n.Name)
			if err := c.RemoveNetwork(ctx, n.Name); err != nil {
				fmt.Fprintf(os.Stderr, "it2: no se pudo borrar la red %s: %v\n", n.Name, err)
			}
		}
	}

	if vols, err := c.ListVolumes(ctx, nil); err == nil {
		for _, v := range vols {
			if !strings.HasPrefix(v.Name, itPrefix) || !strings.HasPrefix(v.Labels[naming.LabelStackNamespace], itPrefix) {
				continue
			}
			created, ok := createdAt(ctx, "volume", v.Name, "{{.CreatedAt}}")
			if !ok || !created.Before(old) {
				continue
			}
			fmt.Fprintf(os.Stderr, "it2: borrando el volumen %s de una corrida anterior\n", v.Name)
			if err := c.RemoveVolume(ctx, v.Name, false); err != nil {
				fmt.Fprintf(os.Stderr, "it2: no se pudo borrar el volumen %s: %v\n", v.Name, err)
			}
		}
	}
}

// createdAt lee la hora de creación de una red o un volumen con el CLI:
// docker.Network y docker.Volume no la traen. Con {{json .Created}} la de una
// red sale en RFC 3339 (sin json sale como time.Time.String()); la de un
// volumen ya es RFC 3339. Una que no se puede leer no se toca (false).
func createdAt(ctx context.Context, kind, name, format string) (time.Time, bool) {
	out, err := exec.CommandContext(ctx, "docker", kind, "inspect", "--format", format, name).Output()
	if err != nil {
		return time.Time{}, false
	}
	s := strings.TrimSpace(string(out))
	if strings.HasPrefix(s, `"`) {
		if err := json.Unmarshal([]byte(s), &s); err != nil {
			return time.Time{}, false
		}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
