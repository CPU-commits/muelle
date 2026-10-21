package http

import (
	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/ingress/app/usecases"
)

// UseCases son los casos de uso que exponen los endpoints de ingress.
type UseCases struct {
	ProxyStatus  *usecases.GetProxyStatus
	ListDomains  *usecases.ListDomains
	CreateDomain *usecases.CreateDomain
	UpdateDomain *usecases.UpdateDomain
	DeleteDomain *usecases.DeleteDomain
	AdoptDomain  *usecases.AdoptDomain
	RetryDomain  *usecases.RetryDomain
}

type handler struct {
	k  *utils.Kit
	uc UseCases
}

// appPath es {app} (contrato §0).
const appPath = "/api/v1/projects/{p}/stages/{s}/apps/{a}"

// Register registra los errores y las rutas de ingress, todas solo con
// sesión (contrato §3): GET /api/v1/proxy y los dominios de una app (§3.2).
func Register(k *utils.Kit, uc UseCases) {
	RegisterErrors(k)
	h := &handler{k: k, uc: uc}
	k.Handle("GET /api/v1/proxy", utils.AccessSession, h.proxy)
	k.Handle("GET "+appPath+"/domains", utils.AccessSession, h.listDomains)
	k.Handle("POST "+appPath+"/domains", utils.AccessSession, h.createDomain)
	k.Handle("PATCH "+appPath+"/domains/{hostname}", utils.AccessSession, h.updateDomain)
	k.Handle("DELETE "+appPath+"/domains/{hostname}", utils.AccessSession, h.deleteDomain)
	k.Handle("POST "+appPath+"/domains/{hostname}/adopt", utils.AccessSession, h.adoptDomain)
	k.Handle("POST "+appPath+"/domains/{hostname}/retry", utils.AccessSession, h.retryDomain)
}
