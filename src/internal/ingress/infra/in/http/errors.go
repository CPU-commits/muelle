// Package http tiene los endpoints REST de ingress (contrato §3): el estado
// del proxy (GET /api/v1/proxy) y los dominios (…/apps/{a}/domains). Todos
// solo con sesión.
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
	k.RegisterError(service.ErrNothingToAdopt, http.StatusConflict, "nothing_to_adopt", "No hay un registro que adoptar")
	k.RegisterError(service.ErrHostnameTaken, http.StatusConflict, "conflict", "Ese hostname ya está en uso en muelle")
}
