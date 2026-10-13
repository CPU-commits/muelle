package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/secrets/app/dto"
	"github.com/CPU-commits/muelle/src/internal/secrets/domain/entity"
	secretsrepo "github.com/CPU-commits/muelle/src/internal/secrets/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/secrets/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// ResolveEnv devuelve las variables descifradas que recibe una app: las de
// su stage y encima las de la app (la app gana, §5.2 del spec). Lo usa el
// deploy (Fase 2); sus valores nunca van a la API ni a la auditoría.
type ResolveEnv struct {
	tx     repository.Transactor
	owners service.OwnerResolver
	repo   secretsrepo.EnvRepository
	crypto *service.EnvCrypto
}

func NewResolveEnv(tx repository.Transactor, owners service.OwnerResolver, repo secretsrepo.EnvRepository, crypto *service.EnvCrypto) *ResolveEnv {
	return &ResolveEnv{tx: tx, owners: owners, repo: repo, crypto: crypto}
}

// Execute toma el stage de la propia app, así que nunca mezcla las variables
// de otro stage. Lee las dos listas en una transacción, para que un cambio
// entre una y otra no deje una mezcla que nunca existió.
func (u *ResolveEnv) Execute(ctx context.Context, in dto.ResolveEnv) (dto.ResolvedEnv, error) {
	var out dto.ResolvedEnv
	// InTx (BEGIN IMMEDIATE) aunque solo lee: es para leer una foto consistente.
	err := u.tx.InTx(ctx, func(ctx context.Context) error {
		app := entity.AppOwner(in.AppID)
		stage, err := u.owners.StageOfApp(ctx, in.AppID)
		if err != nil {
			return err
		}
		stageVars, err := u.open(ctx, stage)
		if err != nil {
			return err
		}
		appVars, err := u.open(ctx, app)
		if err != nil {
			return err
		}
		out = dto.ResolvedEnv{
			Vars:         service.Merge(stageVars, appVars),
			SecretValues: service.SecretValues(stageVars, appVars),
		}
		return nil
	})
	if err != nil {
		return dto.ResolvedEnv{}, err
	}
	return out, nil
}

func (u *ResolveEnv) open(ctx context.Context, o entity.Owner) ([]entity.EnvVar, error) {
	rows, err := u.repo.List(ctx, o)
	if err != nil {
		return nil, err
	}
	return u.crypto.Open(o, rows)
}
