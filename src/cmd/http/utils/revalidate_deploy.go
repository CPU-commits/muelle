package utils

import (
	"net/http"

	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// Revalidate repite las comprobaciones de Handle con la credencial de r (la
// cookie o el Bearer con que se abrió un stream), sin escribir la respuesta
// (contrato §0.3): el alcance del token sobre {p}/{s} de la ruta (como
// lectura), y que la sesión siga completa. nil si sigue valiendo; si no,
// entity.ErrUnauthenticated, entity.ErrForbidden o el error del
// Authenticator (una sesión que se cerró o venció, un token revocado).
//
// Usa el mismo Authenticator que Handle, que toca last_seen_at de la sesión y
// last_used_at del token (a lo más una vez por minuto). Así, un stream
// abierto mantiene viva una sesión que si no vencería por las 12 h sin uso;
// el tope de 7 días de la sesión sigue valiendo, y un stream dura a lo más
// 1 h (el panel reconecta). Es aceptado: el panel abierto también la
// mantiene viva.
func (k *Kit) Revalidate(r *http.Request, a Access) error {
	ctx := r.Context()
	if bearer, ok := BearerToken(r); ok {
		if a != AccessToken {
			return entity.ErrForbidden
		}
		p, err := k.auth.AuthenticateToken(ctx, bearer)
		if err != nil {
			return err
		}
		if p.Token == nil {
			return errTokenWithoutScope
		}
		if !p.Allows(r.PathValue("p"), r.PathValue("s"), false) {
			return entity.ErrForbidden
		}
		return nil
	}

	cookies := r.CookiesNamed(SessionCookie)
	if len(cookies) != 1 || cookies[0].Value == "" {
		return entity.ErrUnauthenticated
	}
	p, err := k.auth.AuthenticateSession(ctx, cookies[0].Value)
	if err != nil {
		return err
	}
	switch {
	case p.MFAPending != (a == AccessMFAPending):
		return entity.ErrUnauthenticated
	case p.NeedsTOTPSetup && a != AccessSetup:
		return entity.ErrForbidden
	}
	return nil
}
