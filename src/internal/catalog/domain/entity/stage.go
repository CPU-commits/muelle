package entity

import (
	"time"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// Stage es un entorno de un proyecto (staging, production…). Su slug es único dentro del proyecto.
type Stage struct {
	ID          int64
	ProjectID   int64
	ProjectSlug string
	Slug        string
	CreatedAt   time.Time
}

// NewStage valida el slug y lo asocia al proyecto. El ID lo asigna el repositorio.
func NewStage(project Project, slug string, now time.Time) (Stage, error) {
	if err := validate.StageSlug(slug); err != nil {
		return Stage{}, err
	}
	return Stage{ProjectID: project.ID, ProjectSlug: project.Slug, Slug: slug, CreatedAt: now}, nil
}
