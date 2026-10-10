// Package form tiene los cuerpos JSON que reciben los endpoints de secrets.
package form

import "github.com/CPU-commits/muelle/src/internal/secrets/app/dto"

// ReplaceEnv es el cuerpo de PUT …/env: {"vars": {"K": "v"}, "secret": true}.
// secret aplica a todas las variables y es true si se omite.
type ReplaceEnv struct {
	Vars   map[string]string `json:"vars"`
	Secret *bool             `json:"secret"`
}

func (f ReplaceEnv) DTO(t dto.EnvTarget) dto.ReplaceEnv {
	return dto.ReplaceEnv{Target: t, Vars: f.Vars, Secret: f.Secret}
}

// PatchEnv es el cuerpo de PATCH …/env: {"set": {"K": "v"}, "unset": ["K2"], "secret": true}.
// secret aplica a las variables de set y es true si se omite.
type PatchEnv struct {
	Set    map[string]string `json:"set"`
	Unset  []string          `json:"unset"`
	Secret *bool             `json:"secret"`
}

func (f PatchEnv) DTO(t dto.EnvTarget) dto.PatchEnv {
	return dto.PatchEnv{Target: t, Set: f.Set, Unset: f.Unset, Secret: f.Secret}
}
