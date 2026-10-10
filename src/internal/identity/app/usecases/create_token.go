package usecases

import (
	"context"
	"strconv"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/clock"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// CreateToken crea un token de API con alcance de proyecto o de proyecto +
// stage. El token en claro se devuelve una sola vez; solo se guarda su hash.
type CreateToken struct {
	tx     sharedrepo.Transactor
	tokens repository.TokenRepository
	scopes service.ScopeResolver
	audit  audit.Recorder
	clock  clock.Clock
}

func NewCreateToken(tx sharedrepo.Transactor, tokens repository.TokenRepository, scopes service.ScopeResolver,
	rec audit.Recorder, c clock.Clock) *CreateToken {
	return &CreateToken{tx: tx, tokens: tokens, scopes: scopes, audit: rec, clock: c}
}

func (uc *CreateToken) Execute(ctx context.Context, in dto.CreateTokenInput) (dto.CreatedTokenOutput, error) {
	now := uc.clock.Now()
	plain, hash := service.RandomToken(service.APITokenPrefix)
	tok, err := entity.NewAPIToken(in.Name, hash, 0, nil, &in.CreatedBy, now, nil)
	if err != nil {
		return dto.CreatedTokenOutput{}, err
	}
	if in.ExpiresInDays < 0 || in.ExpiresInDays > service.MaxTokenDays {
		return dto.CreatedTokenOutput{}, validate.Errorf("expires_in_days", "debe estar entre 0 y %d", service.MaxTokenDays)
	}
	if in.ExpiresInDays > 0 { // 0: no vence
		t := now.AddDate(0, 0, in.ExpiresInDays)
		tok.ExpiresAt = &t
	}

	err = uc.tx.InTx(ctx, func(ctx context.Context) error {
		projectID, stageID, err := uc.scopes.Resolve(ctx, in.Project, in.Stage)
		if err != nil {
			return err
		}
		tok.ProjectID, tok.StageID = projectID, stageID
		if tok, err = uc.tokens.Create(ctx, tok); err != nil {
			return err
		}
		uc.audit.Record(ctx, "token.create", "token:"+strconv.FormatInt(tok.ID, 10),
			map[string]string{"project": tok.ProjectSlug, "stage": tok.StageSlug})
		return nil
	})
	if err != nil {
		return dto.CreatedTokenOutput{}, err
	}
	return dto.CreatedTokenOutput{TokenOutput: toTokenOutput(tok), Plain: plain}, nil
}

func toTokenOutput(t entity.APIToken) dto.TokenOutput {
	return dto.TokenOutput{
		ID: t.ID, Name: t.Name, Project: t.ProjectSlug, Stage: t.StageSlug,
		CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt, ExpiresAt: t.ExpiresAt,
	}
}
