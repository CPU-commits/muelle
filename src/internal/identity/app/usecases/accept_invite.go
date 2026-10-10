package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/clock"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// AcceptInvite crea un usuario con una invitación de un solo uso y le abre
// una sesión. Marcar la invitación como usada y crear el usuario van en la
// misma transacción.
type AcceptInvite struct {
	acceptor inviteAcceptor
}

func NewAcceptInvite(tx sharedrepo.Transactor, users repository.UserRepository, invites repository.InviteRepository,
	sessions repository.SessionRepository, hasher service.PasswordHasher, rec audit.Recorder,
	limiter *service.LoginLimiter, c clock.Clock) *AcceptInvite {
	return &AcceptInvite{acceptor: inviteAcceptor{
		tx: tx, users: users, invites: invites, hasher: hasher, audit: rec, limiter: limiter, clock: c,
		issuer: sessionIssuer{sessions: sessions, clock: c},
	}}
}

func (uc *AcceptInvite) Execute(ctx context.Context, in dto.AcceptInviteInput) (dto.SessionOutput, error) {
	return uc.acceptor.accept(ctx, in, nil)
}
