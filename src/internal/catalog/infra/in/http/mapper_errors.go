package http

import (
	"net/http"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/catalog/domain/service"
)

// registerErrors le dice al Kit cómo responder los errores propios del catálogo.
// not_found, conflict y not_empty los registra cmd/http para todos los módulos.
func registerErrors(k *utils.Kit) {
	k.RegisterError(service.ErrKindMismatch, http.StatusUnprocessableEntity, "kind_mismatch",
		"el campo no corresponde al tipo de app")
}
