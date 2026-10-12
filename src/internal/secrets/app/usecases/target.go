// Package usecases tiene los casos de uso de secrets: leer las variables
// enmascaradas, reemplazarlas (PUT), modificarlas (PATCH) y mezclarlas para el deploy.
package usecases

import (
	"context"
	"slices"

	"github.com/CPU-commits/muelle/src/internal/secrets/app/dto"
	"github.com/CPU-commits/muelle/src/internal/secrets/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/secrets/domain/service"
)

// resolveOwner traduce los slugs al dueño: la app si viene, o el stage.
func resolveOwner(ctx context.Context, owners service.OwnerResolver, t dto.EnvTarget) (entity.Owner, error) {
	if t.App != "" {
		return owners.App(ctx, t.Project, t.Stage, t.App)
	}
	return owners.Stage(ctx, t.Project, t.Stage)
}

// auditTarget es el target de los eventos de auditoría: stage:<p>/<s> o app:<p>/<s>/<a>.
func auditTarget(t dto.EnvTarget) string {
	if t.App != "" {
		return "app:" + t.Project + "/" + t.Stage + "/" + t.App
	}
	return "stage:" + t.Project + "/" + t.Stage
}

// secretOrDefault aplica el valor por defecto de secret: true si se omite.
func secretOrDefault(secret *bool) bool { return secret == nil || *secret }

// keysOf devuelve las claves de vars (que ya vienen ordenadas), nunca los valores.
func keysOf(vars []entity.EnvVar) []string {
	out := make([]string, 0, len(vars))
	for _, v := range vars {
		out = append(out, v.Key)
	}
	return out
}

// sortedUnique devuelve una copia ordenada y sin repetidos.
func sortedUnique(keys []string) []string {
	return slices.Compact(slices.Sorted(slices.Values(keys)))
}
