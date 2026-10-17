// Package entity tiene las entidades del módulo deploy: el deploy (contrato
// §2.1) y la credencial de un registry (contrato §2.10). Las reglas de un
// registry están en shared/domain/validate (también las usa settings.Load).
package entity

import "time"

// Status es el estado de un deploy (contrato §2.1).
type Status string

const (
	StatusQueued     Status = "queued"
	StatusRunning    Status = "running"
	StatusSucceeded  Status = "succeeded"
	StatusFailed     Status = "failed"
	StatusSuperseded Status = "superseded"
)

// Final dice si el deploy terminó: succeeded, failed o superseded.
func (s Status) Final() bool {
	return s == StatusSucceeded || s == StatusFailed || s == StatusSuperseded
}

// Valid dice si s es uno de los estados conocidos.
func (s Status) Valid() bool {
	return s == StatusQueued || s == StatusRunning || s.Final()
}

// Trigger es quién pidió el deploy: una sesión (panel), un token (api) o
// POST …/rollback (contrato §2.1).
type Trigger string

const (
	TriggerPanel    Trigger = "panel"
	TriggerAPI      Trigger = "api"
	TriggerRollback Trigger = "rollback"
)

// Valid dice si t es uno de los triggers conocidos.
func (t Trigger) Valid() bool {
	return t == TriggerPanel || t == TriggerAPI || t == TriggerRollback
}

// Los tipos de actor de un deploy (deploys.actor_type).
const (
	ActorUser  = "user"
	ActorToken = "token"
)

// Deploy es un deploy de una app (tabla deploys, contrato §2.1). El log no
// va acá: se lee y se escribe aparte, por offsets (contrato §2.5).
type Deploy struct {
	ID, AppID int64
	Status    Status
	Trigger   Trigger
	// Images: mientras está queued, los overrides (en un rollback, las del
	// destino); desde el paso 2, las refs resueltas (contrato §2.1).
	Images map[string]string
	// RequestedImages y ConfigImages: las refs pedidas y las de la
	// configuración, que el paso 2 guarda (contrato §2.2). {} hasta entonces.
	RequestedImages, ConfigImages map[string]string
	ActorType                     string // ActorUser o ActorToken
	ActorID                       *int64
	ActorName                     string
	SupersededBy                  *int64
	Error                         *string
	// RoutesWarning: el paso 7 no pudo actualizar las rutas de Caddy y
	// escribió la advertencia en el log (contrato §2.1 y §2.11).
	RoutesWarning bool
	CreatedAt     time.Time
	StartedAt     *time.Time
	FinishedAt    *time.Time
}
