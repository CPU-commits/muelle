package http

import (
	"context"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/identity/app/usecases"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// Authenticator es el utils.Authenticator que usa cmd/http para todas las
// rutas autenticadas, de todos los módulos.
type Authenticator struct {
	Session *usecases.AuthenticateSession
	Token   *usecases.AuthenticateToken
}

var _ utils.Authenticator = Authenticator{}

func (a Authenticator) AuthenticateSession(ctx context.Context, token string) (shared.Principal, error) {
	return a.Session.Execute(ctx, token)
}

func (a Authenticator) AuthenticateToken(ctx context.Context, bearer string) (shared.Principal, error) {
	return a.Token.Execute(ctx, bearer)
}
