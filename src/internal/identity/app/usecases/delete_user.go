package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/package/audit"
)

// DeleteUser borra un usuario con sus sesiones e invitaciones. Nadie puede
// borrarse a sí mismo, así siempre queda al menos un usuario.
type DeleteUser struct {
	users repository.UserRepository
	audit audit.Recorder
}

func NewDeleteUser(users repository.UserRepository, rec audit.Recorder) *DeleteUser {
	return &DeleteUser{users: users, audit: rec}
}

func (uc *DeleteUser) Execute(ctx context.Context, in dto.DeleteUserInput) error {
	if in.Username == in.ActorUsername {
		return service.ErrCannotDeleteSelf
	}
	u, err := uc.users.GetByUsername(ctx, in.Username)
	if err != nil {
		return err
	}
	if err := uc.users.Delete(ctx, u.ID); err != nil {
		return err
	}
	uc.audit.Record(ctx, "user.delete", "user:"+u.Username, nil)
	return nil
}
