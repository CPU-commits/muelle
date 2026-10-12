package usecases

import (
	"context"
	"strconv"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/package/audit"
)

// DeleteToken revoca un token de API.
type DeleteToken struct {
	tokens repository.TokenRepository
	audit  audit.Recorder
}

func NewDeleteToken(tokens repository.TokenRepository, rec audit.Recorder) *DeleteToken {
	return &DeleteToken{tokens: tokens, audit: rec}
}

func (uc *DeleteToken) Execute(ctx context.Context, id int64) error {
	if err := uc.tokens.Delete(ctx, id); err != nil {
		return err
	}
	uc.audit.Record(ctx, "token.delete", "token:"+strconv.FormatInt(id, 10), nil)
	return nil
}
