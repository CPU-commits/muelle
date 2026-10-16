package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

func TestCatalogApp(t *testing.T) {
	db := sqlitetest.New(t)
	api := ingresstest.App(t, db, "auditfy", "staging", "api", "service")
	web := ingresstest.App(t, db, "auditfy", "staging", "web", "compose")
	ingresstest.App(t, db, "auditfy", "prod", "otra", "service")
	c := NewCatalogReader(db)
	ctx := context.Background()

	for id, want := range map[int64]service.App{
		api: {ID: api, Project: "auditfy", Stage: "staging", Slug: "api", Kind: service.KindService},
		web: {ID: web, Project: "auditfy", Stage: "staging", Slug: "web", Kind: service.KindCompose},
	} {
		got, err := c.App(ctx, want.Project, want.Stage, want.Slug)
		if err != nil || got != want {
			t.Errorf("App %d: %+v %v, esperaba %+v", id, got, err, want)
		}
	}
	for _, ref := range [][3]string{
		{"auditfy", "prod", "api"},     // la app es de otro stage
		{"auditfy", "staging", "otra"}, // ídem
		{"auditfy", "staging", "nada"},
		{"nada", "staging", "api"},
	} {
		if _, err := c.App(ctx, ref[0], ref[1], ref[2]); !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("App %v: %v, esperaba ErrNotFound", ref, err)
		}
	}
}

func TestCatalogApps(t *testing.T) {
	db := sqlitetest.New(t)
	c := NewCatalogReader(db)
	ctx := context.Background()
	if got, err := c.Apps(ctx); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("sin apps: %#v %v", got, err)
	}
	api := ingresstest.App(t, db, "auditfy", "staging", "api", "service")
	web := ingresstest.App(t, db, "otro", "prod", "web", "compose")
	got, err := c.Apps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64]service.App{
		api: {ID: api, Project: "auditfy", Stage: "staging", Slug: "api", Kind: service.KindService},
		web: {ID: web, Project: "otro", Stage: "prod", Slug: "web", Kind: service.KindCompose},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Apps:\n%+v\nesperaba\n%+v", got, want)
	}
}

func TestCatalogStages(t *testing.T) {
	db := sqlitetest.New(t)
	c := NewCatalogReader(db)
	ctx := context.Background()
	if got, err := c.Stages(ctx); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("sin stages: %#v %v", got, err)
	}
	ingresstest.Stage(t, db, "zeta", "prod")
	ingresstest.Stage(t, db, "auditfy", "staging")
	ingresstest.App(t, db, "auditfy", "prod", "api", "service")
	ingresstest.Stage(t, db, "auditfy", "staging") // ya existe: no se repite
	got, err := c.Stages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []service.Stage{{Project: "auditfy", Stage: "prod"}, {Project: "auditfy", Stage: "staging"}, {Project: "zeta", Stage: "prod"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Stages: %+v, esperaba %+v", got, want)
	}
}
