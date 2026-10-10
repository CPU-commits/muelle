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

// Setup crea el primer usuario con el link que imprime `muelle serve`. Solo
// funciona mientras no haya usuarios; la cuenta se vuelve a revisar dentro
// de la transacción que crea el usuario, así dos setups a la vez no crean dos.
type Setup struct {
	users    repository.UserRepository
	acceptor inviteAcceptor
}

func NewSetup(tx sharedrepo.Transactor, users repository.UserRepository, invites repository.InviteRepository,
	sessions repository.SessionRepository, hasher service.PasswordHasher, rec audit.Recorder,
	limiter *service.LoginLimiter, c clock.Clock) *Setup {
	return &Setup{users: users, acceptor: inviteAcceptor{
		tx: tx, users: users, invites: invites, hasher: hasher, audit: rec, limiter: limiter, clock: c,
		issuer: sessionIssuer{sessions: sessions, clock: c},
	}}
}

func (uc *Setup) Execute(ctx context.Context, in dto.AcceptInviteInput) (dto.SessionOutput, error) {
	// Primero, para que con usuarios la respuesta sea ErrAlreadySetUp aunque el cuerpo no sea válido.
	if err := uc.noUsers(ctx); err != nil {
		return dto.SessionOutput{}, err
	}
	return uc.acceptor.accept(ctx, in, uc.noUsers)
}

func (uc *Setup) noUsers(ctx context.Context) error {
	n, err := uc.users.Count(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return service.ErrAlreadySetUp
	}
	return nil
}
