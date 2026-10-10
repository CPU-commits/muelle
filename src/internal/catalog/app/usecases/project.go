package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/catalog/app/dto"
	"github.com/CPU-commits/muelle/src/internal/catalog/domain/entity"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

type CreateProject struct{ d Deps }

func NewCreateProject(d Deps) *CreateProject { return &CreateProject{d} }

func (uc *CreateProject) Execute(ctx context.Context, in dto.CreateProjectInput) (dto.ProjectOutput, error) {
	p, err := entity.NewProject(in.Slug, in.Name, uc.d.Clock.Now())
	if err != nil {
		return dto.ProjectOutput{}, err
	}
	if err := uc.d.Projects.Create(ctx, &p); err != nil {
		return dto.ProjectOutput{}, err
	}
	uc.d.Audit.Record(ctx, "project.create", projectTarget(p.Slug), nil)
	return dto.ProjectFrom(p), nil
}

type ListProjects struct{ d Deps }

func NewListProjects(d Deps) *ListProjects { return &ListProjects{d} }

func (uc *ListProjects) Execute(ctx context.Context) ([]dto.ProjectOutput, error) {
	ps, err := uc.d.Projects.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ProjectOutput, 0, len(ps))
	for _, p := range ps {
		out = append(out, dto.ProjectFrom(p))
	}
	return out, nil
}

type GetProject struct{ d Deps }

func NewGetProject(d Deps) *GetProject { return &GetProject{d} }

func (uc *GetProject) Execute(ctx context.Context, slug string) (dto.ProjectOutput, error) {
	p, err := uc.d.Projects.GetBySlug(ctx, slug)
	if err != nil {
		return dto.ProjectOutput{}, err
	}
	return dto.ProjectFrom(p), nil
}

type UpdateProject struct{ d Deps }

func NewUpdateProject(d Deps) *UpdateProject { return &UpdateProject{d} }

func (uc *UpdateProject) Execute(ctx context.Context, in dto.UpdateProjectInput) (dto.ProjectOutput, error) {
	var p entity.Project
	err := uc.d.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if p, err = uc.d.Projects.GetBySlug(ctx, in.Slug); err != nil {
			return err
		}
		if err := p.Rename(in.Name); err != nil {
			return err
		}
		return uc.d.Projects.Update(ctx, p)
	})
	if err != nil {
		return dto.ProjectOutput{}, err
	}
	uc.d.Audit.Record(ctx, "project.update", projectTarget(p.Slug), nil)
	return dto.ProjectFrom(p), nil
}

type DeleteProject struct{ d Deps }

func NewDeleteProject(d Deps) *DeleteProject { return &DeleteProject{d} }

// Execute borra un proyecto sin stages; con stages devuelve ErrNotEmpty.
// El chequeo y el borrado van en la misma transacción.
func (uc *DeleteProject) Execute(ctx context.Context, slug string) error {
	err := uc.d.Tx.InTx(ctx, func(ctx context.Context) error {
		p, err := uc.d.Projects.GetBySlug(ctx, slug)
		if err != nil {
			return err
		}
		n, err := uc.d.Stages.CountByProject(ctx, p.ID)
		if err != nil {
			return err
		}
		if n > 0 {
			return srepo.ErrNotEmpty
		}
		return uc.d.Projects.Delete(ctx, p.ID)
	})
	if err != nil {
		return err
	}
	uc.d.Audit.Record(ctx, "project.delete", projectTarget(slug), nil)
	return nil
}
