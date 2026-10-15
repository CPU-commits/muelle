package service

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

// La contraseña de un registry (descifrada) se usa en cada recreación de
// muelle-caddy, para bajar la imagen. Como el token de ProxySettings, nunca
// sale en un %v, un log ni un JSON: las cuatro vistas de Credential la
// muestran como "***". Así tampoco sale dentro de un CaddyWant: slog lo
// serializa con encoding/json (que llama a MarshalJSON del puntero), y fmt
// imprime el puntero anidado como una dirección.

// redacted es c con la contraseña como "***" (si hay).
func (c Credential) redacted() Credential {
	if c.Password != "" {
		c.Password = "***"
	}
	return c
}

// plainCredential es Credential sin métodos: String y MarshalJSON la usan
// para no volver a llamarse.
type plainCredential Credential

// String oculta la contraseña.
func (c Credential) String() string { return fmt.Sprintf("%+v", plainCredential(c.redacted())) }

// GoString es String para %#v.
func (c Credential) GoString() string { return c.String() }

// LogValue es String para slog: el JSONHandler serializaría el struct.
func (c Credential) LogValue() slog.Value { return slog.StringValue(c.String()) }

// MarshalJSON es el struct con la contraseña como "***", para una Credential
// dentro de otro valor que se serializa (un CaddyWant en un log). Nada se
// guarda ni se responde así.
func (c Credential) MarshalJSON() ([]byte, error) {
	return json.Marshal(plainCredential(c.redacted()))
}
