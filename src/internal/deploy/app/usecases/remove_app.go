// Package usecases tiene los casos de uso del módulo deploy.
package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
)

// RemoveApp borra lo que una app tiene en Swarm. La llama catalog al borrar la
// app, antes de borrar la fila.
type RemoveApp struct {
	stacks service.Stacks
}

func NewRemoveApp(s service.Stacks) *RemoveApp { return &RemoveApp{stacks: s} }

// Execute ejecuta docker stack rm del stack de la app. Los volúmenes
// (<stack>_<nombre>) quedan: sus datos no se borran en la v1.
//
// Fase 2: antes, detener el worker de la app (cancelar su deploy en curso y
// marcar failed los queued) y no aceptar deploys nuevos de esa app hasta que
// la fila desaparezca; un deploy cuya app ya no existe no aplica nada.
func (u *RemoveApp) Execute(ctx context.Context, in dto.AppRef) error {
	return u.stacks.Remove(ctx, naming.Stack(in.Project, in.Stage, in.App))
}
