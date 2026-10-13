package usecases

import (
	"context"
	"errors"

	"github.com/CPU-commits/muelle/src/internal/catalog/app/dto"
	"github.com/CPU-commits/muelle/src/internal/catalog/domain/entity"
	sharedentity "github.com/CPU-commits/muelle/src/shared/domain/entity"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

type CreateStage struct{ d Deps }

func NewCreateStage(d Deps) *CreateStage { return &CreateStage{d} }

// Execute crea la red del stage (Networks, si hay: ingress crea la overlay y
// conecta Caddy, §3.2) y guarda el stage. La red va primero: si Docker no
// responde, no queda un stage sin red. Como crearla es idempotente, una red
// que queda sin stage (un insert que falla después) no molesta: la próxima
// vez se reusa.
func (uc *CreateStage) Execute(ctx context.Context, in dto.CreateStageInput) (dto.StageOutput, error) {
	if uc.d.Networks != nil {
		// Lo que se puede saber sin Docker se responde antes (404, 422, 409).
		p, err := uc.d.Projects.GetBySlug(ctx, in.Project)
		if err != nil {
			return dto.StageOutput{}, err
		}
		if _, err := entity.NewStage(p, in.Slug, uc.d.Clock.Now()); err != nil {
			return dto.StageOutput{}, err
		}
		switch _, err := uc.d.Stages.Get(ctx, in.Project, in.Slug); {
		case err == nil:
			return dto.StageOutput{}, srepo.ErrConflict
		case !errors.Is(err, srepo.ErrNotFound):
			return dto.StageOutput{}, err
		}
		if err := uc.d.Networks.EnsureStageNetwork(ctx, in.Project, in.Slug); err != nil {
			return dto.StageOutput{}, err
		}
	}
	// La lectura del proyecto y el insert van en una transacción; si el
	// proyecto se borra en paralelo, el insert da ErrNotFound y no un error interno.
	var s entity.Stage
	err := uc.d.Tx.InTx(ctx, func(ctx context.Context) error {
		p, err := uc.d.Projects.GetBySlug(ctx, in.Project)
		if err != nil {
			return err
		}
		if s, err = entity.NewStage(p, in.Slug, uc.d.Clock.Now()); err != nil {
			return err
		}
		return uc.d.Stages.Create(ctx, &s)
	})
	if err != nil {
		return dto.StageOutput{}, err
	}
	uc.d.Audit.Record(ctx, "stage.create", stageTarget(s.ProjectSlug, s.Slug), nil)
	return dto.StageFrom(s), nil
}

type ListStages struct{ d Deps }

func NewListStages(d Deps) *ListStages { return &ListStages{d} }

func (uc *ListStages) Execute(ctx context.Context, project string) ([]dto.StageOutput, error) {
	p, err := uc.d.Projects.GetBySlug(ctx, project)
	if err != nil {
		return nil, err
	}
	ss, err := uc.d.Stages.ListByProject(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	// Un token limitado a un stage solo ve ese stage (§10.1 del spec): una sola regla, Principal.Allows.
	pr, hasPrincipal := sharedentity.PrincipalFrom(ctx)
	out := make([]dto.StageOutput, 0, len(ss))
	for _, s := range ss {
		if hasPrincipal && !pr.Allows(project, s.Slug, false) {
			continue
		}
		out = append(out, dto.StageFrom(s))
	}
	return out, nil
}

type GetStage struct{ d Deps }

func NewGetStage(d Deps) *GetStage { return &GetStage{d} }

func (uc *GetStage) Execute(ctx context.Context, project, stage string) (dto.StageOutput, error) {
	s, err := uc.d.Stages.Get(ctx, project, stage)
	if err != nil {
		return dto.StageOutput{}, err
	}
	return dto.StageFrom(s), nil
}

type DeleteStage struct{ d Deps }

func NewDeleteStage(d Deps) *DeleteStage { return &DeleteStage{d} }

// Execute borra un stage sin apps (sus variables se borran en cascada); con
// apps devuelve ErrNotEmpty. El chequeo y el borrado van en la misma
// transacción. Con Networks, entre medio se borra la red del stage (y se
// desconecta Caddy): si Docker no responde, el stage queda. Si una app
// aparece en el stage después del chequeo, el borrado da ErrNotEmpty y la red
// se vuelve a crear con su próximo deploy.
func (uc *DeleteStage) Execute(ctx context.Context, project, stage string) error {
	empty := func(ctx context.Context) (entity.Stage, error) {
		s, err := uc.d.Stages.Get(ctx, project, stage)
		if err != nil {
			return s, err
		}
		n, err := uc.d.Apps.CountByStage(ctx, s.ID)
		if err != nil {
			return s, err
		}
		if n > 0 {
			return s, srepo.ErrNotEmpty
		}
		return s, nil
	}
	if uc.d.Networks != nil {
		if _, err := empty(ctx); err != nil {
			return err
		}
		if err := uc.d.Networks.RemoveStageNetwork(ctx, project, stage); err != nil {
			return err
		}
	}
	err := uc.d.Tx.InTx(ctx, func(ctx context.Context) error {
		s, err := empty(ctx)
		if err != nil {
			return err
		}
		return uc.d.Stages.Delete(ctx, s.ID)
	})
	if err != nil {
		return err
	}
	uc.d.Audit.Record(ctx, "stage.delete", stageTarget(project, stage), nil)
	return nil
}
