package usecases

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"errors"

	"github.com/CPU-commits/muelle/src/internal/catalog/app/dto"
	"github.com/CPU-commits/muelle/src/internal/catalog/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/catalog/domain/service"
	"github.com/CPU-commits/muelle/src/internal/catalog/domain/vo"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// hasJSON dice si el campo vino en el cuerpo y no es null.
func hasJSON(v jsontext.Value) bool {
	t := bytes.TrimSpace(v)
	return len(t) > 0 && string(t) != "null"
}

// kindMismatch traduce el error de tipo de la entidad al del módulo.
func kindMismatch(err error) error {
	if errors.Is(err, entity.ErrWrongKind) {
		return service.ErrKindMismatch
	}
	return err
}

type CreateApp struct{ d Deps }

func NewCreateApp(d Deps) *CreateApp { return &CreateApp{d} }

// Execute crea una app. Las de formulario guardan el spec normalizado; las
// compose se validan con las reglas de §6.3 del diseño. Ningún nombre de
// servicio se repite en el stage (§5.2 regla 2): el de una app de formulario
// es su slug, que no puede ser un servicio de una compose; los de una compose
// no pueden ser de otra app. Un slug que ya usa otra app de formulario sigue
// siendo ErrConflict, del índice único.
func (uc *CreateApp) Execute(ctx context.Context, in dto.CreateAppInput) (dto.AppOutput, error) {
	if err := validate.AppSlug(in.Slug); err != nil {
		return dto.AppOutput{}, err
	}
	var spec vo.AppSpec
	var services []string // los de la compose
	switch entity.Kind(in.Kind) {
	case entity.KindService:
		if in.ComposeYAML != nil {
			return dto.AppOutput{}, validate.Errorf("compose_yaml", "no aplica a apps de tipo service")
		}
		var err error
		if spec, err = vo.ParseAppSpec(in.Spec); err != nil {
			return dto.AppOutput{}, err
		}
	case entity.KindCompose:
		if hasJSON(in.Spec) {
			return dto.AppOutput{}, validate.Errorf("spec", "no aplica a apps de tipo compose")
		}
		if in.ComposeYAML == nil {
			return dto.AppOutput{}, validate.Errorf("compose_yaml", "es obligatorio")
		}
		var err error
		if services, err = entity.ValidateCompose(*in.ComposeYAML); err != nil {
			return dto.AppOutput{}, err
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

	// La lectura del stage y el insert van en una transacción; si el stage se
	// borra en paralelo, el insert da ErrNotFound y no un error interno.
	var app entity.App
	err := uc.d.Tx.InTx(ctx, func(ctx context.Context) error {
		stage, err := uc.d.Stages.Get(ctx, in.Project, in.Stage)
		if err != nil {
			return err
		}
		// Las de formulario solo chocan con los servicios de las compose (con
		// otra de formulario es el 409 del índice único); las compose, con
		// todos.
		kind := entity.Kind(in.Kind)
		owned, err := stageServices(ctx, uc.d.Apps, stage.ID, 0, kind == entity.KindCompose)
		if err != nil {
			return err
		}
		if kind == entity.KindService {
			err = checkServiceNames(owned, []string{in.Slug}, "slug")
		} else {
			err = checkServiceNames(owned, services, "compose_yaml")
		}
		if err != nil {
			return err
		}
		now := uc.d.Clock.Now()
		if kind == entity.KindService {
			app, err = entity.NewServiceApp(stage, in.Slug, spec, timeout, now)
		} else {
			app, err = entity.NewComposeApp(stage, in.Slug, *in.ComposeYAML, timeout, now)
		}
		if err != nil {
			return err
		}
		return uc.d.Apps.Create(ctx, &app)
	})
	if err != nil {
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
	var spec *vo.AppSpec
	var services []string // los del compose nuevo
	if hasJSON(in.Spec) {
		parsed, err := vo.ParseAppSpec(in.Spec)
		if err != nil {
			return dto.AppOutput{}, err
		}
		spec = &parsed
		fields = append(fields, "spec")
	}
	if in.ComposeYAML != nil {
		var err error
		if services, err = entity.ValidateCompose(*in.ComposeYAML); err != nil {
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
		if spec != nil {
			if err := app.ChangeSpec(*spec, now); err != nil {
				return kindMismatch(err)
			}
		}
		if in.ComposeYAML != nil {
			if err := app.ChangeCompose(*in.ComposeYAML, now); err != nil {
				return kindMismatch(err)
			}
			// Sus propios servicios no cuentan (except): editar sin cambiar
			// los nombres no choca consigo misma.
			owned, err := stageServices(ctx, uc.d.Apps, app.StageID, app.ID, true)
			if err != nil {
				return err
			}
			if err := checkServiceNames(owned, services, "compose_yaml"); err != nil {
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

// Execute borra la app: con Runtime, primero su stack (docker stack rm, que
// hace deploy); después la fila, con sus variables y deploys en cascada. Una
// app con dominios devuelve ErrNotEmpty sin tocar nada: primero se borran sus
// dominios, que limpian sus registros de Cloudflare (§9). Si Docker no
// responde, la app queda y se puede reintentar.
//
// Un dominio creado entre el conteo y el borrado no pasa: la clave foránea
// (domains.app_id RESTRICT) hace fallar el DELETE, que el repositorio devuelve
// como ErrNotEmpty (409), no como un error interno. En esa carrera el stack ya
// se borró y la app queda sin servicios, hasta el próximo deploy.
func (uc *DeleteApp) Execute(ctx context.Context, project, stage, slug string) error {
	if uc.d.Runtime != nil || uc.d.Domains != nil {
		a, err := uc.d.Apps.Get(ctx, project, stage, slug)
		if err != nil {
			return err
		}
		if uc.d.Domains != nil {
			n, err := uc.d.Domains.CountByApp(ctx, a.ID)
			if err != nil {
				return err
			}
			if n > 0 {
				return srepo.ErrNotEmpty
			}
		}
		if uc.d.Runtime != nil {
			if err := uc.d.Runtime.RemoveApp(ctx, project, stage, slug); err != nil {
				return err
			}
		}
	}
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
