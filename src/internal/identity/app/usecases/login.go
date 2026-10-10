package usecases

import (
	"context"
	"errors"
	"sync"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// Login verifica usuario y contraseña. Sin 2FA abre una sesión completa; con
// 2FA abre una sesión pendiente de 5 minutos que CompleteTOTP cambia por una
// completa.
type Login struct {
	tx       sharedrepo.Transactor
	users    repository.UserRepository
	hasher   service.PasswordHasher
	limiter  *service.LoginLimiter
	clock    clock.Clock
	failures loginFailures
	issuer   sessionIssuer
	// dummy es un hash para verificar cuando el usuario no existe, así el login
	// tarda lo mismo y no revela qué nombres existen.
	dummy func() (string, error)
}

func NewLogin(tx sharedrepo.Transactor, users repository.UserRepository, sessions repository.SessionRepository,
	hasher service.PasswordHasher, limiter *service.LoginLimiter, c clock.Clock) *Login {
	return &Login{
		tx: tx, users: users, hasher: hasher, limiter: limiter, clock: c,
		failures: loginFailures{tx: tx, users: users, clock: c},
		issuer:   sessionIssuer{sessions: sessions, clock: c},
		dummy:    sync.OnceValues(func() (string, error) { return hasher.Hash("muelle-usuario-inexistente") }),
	}
}

func (uc *Login) Execute(ctx context.Context, in dto.LoginInput) (dto.SessionOutput, error) {
	if !uc.limiter.Allow(in.IP) {
		return dto.SessionOutput{}, service.ErrRateLimited
	}
	u, err := uc.users.GetByUsername(ctx, in.Username)
	if errors.Is(err, sharedrepo.ErrNotFound) {
		if h, err := uc.dummy(); err == nil {
			uc.hasher.Verify(h, in.Password)
		}
		return dto.SessionOutput{}, service.ErrInvalidCredentials
	}
	if err != nil {
		return dto.SessionOutput{}, err
	}
	// Atajo: un usuario bloqueado no gasta el hasher. Abajo se vuelve a
	// revisar, porque Verify puede esperar al semáforo mientras otros
	// intentos bloquean al usuario.
	if u.LockedAt(uc.clock.Now()) {
		return dto.SessionOutput{}, service.ErrLocked
	}
	ok, err := uc.hasher.Verify(u.PasswordHash, in.Password)
	if err != nil {
		return dto.SessionOutput{}, err
	}
	if !ok {
		wasLocked, err := uc.failures.register(ctx, u.ID)
		if err != nil {
			return dto.SessionOutput{}, err
		}
		if wasLocked {
			return dto.SessionOutput{}, service.ErrLocked
		}
		return dto.SessionOutput{}, service.ErrInvalidCredentials
	}

	// La decisión final se toma con el estado actual del usuario y en una
	// transacción (BEGIN IMMEDIATE), así no se cruza con el registro de un fallo.
	var out dto.SessionOutput
	err = uc.tx.InTx(ctx, func(ctx context.Context) error {
		cur, err := uc.users.GetByID(ctx, u.ID)
		if errors.Is(err, sharedrepo.ErrNotFound) {
			return service.ErrInvalidCredentials
		}
		if err != nil {
			return err
		}
		if cur.LockedAt(uc.clock.Now()) {
			return service.ErrLocked
		}
		// Con 2FA, los fallos se reinician recién cuando el código es correcto
		// (CompleteTOTP): así los códigos malos se acumulan entre logins.
		if !cur.HasTOTP() {
			if err := uc.failures.reset(ctx, cur); err != nil {
				return err
			}
		}
		out, err = uc.issuer.issue(ctx, cur, cur.HasTOTP(), in.IP, in.UserAgent)
		return err
	})
	if err != nil {
		return dto.SessionOutput{}, err
	}
	return out, nil
}
