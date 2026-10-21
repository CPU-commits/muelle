// Package usecases tiene los casos de uso del módulo ingress.
package usecases

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/CPU-commits/muelle/src/internal/ingress/app/dto"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
)

// EnsureStageNetwork asegura la red overlay attachable de un stage y que
// muelle-caddy (el nombre de Options) esté conectado a ella (§3.2). La llaman
// catalog al crear el stage y deploy antes de cada deploy (§7 paso 4).
type EnsureStageNetwork struct {
	networks service.Networks
	caddy    string
	log      *slog.Logger
}

// NewEnsureStageNetwork recibe el nombre de muelle-caddy (el de Options:
// naming.CaddyContainer en producción, otro en los tests de integración).
func NewEnsureStageNetwork(n service.Networks, caddyContainer string, log *slog.Logger) *EnsureStageNetwork {
	return &EnsureStageNetwork{networks: n, caddy: caddyContainer, log: log}
}

// Execute crea la red si no existe y conecta muelle-caddy (el nombre de
// Options) en caliente si el contenedor existe (si no, ingress lo conecta a
// todas las redes de stage al crearlo). Una red con ese nombre que no es
// overlay attachable devuelve service.ErrNotStageNetwork.
func (u *EnsureStageNetwork) Execute(ctx context.Context, in dto.StageRef) error {
	name := naming.Network(in.Project, in.Stage)
	n, found, err := u.networks.Inspect(ctx, name)
	if err != nil {
		return err
	}
	switch {
	case !found:
		if err := u.networks.CreateOverlay(ctx, name, naming.StageLabels(in.Project, in.Stage)); err != nil {
			return err
		}
	case !n.Overlay || !n.Attachable:
		return fmt.Errorf("%w (%s)", service.ErrNotStageNetwork, name)
	}
	if _, err := u.networks.Connect(ctx, name, u.caddy); err != nil {
		return err
	}
	return nil
}

// RemoveStageNetwork desconecta muelle-caddy (el nombre de Options) de la red
// de un stage y la borra (§3.2). La llama catalog al borrar un stage vacío.
type RemoveStageNetwork struct {
	networks service.Networks
	caddy    string
	log      *slog.Logger
}

// NewRemoveStageNetwork recibe el nombre de muelle-caddy, como NewEnsureStageNetwork.
func NewRemoveStageNetwork(n service.Networks, caddyContainer string, log *slog.Logger) *RemoveStageNetwork {
	return &RemoveStageNetwork{networks: n, caddy: caddyContainer, log: log}
}

// Execute: una red que no existe no es un error. Si algo sigue conectado (las
// tareas de un stack que Swarm todavía está bajando tras docker stack rm), la
// red queda y devuelve nil: la borra después la reconciliación de ingress
// (Fase 3), que borra las redes con etiquetas de stage cuyo stage ya no existe.
func (u *RemoveStageNetwork) Execute(ctx context.Context, in dto.StageRef) error {
	name := naming.Network(in.Project, in.Stage)
	if err := u.networks.Disconnect(ctx, name, u.caddy); err != nil {
		return err
	}
	inUse, err := u.networks.Remove(ctx, name)
	if err != nil {
		return err
	}
	if inUse {
		u.log.Warn("la red del stage sigue en uso; se borra en la próxima reconciliación", "red", name)
	}
	return nil
}
