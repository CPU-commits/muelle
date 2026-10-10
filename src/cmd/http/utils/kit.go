// Package utils es el kit HTTP que cmd/http le pasa a cada módulo: registrar
// rutas con su nivel de acceso, responder JSON y errores, y leer cuerpos JSON.
package utils

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
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
//
// El alcance de un token se verifica con los comodines {p} y {s} de la ruta,
// así que una ruta AccessToken tiene que nombrarlos exactamente así: hace
// panic al registrarla si no tiene {p}, si tiene /stages/ sin {s}, si tiene
// /apps/ sin {a} o si termina en "/" (abarcaría cualquier ruta debajo).
func (k *Kit) Handle(pattern string, a Access, h http.HandlerFunc) {
	if a == AccessToken {
		checkTokenPattern(pattern)
	}
	k.mux.Handle(pattern, k.authenticate(a, h))
}

// checkTokenPattern hace panic si pattern no sirve para verificar el alcance
// de un token. Es un error de programación, como un patrón inválido del mux.
func checkTokenPattern(pattern string) {
	path := pattern
	if i := strings.IndexByte(pattern, '/'); i >= 0 {
		path = pattern[i:] // sin el método ni el host
	}
	var problem string
	switch {
	case !strings.Contains(path, "{p}"):
		problem = "no tiene el comodín {p}"
	case strings.Contains(path, "/stages/") && !strings.Contains(path, "{s}"):
		problem = "tiene /stages/ sin el comodín {s}"
	case strings.Contains(path, "/apps/") && !strings.Contains(path, "{a}"):
		problem = "tiene /apps/ sin el comodín {a}"
	case strings.HasSuffix(path, "/"):
		problem = `termina en "/" y abarcaría cualquier ruta debajo`
	default:
		return
	}
	panic(fmt.Sprintf("utils: la ruta %q usa AccessToken pero %s; sin eso no se puede verificar el alcance del token", pattern, problem))
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
				k.writeError(w, http.StatusForbidden, "token_not_allowed", "los tokens de API no pueden usar este endpoint")
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
				k.writeError(w, http.StatusForbidden, "out_of_scope", "el token no tiene acceso a este recurso")
				return
			}
			next(w, r.WithContext(entity.WithPrincipal(ctx, p)))
			return
		}

		// Dos cookies de sesión indican que un subdominio plantó la suya
		// (cookie tossing): no se adivina cuál es la buena.
		if len(r.CookiesNamed(SessionCookie)) > 1 {
			k.writeError(w, http.StatusBadRequest, "ambiguous_session", "la petición trae más de una cookie de sesión")
			return
		}
		c, err := r.Cookie(SessionCookie)
		if err != nil || c.Value == "" {
			k.writeError(w, http.StatusUnauthorized, "unauthenticated", "inicia sesión")
			return
		}
		p, err := k.auth.AuthenticateSession(ctx, c.Value)
		if err != nil {
			k.Fail(w, err)
			return
		}
		switch {
		case p.MFAPending && a != AccessMFAPending:
			k.writeError(w, http.StatusUnauthorized, "mfa_required", "falta el código de 2FA")
			return
		case !p.MFAPending && a == AccessMFAPending:
			k.writeError(w, http.StatusBadRequest, "no_mfa_pending", "esta sesión no espera un código")
			return
		case p.NeedsTOTPSetup && a != AccessSetup:
			k.writeError(w, http.StatusForbidden, "totp_setup_required", "configura 2FA para continuar")
			return
		}
		next(w, r.WithContext(entity.WithPrincipal(ctx, p)))
	})
}
