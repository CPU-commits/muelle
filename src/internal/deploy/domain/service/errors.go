package service

import (
	"errors"
	"fmt"
)

// RenderError es un error de render (leer la configuración o armar el compose
// final, §6): el deploy falla con "render: <Msg>" y Swarm no se toca.
//
// Msg puede traer valores de variables (compose-go pone el valor interpolado
// en sus errores de conversión): quien lo escribe en el log o en
// deploys.error lo enmascara antes (ErrorLine, FormatLine), y nunca va crudo
// a slog (decisión 18 del plan).
type RenderError struct{ Msg string }

func (e *RenderError) Error() string { return "render: " + e.Msg }

func renderErrorf(format string, args ...any) error {
	return &RenderError{Msg: fmt.Sprintf(format, args...)}
}

// Los errores de los casos de uso de deploy. RegisterErrors (infra/in/http)
// les da su status y su código.
var (
	// ErrNoPreviousDeploy: un rollback sin deploy actual o sin un succeeded
	// anterior a él (contrato §2.7): 409 no_previous_deploy.
	ErrNoPreviousDeploy = errors.New("no hay un deploy exitoso anterior al actual")
	// ErrAppRemoving: un deploy o un rollback de una app que se está borrando
	// (contrato §1.1 y §2.2): 409 conflict.
	ErrAppRemoving = errors.New("la app se está borrando")
	// ErrOffsetBeyond: el offset del log pasa su largo (contrato §2.5). Lo
	// devuelve DeployRepository.ReadLog; el caso de uso lo traduce a un 422
	// en offset.
	ErrOffsetBeyond = errors.New("el offset pasa el largo del log")
	// ErrWorkerBusy: RemoveApp no pudo detener el worker de la app en 15 s
	// (contrato §1.1): la app no se borra, 409 conflict, y se reintenta.
	ErrWorkerBusy = errors.New("un deploy de la app todavía no se detiene; vuelve a intentar en unos segundos")
)

// Los errores de los puertos de Docker (Images y Cluster).
var (
	// ErrImageNotFound: la imagen o el tag no existen en el registry.
	ErrImageNotFound = errors.New("la imagen no existe en el registry")
	// ErrImageDenied: el registry no da acceso a la imagen con esa credencial
	// (o sin credencial).
	ErrImageDenied = errors.New("el registry no da acceso a la imagen")
	// ErrSwarmNotFound: el servicio o la tarea no están en Swarm.
	ErrSwarmNotFound = errors.New("no está en Swarm")
)

// ErrAppRemoved es la causa con que RemoveApp cancela el worker de una app
// (context.Cause): el deploy en curso cierra failed con "la app se borró
// durante el deploy". Una cancelación con otra causa es el apagado, que deja
// el deploy running (contrato §1.2).
var ErrAppRemoved = errors.New("la app se borró durante el deploy")
