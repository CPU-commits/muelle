package entity

import (
	"time"
	"unicode/utf8"

	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// MaxTokenName es el largo máximo del nombre de un token de API.
const MaxTokenName = 64

// APIToken es un token de API con alcance de proyecto (sin stage), de
// proyecto + stage, o de todos los proyectos (solo de lectura) (§10.1 del
// spec, contrato §8.1). Se guarda solo su SHA-256.
type APIToken struct {
	ID          int64
	Name        string
	TokenHash   string
	ProjectID   int64  // 0 con AllProjects
	ProjectSlug string // vacío con AllProjects
	StageID     *int64
	StageSlug   string // vacío: todo el proyecto
	ReadOnly    bool   // solo lectura: GET/HEAD en REST, herramientas de lectura en MCP
	AllProjects bool   // todos los proyectos; siempre con ReadOnly
	CreatedBy   *int64 // nil si quien lo creó se borró: el token sigue, es del proyecto
	// CreatedByName es el username de CreatedBy, que completa el repositorio;
	// "" si ese usuario se borró.
	CreatedByName string
	CreatedAt     time.Time
	LastUsedAt    *time.Time
	ExpiresAt     *time.Time // nil: no vence
}

// NewAPIToken valida el nombre y arma un token nuevo, todavía sin id. Los
// slugs los completa el repositorio al guardarlo. El alcance lo valida
// CreateToken; ScopeValid lo vuelve a mirar al autenticar.
func NewAPIToken(name, tokenHash string, projectID int64, stageID, createdBy *int64, now time.Time, expiresAt *time.Time, readOnly, allProjects bool) (APIToken, error) {
	if err := validate.Name("name", name, MaxTokenName); err != nil {
		return APIToken{}, err
	}
	// El nombre vuelve en GET /tokens, y json/v2 no serializa UTF-8 inválido.
	if !utf8.ValidString(name) {
		return APIToken{}, validate.Errorf("name", "no es UTF-8 válido")
	}
	return APIToken{
		Name: name, TokenHash: tokenHash, ProjectID: projectID, StageID: stageID,
		CreatedBy: createdBy, CreatedAt: now, ExpiresAt: expiresAt,
		ReadOnly: readOnly, AllProjects: allProjects,
	}, nil
}

// Expired dice si en now el token ya venció.
func (t APIToken) Expired(now time.Time) bool {
	return t.ExpiresAt != nil && !now.Before(*t.ExpiresAt)
}

// ScopeValid dice si el alcance está completo. Uno de todos los proyectos es
// de solo lectura y no tiene proyecto ni stage. Los demás tienen siempre un
// proyecto, y slug de stage si y solo si el token tiene stage. Un alcance
// incompleto no se usa, porque Principal.Allows trataría un stage vacío como
// todo el proyecto.
func (t APIToken) ScopeValid() bool {
	if t.AllProjects {
		return t.ReadOnly && t.ProjectID == 0 && t.ProjectSlug == "" && t.StageID == nil && t.StageSlug == ""
	}
	return t.ProjectSlug != "" && (t.StageID == nil) == (t.StageSlug == "")
}

// Scope es el alcance que se pone en el Principal de una petición con este token.
func (t APIToken) Scope() shared.TokenScope {
	return shared.TokenScope{ID: t.ID, Name: t.Name, ProjectSlug: t.ProjectSlug, StageSlug: t.StageSlug,
		ReadOnly: t.ReadOnly, AllProjects: t.AllProjects}
}
