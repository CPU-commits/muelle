package service

import (
	"context"
	"maps"

	"github.com/CPU-commits/muelle/src/shared/domain/compose"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
)

// Config es la configuración de la app que leyó un deploy (paso 2): el
// modelo base, sin las inyecciones de §6.2, y la ref de cada servicio.
type Config struct {
	File       *compose.File
	Images     map[string]string // servicio → ref de la configuración (config_images)
	HasVolumes bool              // algún servicio monta un volumen nombrado
}

// ReadConfig arma la Config. Formulario: un servicio con el slug de la app,
// con todas las variables como environment (§5.2 regla 3), el command en
// forma exec, el healthcheck de §6.1, los volúmenes nombrados, replicas y
// resources.limits. Compose: compose.Load con env (la interpolación estricta y
// la validación entera de §6.3, ya con los valores). Sus errores son
// *RenderError, salvo un ctx cancelado. env no se modifica ni se comparte.
func ReadConfig(ctx context.Context, app App, env map[string]string) (Config, error) {
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}
	var f *compose.File
	switch app.Kind {
	case KindService:
		spec, err := ParseFormSpec(app.Spec)
		if err != nil {
			return Config{}, err
		}
		f = &compose.File{Services: map[string]compose.Service{app.Slug: spec.service(env)}}
		for _, v := range spec.Volumes {
			if f.Volumes == nil {
				f.Volumes = map[string]compose.Volume{}
			}
			f.Volumes[v.Name] = compose.Volume{}
		}
	case KindCompose:
		var err error
		f, err = compose.Load(ctx, app.ComposeYAML, naming.Stack(app.Project, app.Stage, app.Slug), maps.Clone(env))
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return Config{}, ctxErr
			}
			return Config{}, &RenderError{Msg: err.Error()}
		}
	default:
		return Config{}, renderErrorf("la app %s tiene un tipo desconocido (%q)", app.Slug, app.Kind)
	}
	cfg := Config{File: f, Images: make(map[string]string, len(f.Services))}
	for name, svc := range f.Services {
		cfg.Images[name] = svc.Image
		if hasNamedVolume(svc) {
			cfg.HasVolumes = true
		}
	}
	return cfg, nil
}

// hasNamedVolume dice si el servicio monta un volumen nombrado (un tmpfs no
// cuenta): §6.2 le da stop-first y lo fija al nodo de los volúmenes.
func hasNamedVolume(svc compose.Service) bool {
	for _, m := range svc.Volumes {
		if m.Type == "volume" {
			return true
		}
	}
	return false
}
