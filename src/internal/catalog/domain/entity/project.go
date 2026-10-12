// Package entity tiene las entidades del catálogo: proyectos, stages y apps.
package entity

import (
	"time"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

const maxProjectNameLen = 64

// Project agrupa los stages de un producto. Su slug es único.
type Project struct {
	ID        int64
	Slug      string
	Name      string
	CreatedAt time.Time
}

// NewProject valida slug y nombre. El ID lo asigna el repositorio al guardar.
func NewProject(slug, name string, now time.Time) (Project, error) {
	if err := validate.ProjectSlug(slug); err != nil {
		return Project{}, err
	}
	if err := validate.Name("name", name, maxProjectNameLen); err != nil {
		return Project{}, err
	}
	return Project{Slug: slug, Name: name, CreatedAt: now}, nil
}

// Rename cambia el nombre, con las mismas reglas que NewProject.
func (p *Project) Rename(name string) error {
	if err := validate.Name("name", name, maxProjectNameLen); err != nil {
		return err
	}
	p.Name = name
	return nil
}
