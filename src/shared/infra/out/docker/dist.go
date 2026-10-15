package docker

import "context"

// ServiceRestarter reinicia las tareas de un servicio sin cambiar su spec,
// como docker service update --force (Fase 5: muelle settings
// reset-panel-ips, contrato §5.4). No es parte de Engine: lo pide solo
// quien lo usa.
type ServiceRestarter interface {
	// ForceUpdateService sube TaskTemplate.ForceUpdate y actualiza el
	// servicio con la versión que leyó: Swarm reemplaza sus tareas según su
	// update_config. Las credenciales del registry son las que ya tiene el
	// servicio. ErrNotFound si no existe.
	ForceUpdateService(ctx context.Context, nameOrID string) error
}
