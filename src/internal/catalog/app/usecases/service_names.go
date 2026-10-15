package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/catalog/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/catalog/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/domain/compose"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// stageServices devuelve, para cada nombre de servicio del stage, la app que
// lo usa (§5.2 regla 2), salvo la app except. withForms dice si entran las apps
// de formulario (con su slug); las compose entran siempre, con los servicios de
// su compose (compose.Services: ya se validó al guardar). Corre dentro de la
// transacción del caso de uso.
func stageServices(ctx context.Context, apps repository.AppRepository, stageID, except int64, withForms bool) (map[string]string, error) {
	list, err := apps.ListByStage(ctx, stageID)
	if err != nil {
		return nil, err
	}
	owned := map[string]string{}
	for _, a := range list { // ordenadas por slug: el dueño de un nombre repetido es estable
		if a.ID == except {
			continue
		}
		switch a.Kind {
		case entity.KindService:
			if _, ok := owned[a.Slug]; withForms && !ok {
				owned[a.Slug] = a.Slug
			}
		case entity.KindCompose:
			for _, s := range compose.Services(a.ComposeYAML) {
				if _, ok := owned[s.Name]; !ok {
					owned[s.Name] = a.Slug
				}
			}
		}
	}
	return owned, nil
}

// checkServiceNames da un *validate.Error en field si un nombre ya lo usa otra
// app del stage. field es compose_yaml (los servicios de una app compose) o
// slug (el de una app de formulario, que solo choca con las compose).
func checkServiceNames(owned map[string]string, names []string, field string) error {
	for _, n := range names {
		app, ok := owned[n]
		if !ok {
			continue
		}
		if field == "slug" {
			return validate.Errorf(field, "%q ya es un servicio de la app compose %q del stage", n, app)
		}
		return validate.Errorf(field, "el servicio %q ya existe en la app %q del stage: los servicios de un stage comparten la red", n, app)
	}
	return nil
}
