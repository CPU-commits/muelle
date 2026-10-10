package usecases

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/secrets/app/dto"
	"github.com/CPU-commits/muelle/src/internal/secrets/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/secrets/seedtest"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
)

func TestPatchEnv(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	o := entity.AppOwner(f.appID)
	f.replace.Execute(ctx, dto.ReplaceEnv{Target: appTarget, Vars: map[string]string{"A": "1", "B": "2"}})

	// Agrega C, reemplaza A, borra B y una clave inexistente sin fallar.
	err := f.patch.Execute(ctx, dto.PatchEnv{Target: appTarget, Set: map[string]string{"A": "10", "C": "3"}, Unset: []string{"B", "Z"}, Secret: ptr(false)})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.plain(t, o); !maps.Equal(got, map[string]string{"A": "10", "C": "3"}) {
		t.Fatalf("got=%v", got)
	}
	vars, _ := f.get.Execute(ctx, appTarget)
	if len(vars) != 2 || vars[0].Secret || vars[1].Secret {
		t.Fatalf("secret=false aplica a las de set: %+v", vars)
	}

	// Sin secret, las de set quedan secretas; las demás no cambian.
	f.clock.Advance(time.Minute)
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: appTarget, Set: map[string]string{"D": "4"}}); err != nil {
		t.Fatal(err)
	}
	vars, _ = f.get.Execute(ctx, appTarget)
	if len(vars) != 3 || vars[0].Secret || vars[1].Secret || !vars[2].Secret {
		t.Fatalf("vars=%+v", vars)
	}
	if !vars[2].UpdatedAt.Equal(f.clock.Now()) || vars[0].UpdatedAt.Equal(f.clock.Now()) {
		t.Fatalf("updated_at solo cambia en las de set: %+v", vars)
	}
}

// Portado de TestValidation (Tarea 15 ref), parte de PATCH.
func TestPatchEnvValidation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	cases := []struct {
		name  string
		in    dto.PatchEnv
		field string
	}{
		{"set y unset a la vez", dto.PatchEnv{Set: map[string]string{"A": "1"}, Unset: []string{"A"}}, "unset"},
		{"unset inválido", dto.PatchEnv{Unset: []string{"bad-key"}}, "unset"},
		{"set inválido", dto.PatchEnv{Set: map[string]string{"1BAD": "x"}}, "set"},
		{"set con prefijo COMPOSE_", dto.PatchEnv{Set: map[string]string{"COMPOSE_FILE": "x"}}, "set"},
		{"valor grande", dto.PatchEnv{Set: map[string]string{"BIG": strings.Repeat("x", entity.MaxValueBytes+1)}}, "set"},
	}
	for _, c := range cases {
		c.in.Target = appTarget
		if err := f.patch.Execute(ctx, c.in); validationField(t, err) != c.field {
			t.Errorf("%s: %v", c.name, err)
		}
	}

	many := map[string]string{}
	for i := range entity.MaxVars {
		many[fmt.Sprintf("K%d", i)] = "x"
	}
	if err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: appTarget, Vars: many}); err != nil {
		t.Fatalf("%d variables deberían caber: %v", len(many), err)
	}
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: appTarget, Set: map[string]string{"ONE_MORE": "x"}}); validationField(t, err) != "set" {
		t.Errorf("superar MaxVars con patch: %v", err)
	}
	// Reemplazar una existente, o borrar una y agregar otra, no supera el máximo.
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: appTarget, Set: map[string]string{"K0": "y"}}); err != nil {
		t.Errorf("reemplazar una existente: %v", err)
	}
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: appTarget, Set: map[string]string{"ONE_MORE": "x"}, Unset: []string{"K1"}}); err != nil {
		t.Errorf("borrar una y agregar otra: %v", err)
	}
	if got := f.plain(t, entity.AppOwner(f.appID)); len(got) != entity.MaxVars || got["K0"] != "y" || got["ONE_MORE"] != "x" {
		t.Fatalf("quedaron %d variables", len(got))
	}
}

func TestPatchEnvNotFound(t *testing.T) {
	f := newFixture(t)
	err := f.patch.Execute(context.Background(), dto.PatchEnv{Target: dto.EnvTarget{Project: "auditfy", Stage: "nope"}, Set: map[string]string{"A": "1"}})
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

// Un PATCH sobre un stage marca todas sus apps, y solo las suyas.
func TestPatchStageMarksAllItsApps(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	worker := seedtest.App(t, f.db, f.stageID, "worker")
	production := seedtest.Stage(t, f.db, 1, "production")
	otherApp := seedtest.App(t, f.db, production, "api")

	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: stageTarget, Set: map[string]string{"NATS": "nats:4222"}, Secret: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{f.appID, worker} {
		if !seedtest.ConfigChangedAt(t, f.db, id).Equal(f.clock.Now()) {
			t.Fatalf("la app %d del stage no quedó marcada", id)
		}
	}
	if !seedtest.ConfigChangedAt(t, f.db, otherApp).Equal(seedtest.T0) {
		t.Fatal("un cambio en staging no marca las apps de production")
	}
	if got := f.plain(t, entity.StageOwner(f.stageID)); !maps.Equal(got, map[string]string{"NATS": "nats:4222"}) {
		t.Fatalf("got=%v", got)
	}
}

// Un PATCH que falla en Mark deja las variables como estaban (rollback).
func TestPatchEnvRollsBackWhenMarkFails(t *testing.T) {
	boom := errors.New("boom")
	f := newFixtureWithMarker(t, failingMarker{err: boom})
	ctx := context.Background()
	o := entity.StageOwner(f.stageID)
	sealed, _ := f.patch.crypto.Seal(o, []entity.EnvVar{{Key: "A", Value: []byte("antes"), Secret: true, UpdatedAt: seedtest.T0}})
	if err := f.repo.Replace(ctx, o, sealed); err != nil {
		t.Fatal(err)
	}

	err := f.patch.Execute(ctx, dto.PatchEnv{Target: stageTarget, Set: map[string]string{"B": "nueva"}, Unset: []string{"A"}})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	if got := f.plain(t, o); !maps.Equal(got, map[string]string{"A": "antes"}) {
		t.Fatalf("el rollback debía dejar las variables como estaban: %v", got)
	}
	if events := f.auditEvents(t); len(events) != 0 {
		t.Fatalf("un PATCH revertido no se audita: %+v", events)
	}
}

func TestPatchEnvAuditsKeysOnly(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	err := f.patch.Execute(ctx, dto.PatchEnv{Target: appTarget, Set: map[string]string{"LOG_LEVEL": "debug", "API_KEY": "s3cr3t"}, Unset: []string{"Z", "JWT_KEY", "Z"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: stageTarget}); err != nil {
		t.Fatal(err)
	}
	events := f.auditEvents(t)
	want := []auditRow{
		{"system", "env.patch", "app:auditfy/staging/api", `{"set":["API_KEY","LOG_LEVEL"],"unset":["JWT_KEY","Z"]}`},
		{"system", "env.patch", "stage:auditfy/staging", `{"set":[],"unset":[]}`},
	}
	if len(events) != len(want) {
		t.Fatalf("events=%+v", events)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Errorf("evento %d = %+v, esperaba %+v", i, events[i], want[i])
		}
	}
}
