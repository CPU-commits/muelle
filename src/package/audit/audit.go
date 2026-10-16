// Package audit define el registro de auditoría que usan todos los módulos.
package audit

import "context"

// Recorder guarda un evento de auditoría. El actor sale del Principal del ctx
// (shared/domain/entity.PrincipalFrom); sin Principal, el actor es "system".
// Su nombre (Principal.ActorName: el username o el nombre del token) se copia
// en el evento al guardarlo, así quien llama no lo pasa: le basta un ctx con
// el Principal de la petición.
// details se guarda como JSON y NUNCA debe incluir valores secretos.
// Un fallo al guardar se registra en el log y no interrumpe la operación.
type Recorder interface {
	Record(ctx context.Context, action, target string, details any)
}
