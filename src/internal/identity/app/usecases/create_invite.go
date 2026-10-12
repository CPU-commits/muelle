package usecases

import (
	"context"
	"errors"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// CreateInvite crea una invitación de un solo uso que vence en 24 h. Si quien
// la pide fue borrado mientras su petición estaba en curso, responde
// ErrUnauthenticated.
type CreateInvite struct {
	tx      sharedrepo.Transactor
	users   repository.UserRepository
	invites repository.InviteRepository
	audit   audit.Recorder
	clock   clock.Clock
}

func NewCreateInvite(tx sharedrepo.Transactor, users repository.UserRepository, invites repository.InviteRepository,
	rec audit.Recorder, c clock.Clock) *CreateInvite {
	return &CreateInvite{tx: tx, users: users, invites: invites, audit: rec, clock: c}
}

func (uc *CreateInvite) Execute(ctx context.Context, in dto.CreateInviteInput) (dto.InviteOutput, error) {
	var out dto.InviteOutput
	err := uc.tx.InTx(ctx, func(ctx context.Context) error {
		if _, err := uc.users.GetByID(ctx, in.CreatedBy); err != nil {
			if errors.Is(err, sharedrepo.ErrNotFound) {
				return shared.ErrUnauthenticated
			}
			return err
		}
		var err error
		if out, err = newInvite(ctx, uc.invites, uc.clock.Now(), &in.CreatedBy); err != nil {
			if errors.Is(err, sharedrepo.ErrNotFound) {
				return shared.ErrUnauthenticated
			}
			return err
		}
		uc.audit.Record(ctx, "invite.create", "user:"+in.CreatorUsername, nil)
		return nil
	})
	if err != nil {
		return dto.InviteOutput{}, err
	}
	return out, nil
}
