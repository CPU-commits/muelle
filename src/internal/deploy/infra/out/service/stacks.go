// Package service tiene los adaptadores de salida de deploy.
package service

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

// DockerEngine es lo que los adaptadores usan del cliente compartido.
type DockerEngine interface {
	docker.Services
	docker.Stacks
}

// DockerStacks implementa service.Stacks.
type DockerStacks struct {
	docker DockerEngine
}

var _ service.Stacks = (*DockerStacks)(nil)

func NewDockerStacks(d DockerEngine) *DockerStacks { return &DockerStacks{docker: d} }

// Remove pregunta primero por los servicios del stack: con Docker caído o sin
// Swarm, el SDK devuelve docker.ErrUnavailable (503), mientras que el CLI solo
// diría "terminó con código 1". docker stack rm de un stack sin nada no es un
// error ("Nothing found in stack").
func (s *DockerStacks) Remove(ctx context.Context, stack string) error {
	if _, err := s.docker.ListServices(ctx, map[string]string{naming.LabelStackNamespace: stack}); err != nil {
		return err
	}
	_, err := s.docker.RemoveStack(ctx, stack)
	return err
}
