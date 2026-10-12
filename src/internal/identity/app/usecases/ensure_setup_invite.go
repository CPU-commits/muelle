package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/package/clock"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// EnsureSetupInvite crea un link de setup nuevo si todavía no hay usuarios.
// Cada arranque sin usuarios invalida el link anterior: solo se guarda el
// hash, así que un link viejo no se puede volver a mostrar.
type EnsureSetupInvite struct {
	tx      sharedrepo.Transactor
	users   repository.UserRepository
	invites repository.InviteRepository
	clock   clock.Clock
}

func NewEnsureSetupInvite(tx sharedrepo.Transactor, users repository.UserRepository, invites repository.InviteRepository, c clock.Clock) *EnsureSetupInvite {
	return &EnsureSetupInvite{tx: tx, users: users, invites: invites, clock: c}
}

// Execute devuelve el token en claro y si se creó uno.
func (uc *EnsureSetupInvite) Execute(ctx context.Context) (token string, created bool, err error) {
	err = uc.tx.InTx(ctx, func(ctx context.Context) error {
		n, err := uc.users.Count(ctx)
		if err != nil || n > 0 {
			return err
		}
		if err := uc.invites.DeleteUnusedSystem(ctx); err != nil {
			return err
		}
		inv, err := newInvite(ctx, uc.invites, uc.clock.Now(), nil)
		if err != nil {
			return err
		}
		token, created = inv.Token, true
		return nil
	})
	if err != nil {
		return "", false, err
	}
	return token, created, nil
}
