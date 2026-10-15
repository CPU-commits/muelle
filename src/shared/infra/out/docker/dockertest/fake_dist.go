package dockertest

import (
	"context"

	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

// Lo que agrega la Fase 5 (distribución) al fake. PutContainer es de la Fase
// 2: este archivo nunca lo declara (plan de la Fase 5, "Nombres nuevos en
// paquetes compartidos").

var _ docker.ServiceRestarter = (*Fake)(nil)

// ForceUpdateService sube Version del servicio, como un update del daemon.
// Las tareas nuevas las pone el test con SetTasks, si las necesita.
func (f *Fake) ForceUpdateService(ctx context.Context, nameOrID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ForceUpdateService", nameOrID); err != nil {
		return err
	}
	if err := f.swarm("ForceUpdateService"); err != nil {
		return err
	}
	s := f.findService(nameOrID)
	if s == nil {
		return notFound("servicio", nameOrID)
	}
	s.svc.Version++
	s.svc.UpdatedAt = f.Now()
	return nil
}

// PutNetwork deja una red tal cual, con su Driver, Scope, Attachable y
// Subnets, como una que ya existía en el daemon (install revisa que
// muelle_panel sea overlay, attachable y con subred privada). Sin ID se le
// asigna uno. No mira el Swarm ni registra una llamada.
func (f *Fake) PutNetwork(n docker.Network) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n.ID == "" {
		n.ID = f.id("net")
	}
	c := copyNetwork(&n)
	f.networks[n.Name] = &c
}
