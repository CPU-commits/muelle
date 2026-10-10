package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/clock"
)

// CreateInvite crea una invitación de un solo uso que vence en 24 h.
type CreateInvite struct {
	invites repository.InviteRepository
	audit   audit.Recorder
	clock   clock.Clock
}

func NewCreateInvite(invites repository.InviteRepository, rec audit.Recorder, c clock.Clock) *CreateInvite {
	return &CreateInvite{invites: invites, audit: rec, clock: c}
}

func (uc *CreateInvite) Execute(ctx context.Context, in dto.CreateInviteInput) (dto.InviteOutput, error) {
	out, err := newInvite(ctx, uc.invites, uc.clock.Now(), &in.CreatedBy)
	if err != nil {
		return dto.InviteOutput{}, err
	}
	uc.audit.Record(ctx, "invite.create", "user:"+in.CreatorUsername, nil)
	return out, nil
}
