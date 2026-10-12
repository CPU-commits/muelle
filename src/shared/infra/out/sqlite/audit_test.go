package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

var _ audit.Recorder = (*AuditRecorder)(nil)

type auditRow struct {
	ActorType string
	ActorID   sql.NullInt64
	Action    string
	Target    string
	Details   string
	At        int64
}

func lastAudit(t *testing.T, d *DB) auditRow {
	t.Helper()
	var r auditRow
	err := d.Conn(context.Background()).QueryRowContext(context.Background(),
		`SELECT actor_type, actor_id, action, target, details, at FROM audit_events ORDER BY id DESC LIMIT 1`).
		Scan(&r.ActorType, &r.ActorID, &r.Action, &r.Target, &r.Details, &r.At)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func countAudit(t *testing.T, d *DB) int {
	t.Helper()
	var n int
	if err := d.Conn(context.Background()).QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM audit_events`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func newTestRecorder(t *testing.T) (*DB, *AuditRecorder, *clock.Manual) {
	t.Helper()
	d := newTestDB(t)
	c := clock.NewManual(time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC))
	return d, NewAuditRecorder(d, c, slog.New(slog.NewTextHandler(io.Discard, nil))), c
}

func TestAuditActors(t *testing.T) {
	d, rec, _ := newTestRecorder(t)
	ctx := context.Background()

	rec.Record(ctx, "user.create", "user:ana", nil)
	if r := lastAudit(t, d); r.ActorType != "system" || r.ActorID.Valid || r.Action != "user.create" || r.Target != "user:ana" {
		t.Fatalf("sin Principal: %+v", r)
	}

	userCtx := entity.WithPrincipal(ctx, entity.Principal{UserID: 4, Username: "ana"})
	rec.Record(userCtx, "project.create", "project:auditfy", nil)
	if r := lastAudit(t, d); r.ActorType != "user" || !r.ActorID.Valid || r.ActorID.Int64 != 4 {
		t.Fatalf("usuario: %+v", r)
	}

	tokenCtx := entity.WithPrincipal(ctx, entity.Principal{Token: &entity.TokenScope{ID: 9, ProjectSlug: "auditfy"}})
	rec.Record(tokenCtx, "env.replace", "stage:auditfy/staging", nil)
	if r := lastAudit(t, d); r.ActorType != "token" || !r.ActorID.Valid || r.ActorID.Int64 != 9 {
		t.Fatalf("token: %+v", r)
	}
}

func TestAuditDetails(t *testing.T) {
	d, rec, _ := newTestRecorder(t)
	ctx := context.Background()

	rec.Record(ctx, "env.replace", "app:a/s/x", map[string][]string{"keys": {"A"}})
	if r := lastAudit(t, d); r.Details != `{"keys":["A"]}` {
		t.Fatalf("details=%s", r.Details)
	}

	rec.Record(ctx, "project.delete", "project:a", nil)
	if r := lastAudit(t, d); r.Details != `{}` {
		t.Fatalf("details nil=%s", r.Details)
	}

	var nilMap map[string]string
	rec.Record(ctx, "project.delete", "project:a", nilMap)
	if r := lastAudit(t, d); r.Details != `{}` {
		t.Fatalf("details de un map nil=%s", r.Details)
	}

	// Las claves de un map salen ordenadas, para que el JSON sea estable.
	rec.Record(ctx, "env.patch", "app:a/s/x", map[string][]string{
		"unset": {"B"}, "set": {"A"}, "c": {}, "b": {}, "a": {},
	})
	if r := lastAudit(t, d); r.Details != `{"a":[],"b":[],"c":[],"set":["A"],"unset":["B"]}` {
		t.Fatalf("details=%s", r.Details)
	}
}

// Un string con UTF-8 inválido (ej. un nombre que vino de Docker) no se pierde
// como {}: los bytes inválidos se guardan como U+FFFD.
func TestAuditDetailsInvalidUTF8(t *testing.T) {
	d, rec, _ := newTestRecorder(t)
	rec.Record(context.Background(), "app.update", "app:a/s/x", map[string]string{"name": "a\xffb"})
	r := lastAudit(t, d)
	var got map[string]string
	if err := json.Unmarshal([]byte(r.Details), &got); err != nil {
		t.Fatalf("details=%q no es JSON: %v", r.Details, err)
	}
	if got["name"] != "a\uFFFDb" {
		t.Fatalf("details=%s, esperaba name=%q", r.Details, "a\uFFFDb")
	}
}

func TestAuditUsesClock(t *testing.T) {
	d, rec, c := newTestRecorder(t)
	c.Advance(90 * time.Minute)
	rec.Record(context.Background(), "x", "y", nil)
	if r := lastAudit(t, d); r.At != c.Now().UnixMilli() {
		t.Fatalf("at=%d, esperaba %d", r.At, c.Now().UnixMilli())
	}
}

func TestAuditJoinsTransaction(t *testing.T) {
	d, rec, _ := newTestRecorder(t)
	boom := errors.New("boom")
	err := d.InTx(context.Background(), func(ctx context.Context) error {
		rec.Record(ctx, "project.create", "project:a", nil)
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if n := countAudit(t, d); n != 0 {
		t.Fatalf("el rollback debió deshacer el evento, hay %d", n)
	}

	err = d.InTx(context.Background(), func(ctx context.Context) error {
		rec.Record(ctx, "project.create", "project:a", nil)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := countAudit(t, d); n != 1 {
		t.Fatalf("con commit debe quedar el evento, hay %d", n)
	}
}

func TestAuditFailureIsLogged(t *testing.T) {
	d := newTestDB(t)
	var buf bytes.Buffer
	rec := NewAuditRecorder(d, clock.NewManual(time.Unix(0, 0)), slog.New(slog.NewTextHandler(&buf, nil)))
	d.Close()
	rec.Record(context.Background(), "project.create", "project:a", nil) // no hace panic
	if out := buf.String(); !strings.Contains(out, "auditoría") || !strings.Contains(out, "action=project.create") {
		t.Fatalf("log=%q", out)
	}
}
