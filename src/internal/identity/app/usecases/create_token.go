package usecases

import (
	"context"
	"errors"
	"strconv"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// CreateToken crea un token de API con alcance de proyecto, de proyecto +
// stage, o de todos los proyectos (solo de lectura), y de escritura o de
// solo lectura (contrato §8.1). El token en claro se devuelve una sola vez;
// solo se guarda su hash.
// Si quien lo pide fue borrado mientras su petición estaba en curso, responde
// ErrUnauthenticated (no ErrNotFound, que haría pensar que falta el proyecto).
type CreateToken struct {
	tx     sharedrepo.Transactor
	users  repository.UserRepository
	tokens repository.TokenRepository
	scopes service.ScopeResolver
	audit  audit.Recorder
	clock  clock.Clock
}

func NewCreateToken(tx sharedrepo.Transactor, users repository.UserRepository, tokens repository.TokenRepository,
	scopes service.ScopeResolver, rec audit.Recorder, c clock.Clock) *CreateToken {
	return &CreateToken{tx: tx, users: users, tokens: tokens, scopes: scopes, audit: rec, clock: c}
}

func (uc *CreateToken) Execute(ctx context.Context, in dto.CreateTokenInput) (dto.CreatedTokenOutput, error) {
	now := uc.clock.Now()
	plain, hash := service.RandomToken(service.APITokenPrefix)
	tok, err := entity.NewAPIToken(in.Name, hash, 0, nil, &in.CreatedBy, now, nil, in.ReadOnly, in.AllProjects)
	if err != nil {
		return dto.CreatedTokenOutput{}, err
	}
	if in.ExpiresInDays < 0 || in.ExpiresInDays > service.MaxTokenDays {
		return dto.CreatedTokenOutput{}, validate.Errorf("expires_in_days", "debe estar entre 0 y %d", service.MaxTokenDays)
	}
	if in.AllProjects {
		if in.Project != "" || in.Stage != "" {
			return dto.CreatedTokenOutput{}, validate.Errorf("project", "un token de todos los proyectos no lleva proyecto ni stage")
		}
		if !in.ReadOnly {
			return dto.CreatedTokenOutput{}, validate.Errorf("read_only", "un token de todos los proyectos es de solo lectura")
		}
	}
	if in.ExpiresInDays > 0 { // 0: no vence
		t := now.AddDate(0, 0, in.ExpiresInDays)
		tok.ExpiresAt = &t
	}

	err = uc.tx.InTx(ctx, func(ctx context.Context) error {
		// Dentro de la transacción el creador no puede desaparecer: después,
		// una clave foránea rota solo puede ser del proyecto o del stage.
		if _, err := uc.users.GetByID(ctx, in.CreatedBy); err != nil {
			if errors.Is(err, sharedrepo.ErrNotFound) {
				return shared.ErrUnauthenticated
			}
			return err
		}
		// Un token de todos los proyectos no busca ningún proyecto.
		if !in.AllProjects {
			projectID, stageID, err := uc.scopes.Resolve(ctx, in.Project, in.Stage)
			if err != nil {
				return err
			}
			tok.ProjectID, tok.StageID = projectID, stageID
		}
		if tok, err = uc.tokens.Create(ctx, tok); err != nil {
			return err
		}
		uc.audit.Record(ctx, "token.create", "token:"+strconv.FormatInt(tok.ID, 10), tokenAuditDetails(tok))
		return nil
	})
	if err != nil {
		return dto.CreatedTokenOutput{}, err
	}
	return dto.CreatedTokenOutput{TokenOutput: toTokenOutput(tok), Plain: plain}, nil
}

// tokenAuditDetails son los details de token.create (contrato §8.1): el
// alcance y si es de solo lectura. stage va en "" si es de todo el proyecto.
func tokenAuditDetails(t entity.APIToken) map[string]any {
	if t.AllProjects {
		return map[string]any{"all_projects": true, "read_only": t.ReadOnly}
	}
	return map[string]any{"project": t.ProjectSlug, "stage": t.StageSlug, "read_only": t.ReadOnly}
}

func toTokenOutput(t entity.APIToken) dto.TokenOutput {
	out := dto.TokenOutput{
		ID: t.ID, Name: t.Name, Project: t.ProjectSlug, Stage: t.StageSlug,
		ReadOnly: t.ReadOnly, AllProjects: t.AllProjects,
		CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt, ExpiresAt: t.ExpiresAt,
	}
	if t.CreatedByName != "" {
		out.CreatedBy = &t.CreatedByName
	}
	return out
}
