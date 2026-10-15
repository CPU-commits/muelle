package http

import (
	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/ingress/app/usecases"
)

// UseCases son los casos de uso que exponen los endpoints de ingress.
type UseCases struct {
	ProxyStatus *usecases.GetProxyStatus
}

type handler struct {
	k  *utils.Kit
	uc UseCases
}

// Register registra los errores y las rutas de ingress, todas solo con
// sesión (contrato §3): GET /api/v1/proxy. Los dominios llegan después.
func Register(k *utils.Kit, uc UseCases) {
	RegisterErrors(k)
	h := &handler{k: k, uc: uc}
	k.Handle("GET /api/v1/proxy", utils.AccessSession, h.proxy)
}
