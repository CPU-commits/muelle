package usecases

import (
	"context"
	"maps"
	"slices"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/compose"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// CreateDeploy encola un deploy (POST …/deploys, contrato §2.2).
type CreateDeploy struct{ d Deps }

func NewCreateDeploy(d Deps) *CreateDeploy { return &CreateDeploy{d: d} }

// CheckApp es el 404 de una app que no existe, antes de leer el cuerpo: el
// orden de los errores es app (404), cuerpo (400) y validación (422).
func (u *CreateDeploy) CheckApp(ctx context.Context, ref dto.AppRef) error {
	_, err := u.d.Apps.Get(ctx, ref.Project, ref.Stage, ref.App)
	return err
}

// Execute: la app (404; vuelve a leerla, por si se borró después de
// CheckApp), que acepte deploys (409 mientras se borra), los overrides
// (422) y el actor, y encola. No llama a Docker: con Docker caído igual se
// encola, y el deploy falla al correr.
func (u *CreateDeploy) Execute(ctx context.Context, in dto.CreateDeploy) (dto.Queued, error) {
	app, err := u.d.acceptingApp(ctx, in.AppRef)
	if err != nil {
		return dto.Queued{}, err
	}
	overrides, err := overridesFor(app, in)
	if err != nil {
		return dto.Queued{}, err
	}
	a, err := actorOf(ctx)
	if err != nil {
		return dto.Queued{}, err
	}
	return u.d.enqueue(ctx, app, a, func(context.Context) (map[string]string, error) { return overrides, nil })
}

// overridesFor valida el cuerpo (contrato §2.2) y devuelve los overrides por
// servicio: image (solo una app de formulario: su único servicio se llama
// como la app) o images, contra los servicios de la configuración actual.
func overridesFor(app service.App, in dto.CreateDeploy) (map[string]string, error) {
	switch {
	case in.Image != nil && in.Images != nil:
		return nil, validate.Errorf("image", "manda image o images, no los dos")
	case in.Image != nil && app.Kind == service.KindCompose:
		return nil, validate.Errorf("image", "una app compose recibe images, por servicio")
	case in.Image != nil:
		if err := validate.ImageRef("image", *in.Image); err != nil {
			return nil, err
		}
		return map[string]string{app.Slug: *in.Image}, nil
	}
	services := configServices(app)
	for _, svc := range slices.Sorted(maps.Keys(in.Images)) {
		field := "images." + svc
		if !slices.Contains(services, svc) {
			return nil, validate.Errorf(field, "el servicio no existe en la app")
		}
		if err := validate.ImageRef(field, in.Images[svc]); err != nil {
			return nil, err
		}
	}
	return maps.Clone(in.Images), nil
}

// configServices son los servicios de la configuración guardada: el slug en
// una app de formulario, los del compose en una compose (sin interpolar).
func configServices(app service.App) []string {
	if app.Kind != service.KindCompose {
		return []string{app.Slug}
	}
	var out []string
	for _, s := range compose.Services(app.ComposeYAML) {
		out = append(out, s.Name)
	}
	return out
}
