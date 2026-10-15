package usecases

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	depsqlite "github.com/CPU-commits/muelle/src/internal/deploy/infra/out/repository/sqlite"
	outservice "github.com/CPU-commits/muelle/src/internal/deploy/infra/out/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

var t0 = time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC)

// infraCompose es el compose de la app infra: dos servicios.
const infraCompose = `services:
  web:
    image: ghcr.io/x/web:${TAG}
  worker:
    image: ghcr.io/x/worker:1
`

// fixture tiene la base real (migrada), los repositorios de deploy, el
// cifrado real, un reloj manual, la auditoría en la base, el Hub y una cola
// falsa, sobre auditfy/staging con dos apps: api (formulario) e infra (compose).
type fixture struct {
	db      *sqlite.DB
	clock   *clock.Manual
	cipher  *crypto.Cipher
	deploys *depsqlite.DeployRepository
	regs    *depsqlite.RegistryRepository
	apps    *outservice.CatalogApps
	hub     *Hub
	queue   *fakeQueue
	audit   *sqlite.AuditRecorder
	apiID   int64
	infraID int64
	otherID int64 // auditfy/staging/web, otra app de formulario
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := sqlitetest.New(t)
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	clk := clock.NewManual(t0)
	f := &fixture{
		db: db, clock: clk, cipher: c,
		deploys: depsqlite.NewDeployRepository(db),
		regs:    depsqlite.NewRegistryRepository(db, c),
		apps:    outservice.NewCatalogApps(db),
		hub:     NewHub(),
		queue:   newFakeQueue(),
		audit:   sqlite.NewAuditRecorder(db, clk, slog.New(slog.NewTextHandler(t.Output(), nil))),
	}
	exec(t, db, `INSERT INTO projects (slug, name, created_at) VALUES ('auditfy', 'Auditfy', 0)`)
	exec(t, db, `INSERT INTO stages (project_id, slug, created_at) SELECT id, 'staging', 0 FROM projects`)
	f.apiID = exec(t, db, `INSERT INTO apps (stage_id, slug, kind, spec, config_changed_at, created_at)
		SELECT id, 'api', 'service', '{"image":"ghcr.io/x/api:1","port":3000}', 0, 0 FROM stages`)
	f.infraID = exec(t, db, `INSERT INTO apps (stage_id, slug, kind, compose_yaml, config_changed_at, created_at)
		SELECT id, 'infra', 'compose', ?, 0, 0 FROM stages`, infraCompose)
	f.otherID = exec(t, db, `INSERT INTO apps (stage_id, slug, kind, spec, config_changed_at, created_at)
		SELECT id, 'web', 'service', '{"image":"nginx","port":80}', 0, 0 FROM stages`)
	return f
}

func exec(t *testing.T, db *sqlite.DB, query string, args ...any) int64 {
	t.Helper()
	ctx := context.Background()
	res, err := db.Conn(ctx).ExecContext(ctx, query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func ptr[T any](v T) *T { return &v }

// fakeQueue es la cola de T4 para los tests: anota los Wake y deja de aceptar
// las apps que se marcan como borrándose.
type fakeQueue struct {
	mu       sync.Mutex
	woken    []int64
	removing map[int64]bool
}

func newFakeQueue() *fakeQueue { return &fakeQueue{removing: map[int64]bool{}} }

func (q *fakeQueue) Wake(appID int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.woken = append(q.woken, appID)
}

func (q *fakeQueue) Accepting(appID int64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return !q.removing[appID]
}

func (q *fakeQueue) setRemoving(appID int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.removing[appID] = true
}

func (q *fakeQueue) wakes() []int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]int64(nil), q.woken...)
}

// validationMessage es el texto de un *validate.Error ("campo: mensaje"), o
// falla si err no lo es.
func validationMessage(t *testing.T, err error) string {
	t.Helper()
	ve, ok := errors.AsType[*validate.Error](err)
	if !ok {
		t.Fatalf("esperaba un error de validación, obtuvo %v", err)
	}
	return ve.Error()
}

// auditEvent es un evento de la tabla audit_events.
type auditEvent struct {
	ActorType string
	ActorID   *int64
	Action    string
	Target    string
	Details   string
}

func auditEvents(t *testing.T, db *sqlite.DB) []auditEvent {
	t.Helper()
	ctx := context.Background()
	rows, err := db.Conn(ctx).QueryContext(ctx, `SELECT actor_type, actor_id, action, target, details FROM audit_events ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []auditEvent
	for rows.Next() {
		var e auditEvent
		if err := rows.Scan(&e.ActorType, &e.ActorID, &e.Action, &e.Target, &e.Details); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	return out
}
