package entity

import (
	"time"
	"unicode/utf8"

	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// MaxTokenName es el largo máximo del nombre de un token de API.
const MaxTokenName = 64

// APIToken es un token de CI con alcance de proyecto (sin stage) o de
// proyecto + stage (§10.1 del spec). Se guarda solo su SHA-256.
type APIToken struct {
	ID          int64
	Name        string
	TokenHash   string
	ProjectID   int64
	ProjectSlug string
	StageID     *int64
	StageSlug   string // vacío: todo el proyecto
	CreatedBy   *int64
	CreatedAt   time.Time
	LastUsedAt  *time.Time
	ExpiresAt   *time.Time // nil: no vence
}

// NewAPIToken valida el nombre y arma un token nuevo, todavía sin id. Los
// slugs los completa el repositorio al guardarlo.
func NewAPIToken(name, tokenHash string, projectID int64, stageID, createdBy *int64, now time.Time, expiresAt *time.Time) (APIToken, error) {
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
	}, nil
}

// Expired dice si en now el token ya venció.
func (t APIToken) Expired(now time.Time) bool {
	return t.ExpiresAt != nil && !now.Before(*t.ExpiresAt)
}

// Scope es el alcance que se pone en el Principal de una petición con este token.
func (t APIToken) Scope() shared.TokenScope {
	return shared.TokenScope{ID: t.ID, Name: t.Name, ProjectSlug: t.ProjectSlug, StageSlug: t.StageSlug}
}
