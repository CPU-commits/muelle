package entity

import "errors"

var (
	ErrUnauthenticated = errors.New("inicia sesión")
	ErrForbidden       = errors.New("no tienes permiso para esta operación")

	// ErrDockerUnavailable: el daemon de Docker no responde (§13 del spec). Lo
	// envuelve el cliente de shared/infra/out/docker y cmd/http lo traduce a
	// 503 docker_unavailable para todos los módulos.
	ErrDockerUnavailable = errors.New("Docker no responde")
)
