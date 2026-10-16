// Package dto tiene las entradas y las salidas de los casos de uso de ingress.
package dto

// StageRef identifica un stage por sus slugs.
type StageRef struct {
	Project string
	Stage   string
}
