// Package dto tiene las entradas y salidas de los casos de uso de deploy.
package dto

// AppRef identifica una app por sus slugs.
type AppRef struct {
	Project string
	Stage   string
	App     string
}
