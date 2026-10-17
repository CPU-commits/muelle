package http

import (
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
)

// RegistryRes es un registry en la respuesta (contrato §2.10): nunca la contraseña.
type RegistryRes struct {
	Host      string    `json:"host"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func newRegistryRes(r entity.Registry) RegistryRes {
	return RegistryRes{Host: r.Host, Username: r.Username, CreatedAt: millis(r.CreatedAt), UpdatedAt: millis(r.UpdatedAt)}
}

func newRegistriesRes(list []entity.Registry) []RegistryRes {
	out := make([]RegistryRes, 0, len(list))
	for _, r := range list {
		out = append(out, newRegistryRes(r))
	}
	return out
}

// millis es t en UTC truncada al milisegundo (§10 del diseño).
func millis(t time.Time) time.Time { return t.UTC().Truncate(time.Millisecond) }

// DeployRes es un deploy (contrato §2.1): todos los campos, con null cuando
// corresponde, e images y requested_images nunca null.
type DeployRes struct {
	ID              int64             `json:"id"`
	Status          string            `json:"status"`
	Trigger         string            `json:"trigger"`
	Images          map[string]string `json:"images"`
	RequestedImages map[string]string `json:"requested_images"`
	ActorType       string            `json:"actor_type"`
	ActorID         *int64            `json:"actor_id"`
	ActorName       string            `json:"actor_name"`
	Error           *string           `json:"error"`
	SupersededBy    *int64            `json:"superseded_by"`
	RoutesWarning   bool              `json:"routes_warning"`
	CreatedAt       time.Time         `json:"created_at"`
	StartedAt       *time.Time        `json:"started_at"`
	FinishedAt      *time.Time        `json:"finished_at"`
}

// orEmpty es m, o {} si es nil: un mapa del deploy nunca sale null.
func orEmpty(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func newDeployRes(d entity.Deploy) DeployRes {
	return DeployRes{
		ID: d.ID, Status: string(d.Status), Trigger: string(d.Trigger),
		Images: orEmpty(d.Images), RequestedImages: orEmpty(d.RequestedImages),
		ActorType: d.ActorType, ActorID: d.ActorID, ActorName: d.ActorName,
		Error: d.Error, SupersededBy: d.SupersededBy, RoutesWarning: d.RoutesWarning,
		CreatedAt: millis(d.CreatedAt), StartedAt: millisPtr(d.StartedAt), FinishedAt: millisPtr(d.FinishedAt),
	}
}

func newDeploysRes(list []entity.Deploy) []DeployRes {
	out := make([]DeployRes, 0, len(list))
	for _, d := range list {
		out = append(out, newDeployRes(d))
	}
	return out
}

func millisPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	m := millis(*t)
	return &m
}

// QueuedRes es la respuesta 202 de un deploy o un rollback (contrato §2.2).
type QueuedRes struct {
	ID        int64  `json:"id"`
	Status    string `json:"status"`
	StatusURL string `json:"status_url"`
}

func newQueuedRes(q dto.Queued) QueuedRes {
	return QueuedRes{ID: q.ID, Status: string(q.Status), StatusURL: q.StatusURL}
}

// LogRes es un pedazo del log por polling (contrato §2.5).
type LogRes struct {
	Chunk        string `json:"chunk"`
	NextOffset   int    `json:"next_offset"`
	Status       string `json:"status"`
	SupersededBy *int64 `json:"superseded_by"`
}

func newLogRes(p dto.LogPage) LogRes {
	return LogRes{Chunk: p.Chunk, NextOffset: p.NextOffset, Status: string(p.Status), SupersededBy: p.SupersededBy}
}
