// Package entity tiene las entidades del módulo admin.
package entity

import (
	"encoding/json/jsontext"
	"time"
)

// AuditEvent es un evento de auditoría ya guardado.
type AuditEvent struct {
	ID        int64
	ActorType string // "user" | "token" | "system"
	ActorID   *int64
	// ActorName es el username, el nombre del token o "system", copiado al
	// escribir el evento: sigue aunque se borre el usuario o el token. "" solo
	// en un evento de antes de la migración 0004 cuyo autor ya no estaba.
	ActorName string
	Action    string // ej. "project.create"
	Target    string // ej. "project:auditfy"
	Details   jsontext.Value
	At        time.Time
}
