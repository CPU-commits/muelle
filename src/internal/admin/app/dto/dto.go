// Package dto tiene los datos que entran y salen de los casos de uso de admin.
package dto

import (
	"encoding/json/jsontext"
	"time"
)

type SettingsOutput struct {
	Require2FA bool `json:"require_2fa"`
}

// PatchSettingsInput: un campo nil no se toca.
type PatchSettingsInput struct {
	Require2FA *bool `json:"require_2fa"`
}

// ListAuditInput: Limit nil = 100 y Before nil = desde el último evento.
type ListAuditInput struct {
	Limit  *int
	Before *int64
}

type AuditEventOutput struct {
	ID        int64          `json:"id"`
	ActorType string         `json:"actor_type"`
	ActorID   *int64         `json:"actor_id"`
	Action    string         `json:"action"`
	Target    string         `json:"target"`
	Details   jsontext.Value `json:"details"`
	At        time.Time      `json:"at"`
}

// BackupOutput dice qué pasó con el backup del día.
type BackupOutput struct {
	Name    string // nombre del archivo del día
	Created bool   // false si ya existía
	Removed int    // copias viejas borradas
}
