package service

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

// Credential es la credencial de un registry que deploy le pasa a Docker: al
// resolver una imagen (§7 paso 2, la de su host exacto) y a docker stack
// deploy (paso 5, las de los hosts del compose). Host es como lo da
// ImageHost ("ghcr.io", "docker.io").
//
// Password va en claro: String, GoString, LogValue y MarshalJSON la ocultan,
// así no sale en un %v, en un log de slog ni en un JSON, tampoco dentro de
// otro valor.
type Credential struct{ Host, Username, Password string }

// plainCredential es Credential sin métodos: String y MarshalJSON la usan
// para no volver a llamarse.
type plainCredential Credential

func (c Credential) redacted() plainCredential {
	if c.Password != "" {
		c.Password = "***"
	}
	return plainCredential(c)
}

// String oculta la contraseña.
func (c Credential) String() string { return fmt.Sprintf("%+v", c.redacted()) }

// GoString es String para %#v.
func (c Credential) GoString() string { return c.String() }

// LogValue es String para slog.
func (c Credential) LogValue() slog.Value { return slog.StringValue(c.String()) }

// MarshalJSON es la credencial con la contraseña como "***": slog solo llama
// a LogValue en el valor de primer nivel, y el JSONHandler serializa lo que
// va adentro de otro valor con encoding/json.
func (c Credential) MarshalJSON() ([]byte, error) { return json.Marshal(c.redacted()) }
