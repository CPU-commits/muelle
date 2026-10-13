// Package http tiene los endpoints REST de ingress. En la Fase 3 llegan los
// dominios (…/apps/{a}/domains) y el estado del proxy (GET /api/v1/proxy); por
// ahora solo registra los errores del módulo.
package http

import (
	"net/http"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
)

// RegisterErrors registra los errores de ingress. Como el Kit es uno solo,
// también valen para catalog, que crea la red al crear un stage.
func RegisterErrors(k *utils.Kit) {
	k.RegisterError(service.ErrNotStageNetwork, http.StatusConflict, "network_conflict", service.ErrNotStageNetwork.Error())
}
