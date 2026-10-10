// Package usecases tiene los casos de uso de identity, un struct por caso de
// uso con sus dependencias y un método Execute.
package usecases

import (
	"context"
	"errors"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
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

// loginFailures lleva la cuenta de fallos seguidos de cada usuario y el
// bloqueo progresivo.
type loginFailures struct {
	tx    sharedrepo.Transactor
	users repository.UserRepository
	clock clock.Clock
}

// register suma un fallo y, si corresponde, bloquea al usuario. Devuelve si
// el usuario ya estaba bloqueado antes de este fallo: un intento que esperó
// al hasher pudo pasar el chequeo de bloqueo antes de que otro lo
// estableciera, y entonces hay que responder "bloqueado" igual que si la
// contraseña fuera correcta, para no revelarla.
//
// Se confirma en su propia transacción y sin la cancelación del request (un
// cliente que corta la conexión no deshace el fallo). Quien llama devuelve
// el error de dominio después, fuera de la transacción: InTx deshace todo si
// fn devuelve un error. No se debe llamar dentro de otro InTx que pueda fallar.
func (f loginFailures) register(ctx context.Context, userID int64) (wasLocked bool, err error) {
	err = f.tx.InTx(context.WithoutCancel(ctx), func(ctx context.Context) error {
		u, err := f.users.GetByID(ctx, userID)
		if errors.Is(err, sharedrepo.ErrNotFound) {
			return nil // lo borraron mientras tanto: no hay a quién bloquear
		}
		if err != nil {
			return err
		}
		now := f.clock.Now()
		wasLocked = u.LockedAt(now)
		n, err := f.users.IncrementFailedLogins(ctx, userID)
		if err != nil {
			return err
		}
		if d := service.LockoutFor(n); d > 0 {
			return f.users.SetLockedUntil(ctx, userID, now.Add(d))
		}
		return nil
	})
	return wasLocked, err
}

// reset borra los fallos tras un login completo. No escribe si no hay nada que borrar.
func (f loginFailures) reset(ctx context.Context, u entity.User) error {
	if u.FailedLogins == 0 && u.LockedUntil == nil {
		return nil
	}
	return f.users.ResetLoginFailures(ctx, u.ID)
}
