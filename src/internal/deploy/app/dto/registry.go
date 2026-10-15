package dto

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

// RegistryInput son las credenciales de un registry como llegan a un caso de
// uso: el cuerpo de POST y PATCH /registries (en el PATCH, Host sale de la
// ruta) y el registry inicial de install (InitRegistry).
//
// La contraseña va en claro: String, GoString, LogValue y MarshalJSON la
// ocultan, así no sale en un %v, en un log de slog ni en un JSON.
type RegistryInput struct {
	Host     string
	Username string
	Password string
}

// InitRegistry es el registry inicial de install (MUELLE_INIT_REGISTRY_FILE,
// contrato §2.10). deploy lo guarda solo en el primer arranque de la base.
type InitRegistry = RegistryInput

func (r RegistryInput) redacted() RegistryInput {
	if r.Password != "" {
		r.Password = "***"
	}
	return r
}

// plainRegistryInput es RegistryInput sin métodos: no vuelve a llamarlos.
type plainRegistryInput RegistryInput

// String es {Host:ghcr.io Username:bot Password:***}.
func (r RegistryInput) String() string { return fmt.Sprintf("%+v", plainRegistryInput(r.redacted())) }

// GoString es String para %#v.
func (r RegistryInput) GoString() string { return r.String() }

// LogValue es String para slog: el JSONHandler serializaría el struct.
func (r RegistryInput) LogValue() slog.Value { return slog.StringValue(r.String()) }

// MarshalJSON es el struct con la contraseña como "***" ("" si está vacía).
func (r RegistryInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(plainRegistryInput(r.redacted()))
}
