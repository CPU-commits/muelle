// Package http expone los casos de uso de deploy como endpoints REST
// (contrato de las fases 2 y 3, §2).
package http

import (
	"context"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/deploy/app/usecases"
)

// UseCases son los casos de uso que usan los handlers.
type UseCases struct {
	ListRegistries *usecases.ListRegistries
	CreateRegistry *usecases.CreateRegistry
	UpdateRegistry *usecases.UpdateRegistry
	DeleteRegistry *usecases.DeleteRegistry

	CreateDeploy *usecases.CreateDeploy
	Rollback     *usecases.Rollback
	ListDeploys  *usecases.ListDeploys
	GetDeploy    *usecases.GetDeploy
	ReadLog      *usecases.ReadLog
	FollowLog    *usecases.FollowLog
	GetStatus    *usecases.GetStatus
	AppLogs      *usecases.AppLogs
}

type Handler struct {
	uc    UseCases
	k     *utils.Kit
	limit *usecases.StreamSlots
	cfg   SSEConfig
	base  context.Context
}

// Register registra los errores del módulo y sus rutas en k (contrato §1:
// todas las de deploy). s son el ctx y los límites de los streams.
func Register(k *utils.Kit, uc UseCases, s Streams) {
	RegisterErrors(k)
	if s.Ctx == nil {
		s.Ctx = context.Background()
	}
	if s.Slots == nil {
		s.Slots = usecases.NewStreamSlots(s.Config.MaxPerPrincipal)
	}
	h := &Handler{uc: uc, k: k, limit: s.Slots, cfg: s.Config, base: s.Ctx}

	k.Handle("GET /api/v1/registries", utils.AccessSession, h.listRegistries)
	k.Handle("POST /api/v1/registries", utils.AccessSession, h.createRegistry)
	k.Handle("PATCH /api/v1/registries/{host}", utils.AccessSession, h.updateRegistry)
	k.Handle("DELETE /api/v1/registries/{host}", utils.AccessSession, h.deleteRegistry)

	const app = "/api/v1/projects/{p}/stages/{s}/apps/{a}"
	k.Handle("POST "+app+"/deploys", utils.AccessToken, h.createDeploy)
	k.Handle("GET "+app+"/deploys", utils.AccessToken, h.listDeploys)
	k.Handle("GET "+app+"/deploys/{id}", utils.AccessToken, h.getDeploy)
	k.Handle("GET "+app+"/deploys/{id}/log", utils.AccessToken, h.readLog)
	k.Handle("GET "+app+"/deploys/{id}/log/stream", utils.AccessToken, h.streamLog)
	k.Handle("POST "+app+"/rollback", utils.AccessToken, h.rollback)
	k.Handle("GET "+app+"/status", utils.AccessToken, h.status)
	k.Handle("GET "+app+"/logs", utils.AccessToken, h.appLogs)
}
