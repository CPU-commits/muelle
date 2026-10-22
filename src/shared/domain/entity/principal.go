// Package entity tiene las entidades que comparten todos los módulos.
package entity

import "context"

type ActorType string

const (
	ActorUser   ActorType = "user"
	ActorToken  ActorType = "token"
	ActorSystem ActorType = "system"
)

// TokenScope es el alcance de un token de API (§10.1 del spec, contrato §8.1).
type TokenScope struct {
	ID          int64
	Name        string
	ProjectSlug string // vacío solo con AllProjects
	StageSlug   string // vacío: todo el proyecto
	ReadOnly    bool   // solo GET/HEAD en REST, solo herramientas de lectura en MCP
	AllProjects bool   // todos los proyectos; solo vale con ReadOnly
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

// ReadOnly dice si el principal es un token de solo lectura (también uno de
// todos los proyectos). Una sesión nunca lo es.
func (p Principal) ReadOnly() bool { return p.Token != nil && p.Token.ReadOnly }

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

// Allows dice si el principal puede operar sobre project/stage (contrato
// §8.1), con estas reglas en orden:
//  1. una sesión puede todo;
//  2. un token de solo lectura no escribe;
//  3. un proyecto vacío nunca autoriza a un token;
//  4. un token de todos los proyectos lee cualquier proyecto y stage. Uno que
//     no es de solo lectura, o que trae un proyecto o un stage, no autoriza
//     nada: no debería existir (la base y ScopeValid lo impiden);
//  5. si no, el token tiene que ser de ese proyecto; si está limitado a un
//     stage, solo puede tocar ese stage, y sobre recursos del proyecto (stage
//     vacío) solo puede leer. Un proyecto vacío en el token no autoriza nada.
func (p Principal) Allows(project, stage string, write bool) bool {
	t := p.Token
	if t == nil {
		return true
	}
	if write && t.ReadOnly {
		return false
	}
	if project == "" {
		return false
	}
	if t.AllProjects {
		return t.ReadOnly && !write && t.ProjectSlug == "" && t.StageSlug == ""
	}
	if t.ProjectSlug == "" || project != t.ProjectSlug {
		return false
	}
	if t.StageSlug == "" {
		return true
	}
	if stage == "" {
		return !write
	}
	return stage == t.StageSlug
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// ViaMCP es el via de lo que se hace por el servidor MCP (contrato §8.6): la
// auditoría lo suma a details como "via": "mcp".
const ViaMCP = "mcp"

type viaKey struct{}

// WithVia marca el ctx con el canal por el que llegó la operación (ViaMCP).
// La auditoría lo copia en details.
func WithVia(ctx context.Context, via string) context.Context {
	return context.WithValue(ctx, viaKey{}, via)
}

// ViaFrom devuelve el canal que marcó WithVia, si hay uno.
func ViaFrom(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(viaKey{}).(string)
	return v, ok
}
