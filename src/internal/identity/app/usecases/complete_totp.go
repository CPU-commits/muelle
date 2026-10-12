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
// por una sesión completa con otro token. Cada intento cuenta en el límite
// por IP que comparte con el login (limiter), pero a este paso solo llega
// quien sabe la contraseña, así que los bloqueos por códigos malos no son por
// IP sino:
//   - por sesión pendiente (service.PendingCodeAttempts): se validan a lo más
//     MaxPendingCodeAttempts códigos, cada uno con su intento reservado antes
//     de validarlo; si el último es malo la sesión se borra y hay que volver a
//     poner la contraseña;
//   - por usuario, desde cualquier IP (service.TOTPLockout), con el
//     calendario progresivo; solo un código correcto lo reinicia.
type CompleteTOTP struct {
	tx       sharedrepo.Transactor
	users    repository.UserRepository
	sessions repository.SessionRepository
	cipher   cipher.Cipher
	totp     service.TOTP
	limiter  *service.LoginLimiter
	lockout  *service.TOTPLockout
	attempts *service.PendingCodeAttempts
	replay   *service.TOTPReplayGuard
	clock    clock.Clock
	issuer   sessionIssuer
	audit    *LoginAudit
}

func NewCompleteTOTP(tx sharedrepo.Transactor, users repository.UserRepository, sessions repository.SessionRepository,
	c cipher.Cipher, totp service.TOTP, limiter *service.LoginLimiter, lockout *service.TOTPLockout,
	attempts *service.PendingCodeAttempts, replay *service.TOTPReplayGuard, la *LoginAudit, clk clock.Clock) *CompleteTOTP {
	return &CompleteTOTP{
		tx: tx, users: users, sessions: sessions, cipher: c, totp: totp,
		limiter: limiter, lockout: lockout, attempts: attempts, replay: replay, clock: clk,
		issuer: sessionIssuer{sessions: sessions, clock: clk},
		audit:  la,
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
	if uc.lockout.Locked(u.ID) {
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
	// El intento se reserva antes de validar el código: los pedidos en curso
	// a la vez sobre la misma sesión no pasan del límite, y una sesión agotada
	// no valida ni el código correcto, aunque todavía no se haya borrado.
	last, reserved := uc.attempts.Reserve(ss.IDHash, ss.ExpiresAt)
	if !reserved {
		return dto.SessionOutput{}, shared.ErrUnauthenticated
	}
	step, ok := uc.totp.Validate(in.Code, string(secret), now)
	if !ok {
		// Con el usuario ya bloqueado no cambia nada más (ni se borra la
		// sesión): la respuesta y el estado son los mismos que con un código
		// correcto.
		wasLocked, locked := uc.lockout.Fail(u.ID)
		if wasLocked {
			return dto.SessionOutput{}, service.ErrLocked
		}
		uc.audit.failed(ctx, u.Username, in.IP, "code")
		if locked {
			uc.audit.locked(ctx, u.Username, in.IP, "code")
		}
		return dto.SessionOutput{}, uc.invalidCode(ctx, ss.IDHash, last)
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
		// Bajo el mutex del bloqueo: si otro intento bloqueó al usuario mientras
		// tanto, "bloqueado" sin cambiar nada (y el rollback devuelve la sesión
		// pendiente). Si no, el código correcto reinicia sus fallos.
		if !uc.lockout.Succeed(u.ID) {
			return service.ErrLocked
		}
		if out, err = uc.issuer.issue(ctx, cur, false, in.IP, in.UserAgent); err != nil {
			return err
		}
		uc.audit.login(ctx, cur, in.IP, true)
		return nil
	})
	// Un código ya usado se decide dentro de la transacción; se audita afuera
	// porque con el rollback un evento registrado adentro se perdería.
	if errors.Is(err, service.ErrInvalidCode) {
		uc.audit.failed(ctx, u.Username, in.IP, "code")
		return dto.SessionOutput{}, uc.invalidCode(ctx, ss.IDHash, last)
	}
	if err != nil {
		return dto.SessionOutput{}, err
	}
	uc.attempts.Forget(ss.IDHash)
	return out, nil
}

// invalidCode responde a un código malo o ya usado. Si era el último intento
// de la sesión, la borra: hay que volver a poner la contraseña. Si el borrado
// falla, la sesión igual queda agotada en memoria hasta que vence.
func (uc *CompleteTOTP) invalidCode(ctx context.Context, sessionHash string, last bool) error {
	if last {
		if err := uc.sessions.Delete(ctx, sessionHash); err != nil {
			return err
		}
	}
	return service.ErrInvalidCode
}
