package dto

import "time"

// CreateTokenInput pide un token de API con alcance de proyecto (Stage
// vacío) o de proyecto + stage. ExpiresInDays 0 es que no vence.
type CreateTokenInput struct {
	Name          string
	Project       string
	Stage         string
	ExpiresInDays int
	CreatedBy     int64
}

type TokenOutput struct {
	ID         int64
	Name       string
	Project    string
	Stage      string // vacío: todo el proyecto
	CreatedAt  time.Time
	LastUsedAt *time.Time
	ExpiresAt  *time.Time
}

// CreatedTokenOutput es un token recién creado. Plain va en claro: se
// muestra una sola vez.
type CreatedTokenOutput struct {
	TokenOutput
	Plain string
}
