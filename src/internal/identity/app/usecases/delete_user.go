package usecases

import (
	"context"
	"errors"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// DeleteUser borra un usuario con sus sesiones e invitaciones. Sus tokens de
// API quedan, sin creador (created_by NULL): son del proyecto y el CI depende
// de ellos (§10.1 del spec); se revocan a mano si hace falta. Nadie puede
// borrarse a sí mismo, y quien lo pide tiene que seguir existiendo: los dos
// chequeos y el borrado van en la misma transacción, así dos usuarios que
// se borran mutuamente a la vez no dejan la base sin usuarios.
type DeleteUser struct {
	tx    sharedrepo.Transactor
	users repository.UserRepository
	audit audit.Recorder
}

func NewDeleteUser(tx sharedrepo.Transactor, users repository.UserRepository, rec audit.Recorder) *DeleteUser {
	return &DeleteUser{tx: tx, users: users, audit: rec}
}

func (uc *DeleteUser) Execute(ctx context.Context, in dto.DeleteUserInput) error {
	return uc.tx.InTx(ctx, func(ctx context.Context) error {
		actor, err := uc.users.GetByID(ctx, in.ActorID)
		if errors.Is(err, sharedrepo.ErrNotFound) {
			return shared.ErrUnauthenticated // lo borraron mientras su petición estaba en curso
		}
		if err != nil {
			return err
		}
		target, err := uc.users.GetByUsername(ctx, in.Username)
		if err != nil {
			return err
		}
		if target.ID == actor.ID {
			return service.ErrCannotDeleteSelf
		}
		if err := uc.users.Delete(ctx, target.ID); err != nil {
			return err
		}
		uc.audit.Record(ctx, "user.delete", "user:"+target.Username, nil)
		return nil
	})
}
