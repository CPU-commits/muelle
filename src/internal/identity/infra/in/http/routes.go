// Package http tiene los endpoints de identity: /auth, /users, /invites y /tokens.
package http

import (
	"context"
	"net/http"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/identity/app/usecases"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// Usecases son los casos de uso que exponen las rutas de identity.
type Usecases struct {
	Login          *usecases.Login
	CompleteTOTP   *usecases.CompleteTOTP
	Logout         *usecases.Logout
	Setup          *usecases.Setup
	AcceptInvite   *usecases.AcceptInvite
	Me             *usecases.Me
	BeginTOTPSetup *usecases.BeginTOTPSetup
	EnableTOTP     *usecases.EnableTOTP
	CreateInvite   *usecases.CreateInvite
	ListUsers      *usecases.ListUsers
	DeleteUser     *usecases.DeleteUser
	ResetUserTOTP  *usecases.ResetUserTOTP
	CreateToken    *usecases.CreateToken
	ListTokens     *usecases.ListTokens
	DeleteToken    *usecases.DeleteToken
}

type handler struct {
	k           *utils.Kit
	uc          Usecases
	panelOrigin func(ctx context.Context) string
}

// RegisterRoutes registra los errores y las rutas de identity en el Kit.
// panelOrigin da el origen del panel, para los links de las invitaciones; se
// llama en cada una, y con "" la invitación es un 500. Hace panic si es nil.
func RegisterRoutes(k *utils.Kit, uc Usecases, panelOrigin func(ctx context.Context) string) {
	if panelOrigin == nil {
		panic("identity: RegisterRoutes sin el origen del panel (Deps.PanelOrigin)")
	}
	registerErrors(k)
	h := &handler{k: k, uc: uc, panelOrigin: panelOrigin}

	// Sesión (§10 del spec).
	k.HandlePublic("POST /api/v1/auth/login", h.login)
	k.HandlePublic("POST /api/v1/auth/setup", h.setup)
	k.HandlePublic("POST /api/v1/auth/invite/{token}", h.acceptInvite)
	k.Handle("POST /api/v1/auth/totp", utils.AccessMFAPending, h.completeTOTP)
	k.Handle("POST /api/v1/auth/logout", utils.AccessSetup, h.logout)
	k.Handle("GET /api/v1/auth/me", utils.AccessSetup, h.me)
	k.Handle("POST /api/v1/auth/totp/setup", utils.AccessSetup, h.beginTOTPSetup)
	k.Handle("POST /api/v1/auth/totp/enable", utils.AccessSetup, h.enableTOTP)

	// Administración: solo con sesión completa.
	k.Handle("GET /api/v1/users", utils.AccessSession, h.listUsers)
	k.Handle("DELETE /api/v1/users/{username}", utils.AccessSession, h.deleteUser)
	k.Handle("DELETE /api/v1/users/{username}/totp", utils.AccessSession, h.resetUserTOTP)
	k.Handle("POST /api/v1/invites", utils.AccessSession, h.createInvite)
	k.Handle("GET /api/v1/tokens", utils.AccessSession, h.listTokens)
	k.Handle("POST /api/v1/tokens", utils.AccessSession, h.createToken)
	k.Handle("DELETE /api/v1/tokens/{id}", utils.AccessSession, h.deleteToken)
}

// principal es quien hace la petición; Kit.Handle siempre lo pone en el ctx.
func principal(r *http.Request) shared.Principal {
	p, _ := shared.PrincipalFrom(r.Context())
	return p
}
