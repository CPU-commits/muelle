package service

import "fmt"

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
