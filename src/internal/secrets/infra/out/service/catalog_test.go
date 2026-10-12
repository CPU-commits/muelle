package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/secrets/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/secrets/seedtest"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

func TestOwnerResolver(t *testing.T) {
	db := sqlitetest.New(t)
	r := NewOwnerResolver(db)
	ctx := context.Background()
	stageID, appID := seedtest.Auditfy(t, db)
	otherProject := seedtest.Project(t, db, "otro")
	otherStage := seedtest.Stage(t, db, otherProject, "staging")
	otherApp := seedtest.App(t, db, otherStage, "api")

	if o, err := r.Stage(ctx, "auditfy", "staging"); err != nil || o != entity.StageOwner(stageID) {
		t.Fatalf("stage: %+v %v", o, err)
	}
	if o, err := r.App(ctx, "auditfy", "staging", "api"); err != nil || o != entity.AppOwner(appID) {
		t.Fatalf("app: %+v %v", o, err)
	}
	// El mismo stage y app en otro proyecto son otros dueños.
	if o, err := r.Stage(ctx, "otro", "staging"); err != nil || o != entity.StageOwner(otherStage) {
		t.Fatalf("stage de otro proyecto: %+v %v", o, err)
	}
	if o, err := r.App(ctx, "otro", "staging", "api"); err != nil || o != entity.AppOwner(otherApp) {
		t.Fatalf("app de otro proyecto: %+v %v", o, err)
	}

	// worker solo existe en otro/staging, y production solo en otro: los
	// slugs se buscan bajo su proyecto y su stage, no en cualquier lado.
	seedtest.App(t, db, otherStage, "worker")
	seedtest.Stage(t, db, otherProject, "production")

	notFound := []struct {
		name string
		fn   func() (entity.Owner, error)
	}{
		{"app que solo existe en otro proyecto", func() (entity.Owner, error) { return r.App(ctx, "auditfy", "staging", "worker") }},
		{"stage que solo existe en otro proyecto", func() (entity.Owner, error) { return r.Stage(ctx, "auditfy", "production") }},
		{"app de otro stage", func() (entity.Owner, error) { return r.App(ctx, "otro", "production", "api") }},
		{"proyecto inexistente", func() (entity.Owner, error) { return r.Stage(ctx, "nope", "staging") }},
		{"stage inexistente", func() (entity.Owner, error) { return r.Stage(ctx, "auditfy", "nope") }},
		{"app inexistente", func() (entity.Owner, error) { return r.App(ctx, "auditfy", "staging", "nope") }},
		{"app en un stage inexistente", func() (entity.Owner, error) { return r.App(ctx, "auditfy", "nope", "api") }},
		{"app en un proyecto inexistente", func() (entity.Owner, error) { return r.App(ctx, "nope", "staging", "api") }},
	}
	for _, c := range notFound {
		if o, err := c.fn(); !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("%s: %+v %v", c.name, o, err)
		}
	}
}

func TestOwnerResolverStageOfApp(t *testing.T) {
	db := sqlitetest.New(t)
	r := NewOwnerResolver(db)
	ctx := context.Background()
	stageID, appID := seedtest.Auditfy(t, db)
	production := seedtest.Stage(t, db, seedtest.ProjectID(t, db, "auditfy"), "production")
	prodApp := seedtest.App(t, db, production, "api")

	if o, err := r.StageOfApp(ctx, appID); err != nil || o != entity.StageOwner(stageID) {
		t.Fatalf("staging/api: %+v %v", o, err)
	}
	if o, err := r.StageOfApp(ctx, prodApp); err != nil || o != entity.StageOwner(production) {
		t.Fatalf("production/api: %+v %v", o, err)
	}
	if o, err := r.StageOfApp(ctx, 999); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("app inexistente: %+v %v", o, err)
	}
}

// Portado de TestEnvMarksAppsPending (Tarea 10 ref).
func TestChangeMarker(t *testing.T) {
	db := sqlitetest.New(t)
	m := NewChangeMarker(db)
	ctx := context.Background()
	stageID, api := seedtest.Auditfy(t, db)
	worker := seedtest.App(t, db, stageID, "worker")
	otherApp := seedtest.App(t, db, seedtest.Stage(t, db, seedtest.Project(t, db, "otro"), "staging"), "api")

	t1 := seedtest.T0.Add(time.Minute)
	if err := m.Mark(ctx, entity.AppOwner(api), t1); err != nil {
		t.Fatal(err)
	}
	if !seedtest.ConfigChangedAt(t, db, api).Equal(t1) || !seedtest.ConfigChangedAt(t, db, worker).Equal(seedtest.T0) {
		t.Fatal("cambiar variables de una app solo marca esa app")
	}

	t2 := t1.Add(time.Minute)
	if err := m.Mark(ctx, entity.StageOwner(stageID), t2); err != nil {
		t.Fatal(err)
	}
	if !seedtest.ConfigChangedAt(t, db, api).Equal(t2) || !seedtest.ConfigChangedAt(t, db, worker).Equal(t2) {
		t.Fatal("cambiar variables del stage marca todas sus apps")
	}
	if !seedtest.ConfigChangedAt(t, db, otherApp).Equal(seedtest.T0) {
		t.Fatal("cambiar variables de un stage no marca las apps de otro")
	}

	if err := m.Mark(ctx, entity.AppOwner(999), t2); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("app inexistente: got %v", err)
	}
	// Un stage sin apps no tiene nada que marcar.
	empty := seedtest.Stage(t, db, seedtest.Project(t, db, "vacio"), "staging")
	if err := m.Mark(ctx, entity.StageOwner(empty), t2); err != nil {
		t.Errorf("stage sin apps: got %v", err)
	}
	if err := m.Mark(ctx, entity.Owner{}, t2); !errors.Is(err, entity.ErrInvalidOwner) {
		t.Error("Mark con un dueño vacío debería fallar")
	}
}

// Mark participa en el InTx del caso de uso.
func TestChangeMarkerJoinsTransaction(t *testing.T) {
	db := sqlitetest.New(t)
	m := NewChangeMarker(db)
	ctx := context.Background()
	_, api := seedtest.Auditfy(t, db)

	boom := errors.New("boom")
	err := db.InTx(ctx, func(ctx context.Context) error {
		if err := m.Mark(ctx, entity.AppOwner(api), seedtest.T0.Add(time.Hour)); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) || !seedtest.ConfigChangedAt(t, db, api).Equal(seedtest.T0) {
		t.Fatalf("el rollback debía deshacer la marca: err=%v", err)
	}
}
