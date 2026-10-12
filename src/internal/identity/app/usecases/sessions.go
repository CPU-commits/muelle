// Package usecases tiene los casos de uso de identity, un struct por caso de
// uso con sus dependencias y un método Execute.
package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
)

// sessionIssuer abre sesiones nuevas, siempre con un token nuevo (así una
// sesión pendiente de MFA no se reutiliza como completa: evita la fijación
// de sesión).
type sessionIssuer struct {
	sessions repository.SessionRepository
	clock    clock.Clock
}

func (s sessionIssuer) issue(ctx context.Context, u entity.User, mfaPending bool, ip, userAgent string) (dto.SessionOutput, error) {
	plain, hash := service.RandomToken("")
	ttl := service.SessionMax
	if mfaPending {
		ttl = service.MFAPendingTTL
	}
	ss := entity.NewSession(hash, u.ID, mfaPending, s.clock.Now(), ttl, ip, userAgent)
	if err := s.sessions.Create(ctx, ss); err != nil {
		return dto.SessionOutput{}, err
	}
	return dto.SessionOutput{Token: plain, UserID: u.ID, Username: u.Username, ExpiresAt: ss.ExpiresAt, MFARequired: mfaPending}, nil
}
