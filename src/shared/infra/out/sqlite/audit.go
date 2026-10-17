package sqlite

import (
	"context"
	"database/sql"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"log/slog"

	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// AuditRecorder implementa audit.Recorder sobre la tabla audit_events.
type AuditRecorder struct {
	db    *DB
	clock clock.Clock
	log   *slog.Logger
}

var _ audit.Recorder = (*AuditRecorder)(nil)

func NewAuditRecorder(db *DB, c clock.Clock, log *slog.Logger) *AuditRecorder {
	if log == nil {
		log = slog.Default()
	}
	return &AuditRecorder{db: db, clock: c, log: log}
}

// Record guarda el evento con Conn(ctx), así que dentro de un InTx que hace
// rollback el evento tampoco queda. El nombre del actor se copia del Principal
// (Principal.ActorName): el evento lo conserva aunque se borre el usuario o el
// token. details se guarda como JSON con las claves de los map ordenadas; nil
// (o un valor que da null) se guarda como {}. Los bytes de UTF-8 inválido en
// un string se guardan como U+FFFD.
func (r *AuditRecorder) Record(ctx context.Context, action, target string, details any) {
	actorType, actorID, actorName := entity.ActorSystem, (*int64)(nil), entity.ActorSystemName
	if p, ok := entity.PrincipalFrom(ctx); ok {
		actorType, actorID = p.Actor()
		actorName = p.ActorName()
	}

	raw := "{}"
	if details != nil {
		b, err := jsonv2.Marshal(details, jsonv2.Deterministic(true), jsontext.AllowInvalidUTF8(true))
		switch {
		case err != nil:
			r.log.Error("auditoría: details no es JSON válido", "action", action, "err", err)
		case string(b) != "null":
			raw = string(b)
		}
	}

	var id sql.NullInt64
	if actorID != nil {
		id = sql.NullInt64{Int64: *actorID, Valid: true}
	}
	// Sin cancelación: si la operación ya se hizo, su evento se guarda aunque
	// el cliente corte la conexión. El ctx conserva la transacción en curso.
	ctx = context.WithoutCancel(ctx)
	_, err := r.db.Conn(ctx).ExecContext(ctx,
		`INSERT INTO audit_events (actor_type, actor_id, actor_name, action, target, details, at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		string(actorType), id, actorName, action, target, raw, Millis(r.clock.Now()))
	if err != nil {
		r.log.Error("auditoría", "action", action, "err", err)
	}
}
