// Package identity maneja usuarios, sesiones, 2FA, invitaciones, tokens de
// API y los límites de login (§10.1 del spec). Produce el Principal de cada
// petición (Authenticator) que consultan los demás módulos.
package identity

import (
	"context"
	"log/slog"

	"github.com/CPU-commits/muelle/src/cmd/cron"
	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/identity/app/usecases"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	incron "github.com/CPU-commits/muelle/src/internal/identity/infra/in/cron"
	inhttp "github.com/CPU-commits/muelle/src/internal/identity/infra/in/http"
	sqliterepo "github.com/CPU-commits/muelle/src/internal/identity/infra/out/repository/sqlite"
	outservice "github.com/CPU-commits/muelle/src/internal/identity/infra/out/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/cipher"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

type Deps struct {
	DB     *sqlite.DB
	Cipher cipher.Cipher
	Clock  clock.Clock
	Audit  audit.Recorder
	Log    *slog.Logger
}

type Module struct {
	auth        inhttp.Authenticator
	routes      inhttp.Usecases
	cleanup     *usecases.CleanupSessions
	ensureSetup *usecases.EnsureSetupInvite
	log         *slog.Logger
}

// New arma el módulo. Clock y Log son opcionales (reloj del sistema y
// slog.Default); el resto es obligatorio.
func New(d Deps) *Module {
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	db, c := d.DB, d.Clock

	users := sqliterepo.NewUserRepository(db)
	sessions := sqliterepo.NewSessionRepository(db)
	invites := sqliterepo.NewInviteRepository(db)
	tokens := sqliterepo.NewTokenRepository(db)

	hasher := outservice.NewArgon2Hasher() // un solo hasher: su semáforo vale para todo el proceso
	totp := outservice.NewTOTP()
	scopes := outservice.NewScopeResolver(db)
	settings := outservice.NewSettingsReader(db)
	limiter := service.NewLoginLimiter(service.LoginMaxPerIP, service.LoginWindow, c) // compartido por login, código TOTP, setup e invitaciones
	lockout := service.NewLoginLockout(c)                                             // compartido por login y código TOTP

	return &Module{
		auth: inhttp.Authenticator{
			Session: usecases.NewAuthenticateSession(users, sessions, settings, c),
			Token:   usecases.NewAuthenticateToken(tokens, c),
		},
		routes: inhttp.Usecases{
			Login:          usecases.NewLogin(db, users, sessions, hasher, limiter, lockout, c),
			CompleteTOTP:   usecases.NewCompleteTOTP(db, users, sessions, d.Cipher, totp, limiter, lockout, c),
			Logout:         usecases.NewLogout(sessions),
			Setup:          usecases.NewSetup(db, users, invites, sessions, hasher, d.Audit, limiter, c),
			AcceptInvite:   usecases.NewAcceptInvite(db, users, invites, sessions, hasher, d.Audit, limiter, c),
			Me:             usecases.NewMe(users),
			BeginTOTPSetup: usecases.NewBeginTOTPSetup(users, d.Cipher, totp),
			EnableTOTP:     usecases.NewEnableTOTP(db, users, d.Cipher, totp, d.Audit, c),
			CreateInvite:   usecases.NewCreateInvite(invites, d.Audit, c),
			ListUsers:      usecases.NewListUsers(users),
			DeleteUser:     usecases.NewDeleteUser(db, users, d.Audit),
			ResetUserTOTP:  usecases.NewResetUserTOTP(users, d.Audit),
			CreateToken:    usecases.NewCreateToken(db, tokens, scopes, d.Audit, c),
			ListTokens:     usecases.NewListTokens(tokens),
			DeleteToken:    usecases.NewDeleteToken(tokens, d.Audit),
		},
		cleanup:     usecases.NewCleanupSessions(sessions, c),
		ensureSetup: usecases.NewEnsureSetupInvite(db, users, invites, c),
		log:         d.Log,
	}
}

// Authenticator convierte cookies y tokens Bearer en el Principal de cada
// petición. Es el que recibe cmd/http (Config.Auth) para todos los módulos.
func (m *Module) Authenticator() utils.Authenticator { return m.auth }

// RegisterRoutes registra los errores y las rutas /auth, /users, /invites y /tokens.
func (m *Module) RegisterRoutes(k *utils.Kit) { inhttp.RegisterRoutes(k, m.routes) }

// Jobs devuelve las tareas periódicas: "sesiones vencidas", cada hora.
func (m *Module) Jobs() []cron.Job {
	return []cron.Job{incron.CleanupSessions(m.cleanup, m.log)}
}

// EnsureSetupInvite crea el link de setup si todavía no hay usuarios, e
// invalida el de un arranque anterior (solo se guarda su hash, así que no se
// puede volver a mostrar). Devuelve el token en claro y si se creó uno; quien
// llama arma la URL (<PanelOrigin>/setup?token=<token>).
func (m *Module) EnsureSetupInvite(ctx context.Context) (token string, created bool, err error) {
	return m.ensureSetup.Execute(ctx)
}
