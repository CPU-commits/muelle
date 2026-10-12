// Package service tiene los errores del dominio del catálogo.
package service

import "errors"

// ErrKindMismatch: se intentó cambiar un campo que no corresponde al tipo de la app
// (el spec de una app compose, o el compose de una app de formulario).
var ErrKindMismatch = errors.New("el campo no corresponde al tipo de app")
