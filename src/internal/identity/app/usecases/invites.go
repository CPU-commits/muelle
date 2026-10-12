package usecases

import (
	"context"
	"errors"
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// newInvite guarda una invitación nueva y devuelve su token en claro.
// createdBy nil es un link de setup del sistema.
func newInvite(ctx context.Context, invites repository.InviteRepository, now time.Time, createdBy *int64) (dto.InviteOutput, error) {
	plain, hash := service.RandomToken("")
	inv := entity.NewInvite(hash, createdBy, now, service.InviteTTL)
	if err := invites.Create(ctx, inv); err != nil {
		return dto.InviteOutput{}, err
	}
	return dto.InviteOutput{Token: plain, ExpiresAt: inv.ExpiresAt}, nil
}

// inviteAcceptor crea un usuario a partir de una invitación. Lo usan
// AcceptInvite (invitaciones de usuarios) y Setup (el link del sistema, system).
type inviteAcceptor struct {
	system  bool // acepta solo el link de setup del sistema; si no, solo invitaciones de usuarios
	tx      sharedrepo.Transactor
	users   repository.UserRepository
	invites repository.InviteRepository
	hasher  service.PasswordHasher
	audit   audit.Recorder
	limiter *service.LoginLimiter
	clock   clock.Clock
	issuer  sessionIssuer
}

// accept crea el usuario de la invitación. Como el endpoint es público, antes
// de calcular el hash (lento, y el hasher tiene solo 2 lugares) cuenta el
// intento contra el límite por IP, valida los datos y revisa que la
// invitación sirva. El hash se calcula fuera de la transacción, que tiene el
// lock de escritura. Después, en una sola transacción: corre check (si no es
// nil), vuelve a revisar la invitación, crea el usuario, marca la invitación
// como usada, abre la sesión y audita. Si algo falla no queda nada: con un
// nombre tomado (ErrConflict) la invitación sigue disponible.
func (a inviteAcceptor) accept(ctx context.Context, in dto.AcceptInviteInput, check func(ctx context.Context) error) (dto.SessionOutput, error) {
	if !a.limiter.Allow(in.IP) {
		return dto.SessionOutput{}, service.ErrRateLimited
	}
	if err := validate.Username(in.Username); err != nil {
		return dto.SessionOutput{}, err
	}
	if err := validate.Password(in.Password); err != nil {
		return dto.SessionOutput{}, err
	}
	tokenHash := service.HashToken(in.Token)
	if _, err := a.usableInvite(ctx, tokenHash); err != nil {
		return dto.SessionOutput{}, err
	}
	hash, err := a.hasher.Hash(in.Password)
	if err != nil {
		return dto.SessionOutput{}, err
	}

	var out dto.SessionOutput
	err = a.tx.InTx(ctx, func(ctx context.Context) error {
		if check != nil {
			if err := check(ctx); err != nil {
				return err
			}
		}
		inv, err := a.usableInvite(ctx, tokenHash)
		if err != nil {
			return err
		}
		now := a.clock.Now()
		u, err := entity.NewUser(in.Username, hash, inv.CreatedBy, now)
		if err != nil {
			return err
		}
		if u, err = a.users.Create(ctx, u); err != nil {
			return err
		}
		if err := a.invites.MarkUsed(ctx, inv.ID, now); err != nil {
			if errors.Is(err, sharedrepo.ErrNotFound) {
				return service.ErrInviteInvalid
			}
			return err
		}
		if out, err = a.issuer.issue(ctx, u, false, in.IP, in.UserAgent); err != nil {
			return err
		}
		// El autor de user.create es el usuario recién creado.
		actx := shared.WithPrincipal(ctx, shared.Principal{UserID: u.ID, Username: u.Username})
		a.audit.Record(actx, "user.create", "user:"+u.Username, nil)
		return nil
	})
	if err != nil {
		return dto.SessionOutput{}, err
	}
	return out, nil
}

// usableInvite devuelve la invitación si existe, es del tipo que acepta este
// caso de uso y todavía se puede aceptar; si no, ErrInviteInvalid.
func (a inviteAcceptor) usableInvite(ctx context.Context, tokenHash string) (entity.Invite, error) {
	inv, err := a.invites.GetByTokenHash(ctx, tokenHash)
	if errors.Is(err, sharedrepo.ErrNotFound) {
		return entity.Invite{}, service.ErrInviteInvalid
	}
	if err != nil {
		return entity.Invite{}, err
	}
	if inv.IsSystem() != a.system || !inv.Usable(a.clock.Now()) {
		return entity.Invite{}, service.ErrInviteInvalid
	}
	return inv, nil
}
