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
		users: users, hasher: hasher, limiter: limiter, clock: c,
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
	if u.LockedAt(uc.clock.Now()) {
		return dto.SessionOutput{}, service.ErrLocked
	}
	ok, err := uc.hasher.Verify(u.PasswordHash, in.Password)
	if err != nil {
		return dto.SessionOutput{}, err
	}
	if !ok {
		if err := uc.failures.register(ctx, u.ID); err != nil {
			return dto.SessionOutput{}, err
		}
		return dto.SessionOutput{}, service.ErrInvalidCredentials
	}
	// Con 2FA, los fallos se reinician recién cuando el código es correcto
	// (CompleteTOTP): así los códigos malos se acumulan entre logins.
	if !u.HasTOTP() {
		if err := uc.failures.reset(ctx, u); err != nil {
			return dto.SessionOutput{}, err
		}
	}
	return uc.issuer.issue(ctx, u, u.HasTOTP(), in.IP, in.UserAgent)
}
