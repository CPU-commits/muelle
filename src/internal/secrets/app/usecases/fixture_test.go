package usecases

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/secrets/app/dto"
	"github.com/CPU-commits/muelle/src/internal/secrets/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/secrets/domain/service"
	envsqlite "github.com/CPU-commits/muelle/src/internal/secrets/infra/out/repository/sqlite"
	outservice "github.com/CPU-commits/muelle/src/internal/secrets/infra/out/service"
	"github.com/CPU-commits/muelle/src/internal/secrets/seedtest"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

// fixture arma los casos de uso con la base real, el cifrado real y un reloj
// manual, sobre auditfy/staging/api.
type fixture struct {
	db      *sqlite.DB
	clock   *clock.Manual
	repo    *envsqlite.EnvRepository
	get     *GetEnv
	replace *ReplaceEnv
	patch   *PatchEnv
	resolve *ResolveEnv
	stageID int64
	appID   int64
}

var (
	appTarget   = dto.EnvTarget{Project: "auditfy", Stage: "staging", App: "api"}
	stageTarget = dto.EnvTarget{Project: "auditfy", Stage: "staging"}
)

func newFixture(t *testing.T) *fixture {
	return newFixtureWithMarker(t, nil)
}

// newFixtureWithMarker usa marker como ChangeMarker; con nil usa el adaptador SQL.
func newFixtureWithMarker(t *testing.T, marker service.ChangeMarker) *fixture {
	t.Helper()
	db := sqlitetest.New(t)
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	clk := clock.NewManual(seedtest.T0.Add(time.Hour))
	repo := envsqlite.NewEnvRepository(db)
	owners := outservice.NewOwnerResolver(db)
	if marker == nil {
		marker = outservice.NewChangeMarker(db)
	}
	ec := service.NewEnvCrypto(c)
	rec := sqlite.NewAuditRecorder(db, clk, slog.New(slog.NewTextHandler(t.Output(), nil)))
	stageID, appID := seedtest.Auditfy(t, db)
	return &fixture{
		db:      db,
		clock:   clk,
		repo:    repo,
		get:     NewGetEnv(owners, repo, ec),
		replace: NewReplaceEnv(db, owners, repo, marker, ec, clk, rec),
		patch:   NewPatchEnv(db, owners, repo, marker, ec, clk, rec),
		resolve: NewResolveEnv(repo, ec),
		stageID: stageID,
		appID:   appID,
	}
}

func ptr[T any](v T) *T { return &v }

func validationField(t *testing.T, err error) string {
	t.Helper()
	ve, ok := errors.AsType[*validate.Error](err)
	if !ok {
		t.Fatalf("esperaba un *validate.Error, llegó %v", err)
	}
	return ve.Field
}

// plain devuelve las variables del dueño descifradas, para comparar.
func (f *fixture) plain(t *testing.T, o entity.Owner) map[string]string {
	t.Helper()
	rows, err := f.repo.List(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	vars, err := service.NewEnvCrypto(c).Open(o, rows)
	if err != nil {
		t.Fatal(err)
	}
	return service.Merge(nil, vars)
}

type auditRow struct {
	actorType, action, target, details string
}

func (f *fixture) auditEvents(t *testing.T) []auditRow {
	t.Helper()
	ctx := context.Background()
	rows, err := f.db.Conn(ctx).QueryContext(ctx, `SELECT actor_type, action, target, details FROM audit_events ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []auditRow
	for rows.Next() {
		var r auditRow
		if err := rows.Scan(&r.actorType, &r.action, &r.target, &r.details); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

// failingMarker es un ChangeMarker (puerto hacia catalog) que siempre falla.
type failingMarker struct{ err error }

func (m failingMarker) Mark(context.Context, entity.Owner, time.Time) error { return m.err }
