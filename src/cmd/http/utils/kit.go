// Package utils es el kit HTTP que cmd/http le pasa a cada módulo: registrar
// rutas con su nivel de acceso, responder JSON y errores, y leer cuerpos JSON.
package utils

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"

	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

const (
	SessionCookie = "muelle_session"
	MaxBodyBytes  = 1 << 20
)

// Access define quién puede llamar a un endpoint.
type Access int

const (
	AccessSession    Access = iota // solo sesión completa
	AccessToken                    // sesión completa, o token de API dentro de su alcance (Principal.Allows)
	AccessSetup                    // sesión completa, incluida la que todavía debe configurar 2FA
	AccessMFAPending               // solo la sesión que espera el código TOTP
)

// Authenticator lo implementa identity.
type Authenticator interface {
	AuthenticateSession(ctx context.Context, token string) (entity.Principal, error)
	AuthenticateToken(ctx context.Context, bearer string) (entity.Principal, error)
}

type registeredError struct {
	target  error
	status  int
	code    string
	message string
}

// Kit es lo que cmd/http le pasa a cada módulo para registrar sus rutas.
type Kit struct {
	mux           *http.ServeMux
	auth          Authenticator
	secureCookies bool
	log           *slog.Logger

	mu     sync.RWMutex
	errors []registeredError
}

func NewKit(mux *http.ServeMux, auth Authenticator, secureCookies bool, log *slog.Logger) *Kit {
	if log == nil {
		log = slog.Default()
	}
	return &Kit{mux: mux, auth: auth, secureCookies: secureCookies, log: log}
}

// Handle registra un endpoint autenticado con el nivel de acceso a.
// El Principal queda en el ctx (entity.PrincipalFrom).
func (k *Kit) Handle(pattern string, a Access, h http.HandlerFunc) {
	k.mux.Handle(pattern, k.authenticate(a, h))
}

// HandlePublic registra un endpoint sin autenticación (login, setup, aceptar invitación).
func (k *Kit) HandlePublic(pattern string, h http.HandlerFunc) {
	k.mux.HandleFunc(pattern, h)
}

// errTokenWithoutScope indica un bug del Authenticator: sin TokenScope,
// Principal.Allows trataría al token como una sesión, que puede todo.
var errTokenWithoutScope = errors.New("utils: AuthenticateToken devolvió un Principal sin TokenScope")

func (k *Kit) authenticate(a Access, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if bearer, ok := BearerToken(r); ok {
			if a != AccessToken {
				WriteError(w, http.StatusForbidden, "token_not_allowed", "los tokens de API no pueden usar este endpoint")
				return
			}
			p, err := k.auth.AuthenticateToken(ctx, bearer)
			if err != nil {
				k.Fail(w, err)
				return
			}
			if p.Token == nil {
				k.Fail(w, errTokenWithoutScope)
				return
			}
			write := r.Method != http.MethodGet && r.Method != http.MethodHead
			if !p.Allows(r.PathValue("p"), r.PathValue("s"), write) {
				WriteError(w, http.StatusForbidden, "out_of_scope", "el token no tiene acceso a este recurso")
				return
			}
			next(w, r.WithContext(entity.WithPrincipal(ctx, p)))
			return
		}

		c, err := r.Cookie(SessionCookie)
		if err != nil || c.Value == "" {
			WriteError(w, http.StatusUnauthorized, "unauthenticated", "inicia sesión")
			return
		}
		p, err := k.auth.AuthenticateSession(ctx, c.Value)
		if err != nil {
			k.Fail(w, err)
			return
		}
		switch {
		case p.MFAPending && a != AccessMFAPending:
			WriteError(w, http.StatusUnauthorized, "mfa_required", "falta el código de 2FA")
			return
		case !p.MFAPending && a == AccessMFAPending:
			WriteError(w, http.StatusBadRequest, "no_mfa_pending", "esta sesión no espera un código")
			return
		case p.NeedsTOTPSetup && a != AccessSetup:
			WriteError(w, http.StatusForbidden, "totp_setup_required", "configura 2FA para continuar")
			return
		}
		next(w, r.WithContext(entity.WithPrincipal(ctx, p)))
	})
}
