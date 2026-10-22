// Package utils es el kit HTTP que cmd/http le pasa a cada módulo: registrar
// rutas con su nivel de acceso, responder JSON y errores, y leer cuerpos JSON.
package utils

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
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
// El alcance de un token se verifica con los comodines {p}, {s} y {a} de la
// ruta, así que una ruta AccessToken tiene que tenerlos en su lugar. Si no, hace
// panic al registrarla (ver checkTokenPattern).
func (k *Kit) Handle(pattern string, a Access, h http.HandlerFunc) {
	if a == AccessToken {
		checkTokenPattern(pattern)
	}
	k.mux.Handle(pattern, k.authenticate(a, h))
}

// scopeWildcards asocia cada comodín de alcance al literal que va justo antes,
// y collectionWildcards al revés.
var (
	scopeWildcards      = map[string]string{"{p}": "projects", "{s}": "stages", "{a}": "apps"}
	collectionWildcards = map[string]string{"projects": "{p}", "stages": "{s}", "apps": "{a}"}
)

// checkTokenPattern hace panic si pattern no sirve para verificar el alcance
// de un token. Es un error de programación, como un patrón inválido del mux.
func checkTokenPattern(pattern string) {
	if problem := tokenPatternProblem(pattern); problem != "" {
		panic(fmt.Sprintf("utils: la ruta %q usa AccessToken pero %s; sin eso no se puede verificar el alcance del token", pattern, problem))
	}
}

// tokenPatternProblem revisa la ruta segmento por segmento, como la lee el mux:
//   - empieza con /api/v1/projects/{p} y no termina en "/";
//   - los comodines de alcance son {p}, {s} y {a}, cada uno una vez y justo
//     después de projects, stages y apps; además de {$} al final, solo se
//     permite otro comodín si es de un segmento y va después de {a}
//     (…/apps/{a}/deploys/{id}): ahí ya no puede tragarse un segmento de alcance;
//   - projects, stages y apps, si no son el último segmento, van seguidos de
//     su comodín, y {a} solo aparece después de {s};
//   - los literales se comparan decodificados (st%61ges es stages para el mux).
func tokenPatternProblem(pattern string) string {
	// Como el mux: el método va hasta el primer espacio o tab, y el host hasta la primera "/".
	rest := pattern
	if i := strings.IndexAny(pattern, " \t"); i >= 0 {
		rest = strings.TrimLeft(pattern[i+1:], " \t")
	}
	i := strings.IndexByte(rest, '/')
	if i < 0 {
		return "no empieza con /api/v1/projects/{p}"
	}
	path := rest[i:]
	if strings.HasSuffix(path, "/") {
		return `termina en "/" y abarcaría cualquier ruta debajo`
	}

	type segment struct {
		raw, s string // s es el literal decodificado, o el comodín tal cual
		wild   bool
	}
	var segs []segment
	for _, raw := range strings.Split(path[1:], "/") {
		seg := segment{raw: raw, s: raw, wild: strings.Contains(raw, "{")}
		if !seg.wild {
			if u, err := url.PathUnescape(raw); err == nil {
				seg.s = u
			}
		}
		segs = append(segs, seg)
	}
	literal := func(i int, s string) bool { return i >= 0 && i < len(segs) && !segs[i].wild && segs[i].s == s }
	if !literal(0, "api") || !literal(1, "v1") || !literal(2, "projects") || len(segs) < 4 || !segs[3].wild || segs[3].s != "{p}" {
		return "no empieza con /api/v1/projects/{p}"
	}

	seen := map[string]bool{}
	for i, seg := range segs {
		if seg.wild {
			if seg.s == "{$}" {
				continue // el mux exige que sea el último
			}
			before, ok := scopeWildcards[seg.s]
			switch {
			case !ok && seen["{a}"] && !strings.HasSuffix(seg.s, "...}"):
				continue // un id dentro de la app (…/deploys/{id}): el alcance ya está fijado
			case !ok:
				return fmt.Sprintf("tiene el comodín %s; solo se permiten {p}, {s} y {a} (y después de {a}, uno de un segmento como {id})", seg.s)
			case seen[seg.s]:
				return fmt.Sprintf("repite el comodín %s", seg.s)
			case !literal(i-1, before):
				return fmt.Sprintf("tiene %s fuera de su lugar: va justo después de /%s/", seg.s, before)
			case seg.s == "{a}" && !seen["{s}"]:
				return "tiene {a} sin {s} antes: las apps están dentro de un stage"
			}
			seen[seg.s] = true
			continue
		}
		want, ok := collectionWildcards[seg.s]
		if !ok || i+1 == len(segs) {
			continue // un literal cualquiera, o la colección al final (listar)
		}
		if next := segs[i+1]; !next.wild || (next.s != want && next.s != "{$}") {
			return fmt.Sprintf("tiene /%s/ seguido de %q en vez de %s", seg.s, next.raw, want)
		}
	}
	return ""
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
				k.Fail(w, r, err)
				return
			}
			if p.Token == nil {
				k.Fail(w, r, errTokenWithoutScope)
				return
			}
			write := r.Method != http.MethodGet && r.Method != http.MethodHead
			// Antes que el alcance: no dice si el recurso existe (contrato §8.1).
			if write && p.ReadOnly() {
				k.writeError(w, http.StatusForbidden, "read_only_token", "el token es de solo lectura")
				return
			}
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
			k.Fail(w, r, err)
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
