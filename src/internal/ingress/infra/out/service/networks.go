// Package service tiene los adaptadores de salida de ingress: Docker (con
// shared/infra/out/docker) y lo que ingress lee de otros módulos por SQL
// (§4.1 regla 1): las apps y los stages de catalog, las credenciales de
// registries y los ajustes de la tabla settings, donde también escribe sus
// claves (contrato §3.6).
package service

import (
	"context"
	"errors"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

// DockerEngine es lo que el adaptador usa del cliente compartido.
type DockerEngine interface {
	docker.Networks
}

// DockerNetworks implementa service.Networks con el cliente de Docker.
type DockerNetworks struct {
	docker DockerEngine
}

var _ service.Networks = (*DockerNetworks)(nil)

func NewDockerNetworks(d DockerEngine) *DockerNetworks { return &DockerNetworks{docker: d} }

func (n *DockerNetworks) Inspect(ctx context.Context, name string) (service.Network, bool, error) {
	net, err := n.docker.InspectNetwork(ctx, name)
	if errors.Is(err, docker.ErrNotFound) {
		return service.Network{}, false, nil
	}
	if err != nil {
		return service.Network{}, false, err
	}
	// Recién creada, una red de Swarm puede venir sin driver: cuenta por el scope.
	overlay := net.Scope == "swarm" && (net.Driver == "overlay" || net.Driver == "")
	return service.Network{Name: net.Name, Overlay: overlay, Attachable: net.Attachable, Containers: net.Containers}, true, nil
}

func (n *DockerNetworks) CreateOverlay(ctx context.Context, name string, labels map[string]string) error {
	_, err := n.docker.CreateNetwork(ctx, docker.NetworkSpec{Name: name, Driver: "overlay", Attachable: true, Labels: labels})
	if errors.Is(err, docker.ErrConflict) {
		return nil
	}
	return err
}

func (n *DockerNetworks) Remove(ctx context.Context, name string) (bool, error) {
	err := n.docker.RemoveNetwork(ctx, name)
	switch {
	case err == nil, errors.Is(err, docker.ErrNotFound):
		return false, nil
	case errors.Is(err, docker.ErrConflict):
		return true, nil
	}
	return false, err
}

func (n *DockerNetworks) Connect(ctx context.Context, network, container string) (bool, error) {
	err := n.docker.ConnectNetwork(ctx, network, container, nil)
	switch {
	case err == nil, errors.Is(err, docker.ErrConflict):
		return true, nil
	case errors.Is(err, docker.ErrNotFound):
		// El contenedor no existe (la red sí: la acaba de ver o crear quien llama).
		return false, nil
	}
	return false, err
}

func (n *DockerNetworks) Disconnect(ctx context.Context, network, container string) error {
	err := n.docker.DisconnectNetwork(ctx, network, container)
	if errors.Is(err, docker.ErrNotFound) {
		return nil
	}
	return err
}
