package usecases

import (
	"context"
	"log/slog"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
)

// StoreManagerNode guarda manager_node_id al arrancar (contrato §1.2 paso 1):
// el id del nodo local, si falta. Es el storage_node_id por defecto de una app
// con volúmenes (§6.2).
type StoreManagerNode struct {
	cluster service.Cluster
	manager service.ManagerNode
	log     *slog.Logger
}

func NewStoreManagerNode(c service.Cluster, m service.ManagerNode, log *slog.Logger) *StoreManagerNode {
	return &StoreManagerNode{cluster: c, manager: m, log: log}
}

// Execute no falla: con Docker caído (o sin Swarm) registra una advertencia y
// sigue, y el próximo deploy lo reintenta (RunDeploy, paso 3).
func (u *StoreManagerNode) Execute(ctx context.Context) {
	id, err := u.manager.Get(ctx)
	if err != nil {
		u.log.Error("deploy: no se pudo leer manager_node_id", "error", err)
		return
	}
	if id != "" {
		return
	}
	local, err := u.cluster.LocalNodeID(ctx)
	if err != nil {
		if ctx.Err() == nil {
			u.log.Warn("deploy: no se pudo guardar manager_node_id; lo reintenta el próximo deploy", "error", dockerMessage(err))
		}
		return
	}
	if _, err := u.manager.SetIfMissing(ctx, local); err != nil {
		u.log.Error("deploy: no se pudo guardar manager_node_id", "error", err)
	}
}
