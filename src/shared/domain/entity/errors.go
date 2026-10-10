package entity

import "errors"

var (
	ErrUnauthenticated = errors.New("inicia sesión")
	ErrForbidden       = errors.New("no tienes permiso para esta operación")
)
