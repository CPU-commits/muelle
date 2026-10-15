// Package form tiene los cuerpos JSON que reciben los endpoints de deploy.
package form

import "github.com/CPU-commits/muelle/src/internal/deploy/app/dto"

// CreateRegistry es el cuerpo de POST /registries (contrato §2.10).
type CreateRegistry struct {
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (f CreateRegistry) DTO() dto.RegistryInput {
	return dto.RegistryInput{Host: f.Host, Username: f.Username, Password: f.Password}
}

// UpdateRegistry es el cuerpo de PATCH /registries/{host}: los dos campos
// son obligatorios y reemplazan las credenciales.
type UpdateRegistry struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (f UpdateRegistry) DTO(host string) dto.RegistryInput {
	return dto.RegistryInput{Host: host, Username: f.Username, Password: f.Password}
}
