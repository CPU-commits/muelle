package http

import (
	"net/http"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
)

// StatusRes es GET …/status (contrato §2.8).
type StatusRes struct {
	Services []ServiceStatusRes `json:"services"`
}

type ServiceStatusRes struct {
	Name             string          `json:"name"`
	Image            string          `json:"image"`
	ReplicasDesired  uint64          `json:"replicas_desired"`
	ReplicasRunning  uint64          `json:"replicas_running"`
	CPUPercent       *float64        `json:"cpu_percent"`
	MemoryBytes      *uint64         `json:"memory_bytes"`
	MemoryLimitBytes *int64          `json:"memory_limit_bytes"`
	UpdateState      *string         `json:"update_state"`
	Tasks            []TaskStatusRes `json:"tasks"`
}

type TaskStatusRes struct {
	ID           string     `json:"id"`
	Slot         *int       `json:"slot"`
	State        string     `json:"state"`
	DesiredState string     `json:"desired_state"`
	Health       *string    `json:"health"`
	Node         *string    `json:"node"`
	Since        *time.Time `json:"since"`
	Error        *string    `json:"error"`
	CPUPercent   *float64   `json:"cpu_percent"`
	MemoryBytes  *uint64    `json:"memory_bytes"`
}

func newStatusRes(st dto.AppStatus) StatusRes {
	out := StatusRes{Services: make([]ServiceStatusRes, 0, len(st.Services))}
	for _, s := range st.Services {
		svc := ServiceStatusRes{
			Name: s.Name, Image: s.Image, ReplicasDesired: s.ReplicasDesired, ReplicasRunning: s.ReplicasRunning,
			CPUPercent: s.CPUPercent, MemoryBytes: s.MemoryBytes, MemoryLimitBytes: s.MemoryLimitBytes,
			UpdateState: s.UpdateState, Tasks: make([]TaskStatusRes, 0, len(s.Tasks)),
		}
		for _, t := range s.Tasks {
			svc.Tasks = append(svc.Tasks, TaskStatusRes{
				ID: t.ID, Slot: t.Slot, State: t.State, DesiredState: t.DesiredState, Health: t.Health,
				Node: t.Node, Since: millisPtr(t.Since), Error: t.Error, CPUPercent: t.CPUPercent, MemoryBytes: t.MemoryBytes,
			})
		}
		out.Services = append(out.Services, svc)
	}
	return out
}

// status es GET …/apps/{a}/status (contrato §2.8).
func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	st, err := h.uc.GetStatus.Execute(r.Context(), appRef(r))
	if err != nil {
		h.k.Fail(w, r, err)
		return
	}
	utils.WriteJSON(w, http.StatusOK, newStatusRes(st))
}
