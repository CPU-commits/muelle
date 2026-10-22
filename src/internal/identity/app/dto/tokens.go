package dto

import "time"

// CreateTokenInput pide un token de API con alcance de proyecto (Stage
// vacío), de proyecto + stage, o de todos los proyectos (AllProjects, sin
// Project ni Stage y con ReadOnly). ExpiresInDays 0 es que no vence.
type CreateTokenInput struct {
	Name          string
	Project       string
	Stage         string
	ReadOnly      bool
	AllProjects   bool
	ExpiresInDays int
	CreatedBy     int64
}

type TokenOutput struct {
	ID          int64
	Name        string
	Project     string // vacío solo con AllProjects
	Stage       string // vacío: todo el proyecto
	ReadOnly    bool
	AllProjects bool
	CreatedBy   *string // username de quien lo creó; nil si ese usuario se borró
	CreatedAt   time.Time
	LastUsedAt  *time.Time
	ExpiresAt   *time.Time
}

// CreatedTokenOutput es un token recién creado. Plain va en claro: se
// muestra una sola vez.
type CreatedTokenOutput struct {
	TokenOutput
	Plain string
}
