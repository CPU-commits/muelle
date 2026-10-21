//go:build integration

package container

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

// e2eStaleAfter es la edad desde la que algo it9 se considera de una corrida
// anterior que no limpió: más que el -timeout de 25 min de la prueba, así
// nunca se toca lo de una corrida que sigue viva.
const e2eStaleAfter = 30 * time.Minute

// TestMain toma el candado del Swarm (swarmlock): los paquetes de
// integración que usan el Swarm corren de a uno. Después barre lo que dejó
// una corrida anterior cortada por un -timeout (un panic que no corre los
// t.Cleanup), y solo lo de esta línea (it9) con más de 30 min:
//   - los stacks cuyos servicios se llaman it9… y llevan muelle.project = it9…;
//   - los contenedores it9-… (muelle-caddy, su prueba de puertos y el puente
//     de la admin API de la prueba);
//   - las redes it9… y los volúmenes it9-… o it9…_….
//
// Si Docker no está, no hace nada (la prueba se salta sola).
func TestMain(m *testing.M) {
	unlock := swarmlock.Lock()
	e2eSweepStale()
	code := m.Run()
	unlock()
	os.Exit(code)
}

func e2eSweepStale() {
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
	old := time.Now().Add(-e2eStaleAfter)
	stale := func(kind, name, format string) bool {
		created, ok := e2eCreatedAt(ctx, kind, name, format)
		return ok && created.Before(old)
	}

	if services, err := c.ListServices(ctx, nil); err == nil {
		stacks := map[string]bool{}
		for _, s := range services {
			stack := s.Labels[naming.LabelStackNamespace]
			if strings.HasPrefix(s.Name, e2ePrefix) && strings.HasPrefix(s.Labels[naming.LabelProject], e2ePrefix) &&
				strings.HasPrefix(stack, e2ePrefix) && s.CreatedAt.Before(old) {
				stacks[stack] = true
			}
		}
		for stack := range stacks {
			fmt.Fprintf(os.Stderr, "it9: borrando el stack %s de una corrida anterior\n", stack)
			_, _ = c.RemoveStack(ctx, stack)
		}
	}

	if out, err := exec.CommandContext(ctx, "docker", "ps", "-a", "--format", "{{.Names}}").Output(); err == nil {
		for _, name := range strings.Fields(string(out)) {
			if !strings.HasPrefix(name, e2ePrefix+"-") || !stale("container", name, "{{json .Created}}") {
				continue
			}
			fmt.Fprintf(os.Stderr, "it9: borrando el contenedor %s de una corrida anterior\n", name)
			_ = c.RemoveContainer(ctx, name, true)
		}
	}

	if nets, err := c.ListNetworks(ctx, nil); err == nil {
		for _, n := range nets {
			if !strings.HasPrefix(n.Name, e2ePrefix) || !stale("network", n.Name, "{{json .Created}}") {
				continue
			}
			fmt.Fprintf(os.Stderr, "it9: borrando la red %s de una corrida anterior\n", n.Name)
			if err := c.RemoveNetwork(ctx, n.Name); err != nil {
				fmt.Fprintf(os.Stderr, "it9: no se pudo borrar la red %s: %v\n", n.Name, err)
			}
		}
	}

	if vols, err := c.ListVolumes(ctx, nil); err == nil {
		for _, v := range vols {
			if !strings.HasPrefix(v.Name, e2ePrefix) || !stale("volume", v.Name, "{{.CreatedAt}}") {
				continue
			}
			fmt.Fprintf(os.Stderr, "it9: borrando el volumen %s de una corrida anterior\n", v.Name)
			if err := c.RemoveVolume(ctx, v.Name, true); err != nil {
				fmt.Fprintf(os.Stderr, "it9: no se pudo borrar el volumen %s: %v\n", v.Name, err)
			}
		}
	}
}

// e2eCreatedAt lee la hora de creación de un contenedor, una red o un volumen
// con el CLI (docker.Container, docker.Network y docker.Volume no la traen).
// Con {{json .Created}} la de un contenedor o una red sale en RFC 3339; la de
// un volumen ya es RFC 3339. Una que no se puede leer no se toca (false).
func e2eCreatedAt(ctx context.Context, kind, name, format string) (time.Time, bool) {
	out, err := exec.CommandContext(ctx, "docker", kind, "inspect", "--format", format, name).Output()
	if err != nil {
		return time.Time{}, false
	}
	s := string(bytes.TrimSpace(out))
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
