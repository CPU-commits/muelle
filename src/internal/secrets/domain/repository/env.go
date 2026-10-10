// Package repository define cómo secrets guarda sus variables.
package repository

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/secrets/domain/entity"
)

// EnvRepository guarda y lee variables ya cifradas: no cifra ni valida.
// Con el ctx de un InTx participa en esa transacción.
type EnvRepository interface {
	// List devuelve las variables del dueño ordenadas por clave.
	List(ctx context.Context, o entity.Owner) ([]entity.EncryptedEnvVar, error)
	// Replace borra todas las variables del dueño y guarda vars.
	Replace(ctx context.Context, o entity.Owner, vars []entity.EncryptedEnvVar) error
	// Patch agrega o reemplaza las variables de set y borra las claves de
	// unset. Borrar una clave que no existe no es un error.
	Patch(ctx context.Context, o entity.Owner, set []entity.EncryptedEnvVar, unset []string) error
}
