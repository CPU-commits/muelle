package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/secrets/app/dto"
	"github.com/CPU-commits/muelle/src/internal/secrets/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/secrets/domain/service"
)

// GetEnv devuelve las variables de un stage o una app, con los secretos enmascarados.
type GetEnv struct {
	owners service.OwnerResolver
	repo   repository.EnvRepository
	crypto *service.EnvCrypto
}

func NewGetEnv(owners service.OwnerResolver, repo repository.EnvRepository, crypto *service.EnvCrypto) *GetEnv {
	return &GetEnv{owners: owners, repo: repo, crypto: crypto}
}

func (u *GetEnv) Execute(ctx context.Context, in dto.EnvTarget) ([]service.MaskedVar, error) {
	owner, err := resolveOwner(ctx, u.owners, in)
	if err != nil {
		return nil, err
	}
	rows, err := u.repo.List(ctx, owner)
	if err != nil {
		return nil, err
	}
	return u.crypto.Mask(owner, rows)
}
