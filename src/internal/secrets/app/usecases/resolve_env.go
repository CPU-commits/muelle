package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/secrets/app/dto"
	"github.com/CPU-commits/muelle/src/internal/secrets/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/secrets/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/secrets/domain/service"
)

// ResolveEnv devuelve las variables descifradas que recibe una app: las del
// stage y encima las de la app (la app gana, §5.2 del spec). Lo usa el deploy
// (Fase 2); sus valores nunca van a la API ni a la auditoría.
type ResolveEnv struct {
	repo   repository.EnvRepository
	crypto *service.EnvCrypto
}

func NewResolveEnv(repo repository.EnvRepository, crypto *service.EnvCrypto) *ResolveEnv {
	return &ResolveEnv{repo: repo, crypto: crypto}
}

func (u *ResolveEnv) Execute(ctx context.Context, in dto.ResolveEnv) (map[string]string, error) {
	stage, err := u.open(ctx, entity.StageOwner(in.StageID))
	if err != nil {
		return nil, err
	}
	app, err := u.open(ctx, entity.AppOwner(in.AppID))
	if err != nil {
		return nil, err
	}
	return service.Merge(stage, app), nil
}

func (u *ResolveEnv) open(ctx context.Context, o entity.Owner) ([]entity.EnvVar, error) {
	rows, err := u.repo.List(ctx, o)
	if err != nil {
		return nil, err
	}
	return u.crypto.Open(o, rows)
}
