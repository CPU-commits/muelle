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
//
// El bloqueo progresivo es por usuario e IP (service.LoginLockout) y se
// aplica igual a nombres que no existen: ni la respuesta ni los tiempos
// revelan qué usuarios hay.
type Login struct {
	tx      sharedrepo.Transactor
	users   repository.UserRepository
	hasher  service.PasswordHasher
	limiter *service.LoginLimiter
	lockout *service.LoginLockout
	clock   clock.Clock
	issuer  sessionIssuer
	// dummy es un hash para verificar cuando el usuario no existe, así el login
	// tarda lo mismo y no revela qué nombres existen.
	dummy func() (string, error)
}

func NewLogin(tx sharedrepo.Transactor, users repository.UserRepository, sessions repository.SessionRepository,
	hasher service.PasswordHasher, limiter *service.LoginLimiter, lockout *service.LoginLockout, c clock.Clock) *Login {
	return &Login{
		tx: tx, users: users, hasher: hasher, limiter: limiter, lockout: lockout, clock: c,
		issuer: sessionIssuer{sessions: sessions, clock: c},
		dummy:  sync.OnceValues(func() (string, error) { return hasher.Hash("muelle-usuario-inexistente") }),
	}
}

func (uc *Login) Execute(ctx context.Context, in dto.LoginInput) (dto.SessionOutput, error) {
	if !uc.limiter.Allow(in.IP) {
		return dto.SessionOutput{}, service.ErrRateLimited
	}
	// Un par bloqueado no gasta el hasher, exista el usuario o no.
	if uc.lockout.Locked(in.Username, in.IP) {
		return dto.SessionOutput{}, service.ErrLocked
	}
	u, err := uc.users.GetByUsername(ctx, in.Username)
	if errors.Is(err, sharedrepo.ErrNotFound) {
		if h, err := uc.dummy(); err == nil {
			uc.hasher.Verify(h, in.Password)
		}
		return dto.SessionOutput{}, uc.fail(in)
	}
	if err != nil {
		return dto.SessionOutput{}, err
	}
	ok, err := uc.hasher.Verify(u.PasswordHash, in.Password)
	if err != nil {
		return dto.SessionOutput{}, err
	}
	if !ok {
		return dto.SessionOutput{}, uc.fail(in)
	}
	// La decisión final se toma bajo el mutex del bloqueo: un intento que
	// esperó al hasher mientras otros bloqueaban el par responde "bloqueado"
	// sin cambiar nada, igual que uno malo. Con 2FA los fallos se reinician
	// recién cuando el código es correcto (CompleteTOTP), así los códigos
	// malos se acumulan entre logins.
	if !uc.lockout.Succeed(in.Username, in.IP, !u.HasTOTP()) {
		return dto.SessionOutput{}, service.ErrLocked
	}
	return uc.issuer.issue(ctx, u, u.HasTOTP(), in.IP, in.UserAgent)
}

// fail registra el fallo y devuelve el error que corresponde.
func (uc *Login) fail(in dto.LoginInput) error {
	if uc.lockout.Fail(in.Username, in.IP) {
		return service.ErrLocked
	}
	return service.ErrInvalidCredentials
}
