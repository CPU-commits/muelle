package usecases

import (
	"context"
	"time"

	"github.com/CPU-commits/muelle/src/internal/admin/app/dto"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/repository"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

const (
	defaultAuditLimit = 100
	maxAuditLimit     = 500

	// AuditRetentionDays es cuánto se conserva la auditoría (§5.2 del spec).
	AuditRetentionDays = 180
)

type ListAudit struct {
	audit repository.AuditRepository
}

func NewListAudit(audit repository.AuditRepository) *ListAudit { return &ListAudit{audit: audit} }

// Execute devuelve los eventos más nuevos primero, paginados por id (Before).
func (u *ListAudit) Execute(ctx context.Context, in dto.ListAuditInput) ([]dto.AuditEventOutput, error) {
	limit := defaultAuditLimit
	if in.Limit != nil {
		if *in.Limit < 1 || *in.Limit > maxAuditLimit {
			return nil, validate.Errorf("limit", "debe estar entre 1 y %d", maxAuditLimit)
		}
		limit = *in.Limit
	}
	var before int64
	if in.Before != nil {
		if *in.Before < 1 {
			return nil, validate.Errorf("before", "debe ser un id válido")
		}
		before = *in.Before
	}
	events, err := u.audit.List(ctx, before, limit)
	if err != nil {
		return nil, err
	}
	out := make([]dto.AuditEventOutput, 0, len(events))
	for _, e := range events {
		out = append(out, dto.AuditEventOutput{
			ID: e.ID, ActorType: e.ActorType, ActorID: e.ActorID, ActorName: e.ActorName,
			Action: e.Action, Target: e.Target, Details: e.Details, At: e.At,
		})
	}
	return out, nil
}

type PurgeAudit struct {
	audit repository.AuditRepository
	clock clock.Clock
}

func NewPurgeAudit(audit repository.AuditRepository, c clock.Clock) *PurgeAudit {
	return &PurgeAudit{audit: audit, clock: c}
}

// Execute borra los eventos de más de 180 días y devuelve cuántos.
func (u *PurgeAudit) Execute(ctx context.Context) (int64, error) {
	return u.audit.DeleteBefore(ctx, u.clock.Now().Add(-AuditRetentionDays*24*time.Hour))
}
