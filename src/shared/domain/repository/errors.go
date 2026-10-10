// Package repository tiene los errores y contratos comunes a todos los repositorios.
package repository

import "errors"

var (
	ErrNotFound = errors.New("no existe")
	ErrConflict = errors.New("ya existe")
	ErrNotEmpty = errors.New("primero hay que borrar lo que contiene")
)
