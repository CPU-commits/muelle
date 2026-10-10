package usecases

import (
	"context"
	"errors"
	"strings"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// AuthenticateToken convierte un token Bearer en un Principal que siempre
// trae su TokenScope con los slugs del proyecto y el stage (Kit.Handle
// responde 500 si falta, porque Principal.Allows lo trataría como una sesión).
type AuthenticateToken struct {
	tokens repository.TokenRepository
	clock  clock.Clock
}

func NewAuthenticateToken(tokens repository.TokenRepository, c clock.Clock) *AuthenticateToken {
	return &AuthenticateToken{tokens: tokens, clock: c}
}

func (uc *AuthenticateToken) Execute(ctx context.Context, bearer string) (shared.Principal, error) {
	if !strings.HasPrefix(bearer, service.APITokenPrefix) {
		return shared.Principal{}, shared.ErrUnauthenticated
	}
	t, err := uc.tokens.GetByHash(ctx, service.HashToken(bearer))
	if errors.Is(err, sharedrepo.ErrNotFound) {
		return shared.Principal{}, shared.ErrUnauthenticated
	}
	if err != nil {
		return shared.Principal{}, err
	}
	now := uc.clock.Now()
	if t.Expired(now) || !t.ScopeValid() {
		return shared.Principal{}, shared.ErrUnauthenticated
	}
	if t.LastUsedAt == nil || now.Sub(*t.LastUsedAt) >= service.TouchEvery {
		if err := uc.tokens.Touch(ctx, t.ID, now); err != nil {
			return shared.Principal{}, err
		}
	}
	scope := t.Scope()
	return shared.Principal{Token: &scope}, nil
}
