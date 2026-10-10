package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
)

// ListTokens devuelve los tokens de API en orden de creación, sin el token en claro.
type ListTokens struct {
	tokens repository.TokenRepository
}

func NewListTokens(tokens repository.TokenRepository) *ListTokens { return &ListTokens{tokens: tokens} }

func (uc *ListTokens) Execute(ctx context.Context) ([]dto.TokenOutput, error) {
	tokens, err := uc.tokens.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.TokenOutput, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, toTokenOutput(t))
	}
	return out, nil
}
