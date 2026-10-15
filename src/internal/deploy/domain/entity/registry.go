package entity

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

// Registry es la credencial de un registry privado, una por host (contrato
// §2.10). Las reglas del host, el usuario y la contraseña están en
// shared/domain/validate (RegistryHost, RegistryUsername, RegistryPassword).
//
// Password va en claro en memoria (se descifra para pasarla a Docker):
// String, GoString, LogValue y MarshalJSON la ocultan, así no sale en un %v,
// en un log de slog ni en un JSON, tampoco dentro de otro valor. Las
// respuestas de la API van por los DTOs, que no la llevan.
type Registry struct {
	ID        int64
	Host      string
	Username  string
	Password  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// PasswordAAD liga la contraseña cifrada a su fila (contrato §1.3, la lista
// de shared/infra/out/crypto): registries:password:<id>.
func PasswordAAD(id int64) string { return "registries:password:" + strconv.FormatInt(id, 10) }

// redacted es r con la contraseña como "***" (si hay).
func (r Registry) redacted() Registry {
	if r.Password != "" {
		r.Password = "***"
	}
	return r
}

// plainRegistry es Registry sin métodos: String y MarshalJSON lo usan para no
// volver a llamarse.
type plainRegistry Registry

// String oculta la contraseña.
func (r Registry) String() string { return fmt.Sprintf("%+v", plainRegistry(r.redacted())) }

// GoString es String para %#v.
func (r Registry) GoString() string { return r.String() }

// LogValue es String para slog: el JSONHandler serializaría el struct.
func (r Registry) LogValue() slog.Value { return slog.StringValue(r.String()) }

// MarshalJSON es el struct con la contraseña como "***". slog solo llama a
// LogValue en el valor de primer nivel: un struct que lleva un Registry
// adentro lo serializa el JSONHandler con encoding/json.
func (r Registry) MarshalJSON() ([]byte, error) { return json.Marshal(plainRegistry(r.redacted())) }
