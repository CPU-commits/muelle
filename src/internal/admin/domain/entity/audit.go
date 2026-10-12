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
	Action    string // ej. "project.create"
	Target    string // ej. "project:auditfy"
	Details   jsontext.Value
	At        time.Time
}
