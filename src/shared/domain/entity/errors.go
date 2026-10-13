package entity

import "errors"

var (
	ErrUnauthenticated = errors.New("inicia sesión")
	ErrForbidden       = errors.New("no tienes permiso para esta operación")

	// ErrDockerUnavailable: el daemon de Docker no responde (§13 del spec). Lo
	// envuelve el cliente de shared/infra/out/docker y cmd/http lo traduce a
	// 503 docker_unavailable para todos los módulos.
	ErrDockerUnavailable = errors.New("Docker no responde")

	// ErrNotImplemented lo devuelven los métodos de los módulos que todavía
	// son esqueleto (deploy en la Fase 2, ingress en la Fase 3). cmd/http lo
	// responde como 501 not_implemented. Una ruta que todavía no existe no
	// llega acá: responde 404 no_route.
	ErrNotImplemented = errors.New("todavía no está implementado")
)
