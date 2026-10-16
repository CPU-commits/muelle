// Package entity tiene las entidades que comparten todos los módulos.
package entity

import "context"

type ActorType string

const (
	ActorUser   ActorType = "user"
	ActorToken  ActorType = "token"
	ActorSystem ActorType = "system"
)

// TokenScope es el alcance de un token de API (§10.1 del spec).
type TokenScope struct {
	ID          int64
	Name        string
	ProjectSlug string
	StageSlug   string // vacío: todo el proyecto
}

// Principal es quien hace la petición: una sesión de usuario o un token de API.
// Lo produce identity y lo consultan los demás módulos.
type Principal struct {
	UserID         int64
	Username       string
	SessionHash    string
	MFAPending     bool // contraseña correcta, falta el código TOTP
	NeedsTOTPSetup bool // require_2fa activo y el usuario no tiene TOTP
	Token          *TokenScope
}

func (p Principal) IsToken() bool { return p.Token != nil }

// Actor identifica al autor de un evento de auditoría.
func (p Principal) Actor() (ActorType, *int64) {
	if p.Token != nil {
		id := p.Token.ID
		return ActorToken, &id
	}
	if p.UserID == 0 {
		return ActorSystem, nil
	}
	id := p.UserID
	return ActorUser, &id
}

// ActorSystemName es el actor_name de un evento del sistema (sin Principal, o
// con uno sin usuario ni token): los jobs, el arranque y la CLI.
const ActorSystemName = string(ActorSystem)

// ActorName es el nombre del autor que Actor identifica, para copiarlo en la
// auditoría al escribir el evento (sigue aunque se borre el usuario o el
// token): el nombre del token, el username de una sesión, o ActorSystemName.
// Un Principal armado a mano tiene que traer Username o Token.Name.
func (p Principal) ActorName() string {
	switch typ, _ := p.Actor(); typ {
	case ActorToken:
		return p.Token.Name
	case ActorUser:
		return p.Username
	default:
		return ActorSystemName
	}
}

// Allows dice si el principal puede operar sobre project/stage.
// Una sesión puede todo. Un token tiene que ser de ese proyecto; si está
// limitado a un stage, solo puede tocar ese stage, y sobre recursos del
// proyecto (stage vacío) solo puede leer. Un proyecto vacío, en la ruta o en
// el token, nunca autoriza a un token.
func (p Principal) Allows(project, stage string, write bool) bool {
	if p.Token == nil {
		return true
	}
	if project == "" || p.Token.ProjectSlug == "" || project != p.Token.ProjectSlug {
		return false
	}
	if p.Token.StageSlug == "" {
		return true
	}
	if stage == "" {
		return !write
	}
	return stage == p.Token.StageSlug
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}
