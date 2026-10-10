package usecases

import (
	"context"
	"errors"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// AuthenticateSession convierte el token de la cookie en un Principal. Una
// sesión vencida (7 días) o sin uso por 12 h se borra y da
// ErrUnauthenticated. NeedsTOTPSetup se calcula en cada petición a partir de
// require_2fa.
type AuthenticateSession struct {
	users    repository.UserRepository
	sessions repository.SessionRepository
	settings service.SettingsReader
	clock    clock.Clock
}

func NewAuthenticateSession(users repository.UserRepository, sessions repository.SessionRepository,
	settings service.SettingsReader, c clock.Clock) *AuthenticateSession {
	return &AuthenticateSession{users: users, sessions: sessions, settings: settings, clock: c}
}

func (uc *AuthenticateSession) Execute(ctx context.Context, token string) (shared.Principal, error) {
	ss, err := uc.sessions.Get(ctx, service.HashToken(token))
	if errors.Is(err, sharedrepo.ErrNotFound) {
		return shared.Principal{}, shared.ErrUnauthenticated
	}
	if err != nil {
		return shared.Principal{}, err
	}
	now := uc.clock.Now()
	if ss.Expired(now, service.SessionIdle) {
		if err := uc.sessions.Delete(ctx, ss.IDHash); err != nil {
			return shared.Principal{}, err
		}
		return shared.Principal{}, shared.ErrUnauthenticated
	}
	u, err := uc.users.GetByID(ctx, ss.UserID)
	if errors.Is(err, sharedrepo.ErrNotFound) {
		return shared.Principal{}, shared.ErrUnauthenticated
	}
	if err != nil {
		return shared.Principal{}, err
	}
	p := shared.Principal{UserID: u.ID, Username: u.Username, SessionHash: ss.IDHash, MFAPending: ss.MFAPending}
	if ss.MFAPending {
		return p, nil
	}
	if now.Sub(ss.LastSeenAt) >= service.TouchEvery {
		if err := uc.sessions.Touch(ctx, ss.IDHash, now); err != nil {
			return shared.Principal{}, err
		}
	}
	required, err := uc.settings.Require2FA(ctx)
	if err != nil {
		return shared.Principal{}, err
	}
	p.NeedsTOTPSetup = required && !u.HasTOTP()
	return p, nil
}
