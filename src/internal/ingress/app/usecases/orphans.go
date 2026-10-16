package usecases

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
)

// RemoveOrphanNetworks borra las redes de stage huérfanas (D13 del plan de
// la Fase 3, contrato §1.2): las que tienen las etiquetas muelle.project y
// muelle.stage, se llaman naming.Network de esas etiquetas y no tienen un
// stage así en la base. Recién la segunda vez seguida que las ve: catalog
// crea la red antes de guardar el stage, y así no se borra la de un stage que
// se está creando. Antes de borrarla desconecta muelle-caddy; una que sigue
// en uso queda para la próxima. Es parte del job "proxy".
type RemoveOrphanNetworks struct {
	networks service.Networks
	catalog  service.Catalog
	caddy    string
	prefix   string
	log      *slog.Logger

	mu sync.Mutex
	// seen son las huérfanas que vio la pasada anterior.
	seen map[string]bool
}

// NewRemoveOrphanNetworks recibe el nombre de muelle-caddy y el prefijo de
// Options.NetworkPrefix: con uno, solo mira las redes que empiezan así (los
// tests de integración); "" mira todas (producción).
func NewRemoveOrphanNetworks(n service.Networks, c service.Catalog, caddyContainer, prefix string, log *slog.Logger) *RemoveOrphanNetworks {
	return &RemoveOrphanNetworks{networks: n, catalog: c, caddy: caddyContainer, prefix: prefix, log: log, seen: map[string]bool{}}
}

// Execute hace una pasada. Un error de una red no corta las demás: se juntan
// y esa red queda para la próxima.
func (u *RemoveOrphanNetworks) Execute(ctx context.Context) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	nets, err := u.networks.List(ctx)
	if err != nil {
		return fmt.Errorf("ingress: listando las redes: %w", err)
	}
	stages, err := u.catalog.Stages(ctx)
	if err != nil {
		return fmt.Errorf("ingress: leyendo los stages: %w", err)
	}
	exists := make(map[string]bool, len(stages))
	for _, s := range stages {
		exists[naming.Network(s.Project, s.Stage)] = true
	}

	seen := map[string]bool{}
	var errs []error
	for _, n := range nets {
		project, stage := n.Labels[naming.LabelProject], n.Labels[naming.LabelStage]
		switch {
		case project == "" || stage == "" || n.Name != naming.Network(project, stage):
			continue // no es una red de stage de muelle
		case u.prefix != "" && !strings.HasPrefix(n.Name, u.prefix):
			continue
		case exists[n.Name]:
			continue
		}
		seen[n.Name] = true
		if !u.seen[n.Name] {
			u.log.Info("ingress: red de stage sin stage; se borra si sigue así en la próxima pasada", "red", n.Name)
			continue
		}
		if err := u.networks.Disconnect(ctx, n.Name, u.caddy); err != nil {
			errs = append(errs, fmt.Errorf("ingress: desconectando muelle-caddy de la red huérfana %s: %w", n.Name, err))
			continue
		}
		inUse, err := u.networks.Remove(ctx, n.Name)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("ingress: borrando la red huérfana %s: %w", n.Name, err))
		case inUse:
			u.log.Info("ingress: la red huérfana sigue en uso; queda para la próxima pasada", "red", n.Name)
		default:
			delete(seen, n.Name)
			u.log.Info("ingress: red de stage huérfana borrada", "red", n.Name)
		}
	}
	u.seen = seen
	return errors.Join(errs...)
}
