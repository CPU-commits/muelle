package usecases

import (
	"bytes"
	"context"
	"errors"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/package/cipher"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// CompleteTOTP valida el código de una sesión pendiente de MFA y la cambia
// por una sesión completa con otro token. Los códigos malos cuentan en el
// bloqueo de login del par (usuario, IP), y solo un código correcto lo reinicia.
type CompleteTOTP struct {
	tx       sharedrepo.Transactor
	users    repository.UserRepository
	sessions repository.SessionRepository
	cipher   cipher.Cipher
	totp     service.TOTP
	limiter  *service.LoginLimiter
	lockout  *service.LoginLockout
	replay   *service.TOTPReplayGuard
	clock    clock.Clock
	issuer   sessionIssuer
}

func NewCompleteTOTP(tx sharedrepo.Transactor, users repository.UserRepository, sessions repository.SessionRepository,
	c cipher.Cipher, totp service.TOTP, limiter *service.LoginLimiter, lockout *service.LoginLockout,
	replay *service.TOTPReplayGuard, clk clock.Clock) *CompleteTOTP {
	return &CompleteTOTP{
		tx: tx, users: users, sessions: sessions, cipher: c, totp: totp,
		limiter: limiter, lockout: lockout, replay: replay, clock: clk,
		issuer: sessionIssuer{sessions: sessions, clock: clk},
	}
}

func (uc *CompleteTOTP) Execute(ctx context.Context, in dto.CompleteTOTPInput) (dto.SessionOutput, error) {
	if !uc.limiter.Allow(in.IP) {
		return dto.SessionOutput{}, service.ErrRateLimited
	}
	ss, err := uc.sessions.Get(ctx, in.SessionHash)
	if errors.Is(err, sharedrepo.ErrNotFound) {
		return dto.SessionOutput{}, shared.ErrUnauthenticated
	}
	if err != nil {
		return dto.SessionOutput{}, err
	}
	now := uc.clock.Now()
	if !ss.MFAPending || ss.Expired(now, service.SessionIdle) {
		return dto.SessionOutput{}, shared.ErrUnauthenticated
	}
	u, err := uc.users.GetByID(ctx, ss.UserID)
	if errors.Is(err, sharedrepo.ErrNotFound) {
		return dto.SessionOutput{}, shared.ErrUnauthenticated
	}
	if err != nil {
		return dto.SessionOutput{}, err
	}
	if uc.lockout.Locked(u.Username, in.IP) {
		return dto.SessionOutput{}, service.ErrLocked
	}
	if !u.HasTOTP() {
		// Le reiniciaron el 2FA mientras esperaba el código: que vuelva a entrar.
		if err := uc.sessions.Delete(ctx, ss.IDHash); err != nil {
			return dto.SessionOutput{}, err
		}
		return dto.SessionOutput{}, shared.ErrUnauthenticated
	}
	secret, err := uc.cipher.Decrypt(u.TOTPSecret, service.TOTPSecretAAD(u.ID))
	if err != nil {
		return dto.SessionOutput{}, err
	}
	step, ok := uc.totp.Validate(in.Code, string(secret), now)
	if !ok {
		if uc.lockout.Fail(u.Username, in.IP) {
			return dto.SessionOutput{}, service.ErrLocked
		}
		return dto.SessionOutput{}, service.ErrInvalidCode
	}

	// El canje se decide de nuevo dentro de la transacción (BEGIN IMMEDIATE),
	// con el estado actual: dos canjes a la vez de la misma sesión, un
	// reinicio del 2FA o un bloqueo mientras se validaba el código no abren sesión.
	var out dto.SessionOutput
	err = uc.tx.InTx(ctx, func(ctx context.Context) error {
		cur, err := uc.users.GetByID(ctx, u.ID)
		if errors.Is(err, sharedrepo.ErrNotFound) {
			return shared.ErrUnauthenticated
		}
		if err != nil {
			return err
		}
		if !bytes.Equal(cur.TOTPSecret, u.TOTPSecret) {
			return shared.ErrUnauthenticated // el código se validó contra un secreto que ya no está activo
		}
		if err := uc.sessions.ConsumePending(ctx, ss.IDHash, uc.clock.Now()); err != nil {
			if errors.Is(err, sharedrepo.ErrNotFound) {
				return shared.ErrUnauthenticated // ya se canjeó o venció
			}
			return err
		}
		// Cada código vale una sola vez (RFC 6238 §5.2): si otra sesión ya usó
		// este paso, se rechaza y el rollback devuelve la sesión pendiente.
		if !uc.replay.Use(u.ID, step) {
			return service.ErrInvalidCode
		}
		// Bajo el mutex del bloqueo: si otro intento bloqueó el par mientras
		// tanto, "bloqueado" sin cambiar nada (y el rollback devuelve la sesión
		// pendiente). Si no, el código correcto reinicia los fallos.
		if !uc.lockout.Succeed(u.Username, in.IP, true) {
			return service.ErrLocked
		}
		out, err = uc.issuer.issue(ctx, cur, false, in.IP, in.UserAgent)
		return err
	})
	if err != nil {
		return dto.SessionOutput{}, err
	}
	return out, nil
}
