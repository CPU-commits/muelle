package http

import (
	"net/http"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
)

// RegisterErrors le dice al Kit cómo responder los errores propios de deploy.
// not_found, conflict y docker_unavailable los registra cmd/http para todos
// los módulos.
func RegisterErrors(k *utils.Kit) {
	k.RegisterError(service.ErrNoPreviousDeploy, http.StatusConflict, "no_previous_deploy",
		"no hay un deploy exitoso anterior al actual")
	k.RegisterError(service.ErrAppRemoving, http.StatusConflict, "conflict", "la app se está borrando")
	// Lo devuelve RemoveApp, así que lo responde el DELETE de una app
	// (catalog): el Kit es uno solo (contrato §1.1).
	k.RegisterError(service.ErrWorkerBusy, http.StatusConflict, "conflict",
		"un deploy de la app todavía no se detiene; vuelve a intentar en unos segundos")
}
