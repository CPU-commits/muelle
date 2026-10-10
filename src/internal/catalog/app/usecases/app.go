package usecases

import (
	"bytes"
	"context"
	"encoding/json/jsontext"

	"github.com/CPU-commits/muelle/src/internal/catalog/app/dto"
	"github.com/CPU-commits/muelle/src/internal/catalog/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/catalog/domain/service"
	"github.com/CPU-commits/muelle/src/internal/catalog/domain/vo"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// hasJSON dice si el campo vino en el cuerpo y no es null.
func hasJSON(v jsontext.Value) bool {
	t := bytes.TrimSpace(v)
	return len(t) > 0 && string(t) != "null"
}

type CreateApp struct{ d Deps }

func NewCreateApp(d Deps) *CreateApp { return &CreateApp{d} }

// Execute crea una app. Las de formulario guardan el spec normalizado; las
// compose solo se validan como no vacías y de hasta 256 KiB.
func (uc *CreateApp) Execute(ctx context.Context, in dto.CreateAppInput) (dto.AppOutput, error) {
	if err := validate.AppSlug(in.Slug); err != nil {
		return dto.AppOutput{}, err
	}
	var specJSON []byte
	switch entity.Kind(in.Kind) {
	case entity.KindService:
		if in.ComposeYAML != nil {
			return dto.AppOutput{}, validate.Errorf("compose_yaml", "no aplica a apps de tipo service")
		}
		spec, err := vo.ParseAppSpec(in.Spec)
		if err != nil {
			return dto.AppOutput{}, err
		}
		specJSON = spec.JSON()
	case entity.KindCompose:
		if hasJSON(in.Spec) {
			return dto.AppOutput{}, validate.Errorf("spec", "no aplica a apps de tipo compose")
		}
		if in.ComposeYAML == nil {
			return dto.AppOutput{}, validate.Errorf("compose_yaml", "es obligatorio")
		}
	default:
		return dto.AppOutput{}, validate.Errorf("kind", "debe ser service o compose")
	}
	timeout := 0
	if in.DeployTimeoutS != nil {
		if err := entity.ValidateDeployTimeout(*in.DeployTimeoutS); err != nil {
			return dto.AppOutput{}, err
		}
		timeout = *in.DeployTimeoutS
	}

	stage, err := uc.d.Stages.Get(ctx, in.Project, in.Stage)
	if err != nil {
		return dto.AppOutput{}, err
	}
	now := uc.d.Clock.Now()
	var app entity.App
	if entity.Kind(in.Kind) == entity.KindService {
		app, err = entity.NewServiceApp(stage, in.Slug, specJSON, timeout, now)
	} else {
		app, err = entity.NewComposeApp(stage, in.Slug, *in.ComposeYAML, timeout, now)
	}
	if err != nil {
		return dto.AppOutput{}, err
	}
	if err := uc.d.Apps.Create(ctx, &app); err != nil {
		return dto.AppOutput{}, err
	}
	uc.d.Audit.Record(ctx, "app.create", appTarget(app.ProjectSlug, app.StageSlug, app.Slug),
		map[string]string{"kind": string(app.Kind)})
	return dto.AppFrom(app), nil
}

type ListApps struct{ d Deps }

func NewListApps(d Deps) *ListApps { return &ListApps{d} }

func (uc *ListApps) Execute(ctx context.Context, project, stage string) ([]dto.AppOutput, error) {
	s, err := uc.d.Stages.Get(ctx, project, stage)
	if err != nil {
		return nil, err
	}
	apps, err := uc.d.Apps.ListByStage(ctx, s.ID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.AppOutput, 0, len(apps))
	for _, a := range apps {
		out = append(out, dto.AppFrom(a))
	}
	return out, nil
}

type GetApp struct{ d Deps }

func NewGetApp(d Deps) *GetApp { return &GetApp{d} }

func (uc *GetApp) Execute(ctx context.Context, project, stage, slug string) (dto.AppOutput, error) {
	a, err := uc.d.Apps.Get(ctx, project, stage, slug)
	if err != nil {
		return dto.AppOutput{}, err
	}
	return dto.AppFrom(a), nil
}

type UpdateApp struct{ d Deps }

func NewUpdateApp(d Deps) *UpdateApp { return &UpdateApp{d} }

// Execute aplica los campos presentes. Cambiar el spec o el compose actualiza
// config_changed_at (cambios pendientes); cambiar solo el timeout, no.
// Un campo que no corresponde al tipo de la app da ErrKindMismatch.
func (uc *UpdateApp) Execute(ctx context.Context, in dto.UpdateAppInput) (dto.AppOutput, error) {
	var fields []string
	var specJSON []byte
	if hasJSON(in.Spec) {
		spec, err := vo.ParseAppSpec(in.Spec)
		if err != nil {
			return dto.AppOutput{}, err
		}
		specJSON = spec.JSON()
		fields = append(fields, "spec")
	}
	if in.ComposeYAML != nil {
		if err := entity.ValidateCompose(*in.ComposeYAML); err != nil {
			return dto.AppOutput{}, err
		}
		fields = append(fields, "compose_yaml")
	}
	if in.DeployTimeoutS != nil {
		if err := entity.ValidateDeployTimeout(*in.DeployTimeoutS); err != nil {
			return dto.AppOutput{}, err
		}
		fields = append(fields, "deploy_timeout_s")
	}

	var app entity.App
	err := uc.d.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if app, err = uc.d.Apps.Get(ctx, in.Project, in.Stage, in.Slug); err != nil {
			return err
		}
		if len(fields) == 0 {
			return nil
		}
		now := uc.d.Clock.Now()
		if specJSON != nil {
			if app.Kind != entity.KindService {
				return service.ErrKindMismatch
			}
			app.ChangeSpec(specJSON, now)
		}
		if in.ComposeYAML != nil {
			if app.Kind != entity.KindCompose {
				return service.ErrKindMismatch
			}
			if err := app.ChangeCompose(*in.ComposeYAML, now); err != nil {
				return err
			}
		}
		if in.DeployTimeoutS != nil {
			if err := app.SetDeployTimeout(*in.DeployTimeoutS); err != nil {
				return err
			}
		}
		return uc.d.Apps.Update(ctx, app)
	})
	if err != nil {
		return dto.AppOutput{}, err
	}
	if len(fields) > 0 {
		uc.d.Audit.Record(ctx, "app.update", appTarget(in.Project, in.Stage, in.Slug),
			map[string][]string{"fields": fields})
	}
	return dto.AppFrom(app), nil
}

type DeleteApp struct{ d Deps }

func NewDeleteApp(d Deps) *DeleteApp { return &DeleteApp{d} }

// Execute solo borra el registro (y sus variables en cascada). La Fase 2 agrega `docker stack rm`.
func (uc *DeleteApp) Execute(ctx context.Context, project, stage, slug string) error {
	err := uc.d.Tx.InTx(ctx, func(ctx context.Context) error {
		a, err := uc.d.Apps.Get(ctx, project, stage, slug)
		if err != nil {
			return err
		}
		return uc.d.Apps.Delete(ctx, a.ID)
	})
	if err != nil {
		return err
	}
	uc.d.Audit.Record(ctx, "app.delete", appTarget(project, stage, slug), nil)
	return nil
}
