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
