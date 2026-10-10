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
	sharedentity "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
)

func TestReplaceEnv(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	o := entity.AppOwner(f.appID)
	if err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: appTarget, Vars: map[string]string{"A": "1", "B": "2"}}); err != nil {
		t.Fatal(err)
	}
	// PUT reemplaza todo el set: lo que no viene se borra.
	if err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: appTarget, Vars: map[string]string{"B": "3", "C": ""}}); err != nil {
		t.Fatal(err)
	}
	if got := f.plain(t, o); !maps.Equal(got, map[string]string{"B": "3", "C": ""}) {
		t.Fatalf("got=%v", got)
	}
	// Un set vacío borra todo.
	if err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: appTarget, Vars: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	if got := f.plain(t, o); len(got) != 0 {
		t.Fatalf("got=%v", got)
	}
}

// secret es true si se omite, y aplica a todas las variables del PUT.
func TestReplaceEnvSecretDefault(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.replace.Execute(ctx, dto.ReplaceEnv{Target: appTarget, Vars: map[string]string{"A": "1", "B": "2"}})
	vars, _ := f.get.Execute(ctx, appTarget)
	if len(vars) != 2 || !vars[0].Secret || !vars[1].Secret {
		t.Fatalf("sin secret: %+v", vars)
	}
	f.replace.Execute(ctx, dto.ReplaceEnv{Target: appTarget, Vars: map[string]string{"A": "1"}, Secret: ptr(false)})
	vars, _ = f.get.Execute(ctx, appTarget)
	if len(vars) != 1 || vars[0].Secret {
		t.Fatalf("secret false: %+v", vars)
	}
	f.replace.Execute(ctx, dto.ReplaceEnv{Target: appTarget, Vars: map[string]string{"A": "1"}, Secret: ptr(true)})
	vars, _ = f.get.Execute(ctx, appTarget)
	if len(vars) != 1 || !vars[0].Secret {
		t.Fatalf("secret true: %+v", vars)
	}
}

// Portado de TestValidation (Tarea 15 ref), parte de PUT.
func TestReplaceEnvValidation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	cases := []struct {
		name string
		vars map[string]string
	}{
		{"sin vars", nil},
		{"clave inválida", map[string]string{"1BAD": "x"}},
		{"clave con guion", map[string]string{"bad-key": "x"}},
		{"prefijo COMPOSE_", map[string]string{"COMPOSE_PROFILES": "x"}},
		{"valor grande", map[string]string{"BIG": strings.Repeat("x", entity.MaxValueBytes+1)}},
	}
	for _, c := range cases {
		err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: appTarget, Vars: c.vars})
		if validationField(t, err) != "vars" {
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
	many["ONE_MORE"] = "x"
	if err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: appTarget, Vars: many}); validationField(t, err) != "vars" {
		t.Errorf("superar MaxVars: %v", err)
	}
	// Lo que falla la validación no toca nada.
	if got := f.plain(t, entity.AppOwner(f.appID)); len(got) != entity.MaxVars {
		t.Fatalf("quedaron %d variables", len(got))
	}
}

func TestReplaceEnvNotFound(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: dto.EnvTarget{Project: "auditfy", Stage: "staging", App: "nope"}, Vars: map[string]string{"A": "1"}})
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	if n := seedtest.Count(t, f.db, `SELECT COUNT(*) FROM env_vars`); n != 0 {
		t.Fatalf("quedaron %d variables", n)
	}
	if events := f.auditEvents(t); len(events) != 0 {
		t.Fatalf("un PUT fallido no se audita: %+v", events)
	}
}

// Portado de TestEnvMarksAppsPending (Tarea 10 ref), parte de la app.
func TestReplaceEnvMarksOnlyThatApp(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	worker := seedtest.App(t, f.db, f.stageID, "worker")
	if err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: appTarget, Vars: map[string]string{"A": "x"}}); err != nil {
		t.Fatal(err)
	}
	if !seedtest.ConfigChangedAt(t, f.db, f.appID).Equal(f.clock.Now()) {
		t.Fatal("cambiar variables de una app marca esa app")
	}
	if !seedtest.ConfigChangedAt(t, f.db, worker).Equal(seedtest.T0) {
		t.Fatal("cambiar variables de una app no marca las otras")
	}
}

// Un PUT que falla en Mark deja las variables como estaban (rollback), y no se audita.
func TestReplaceEnvRollsBackWhenMarkFails(t *testing.T) {
	boom := errors.New("boom")
	f := newFixtureWithMarker(t, failingMarker{err: boom})
	ctx := context.Background()
	o := entity.AppOwner(f.appID)
	sealed, _ := f.replace.crypto.Seal(o, []entity.EnvVar{{Key: "A", Value: []byte("antes"), Secret: true, UpdatedAt: seedtest.T0}})
	if err := f.repo.Replace(ctx, o, sealed); err != nil {
		t.Fatal(err)
	}

	err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: appTarget, Vars: map[string]string{"A": "después", "B": "nueva"}})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	if got := f.plain(t, o); !maps.Equal(got, map[string]string{"A": "antes"}) {
		t.Fatalf("el rollback debía dejar las variables como estaban: %v", got)
	}
	rows, _ := f.repo.List(ctx, o)
	if !rows[0].UpdatedAt.Equal(seedtest.T0) {
		t.Fatalf("updated_at cambió: %v", rows[0].UpdatedAt)
	}
	if events := f.auditEvents(t); len(events) != 0 {
		t.Fatalf("un PUT revertido no se audita: %+v", events)
	}
}

// La auditoría guarda claves, nunca valores, y el actor sale del Principal.
func TestReplaceEnvAuditsKeysOnly(t *testing.T) {
	f := newFixture(t)
	ctx := sharedentity.WithPrincipal(context.Background(),
		sharedentity.Principal{Token: &sharedentity.TokenScope{ID: 7, ProjectSlug: "auditfy", StageSlug: "staging"}})
	if err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: appTarget, Vars: map[string]string{"JWT_KEY": "k", "DATABASE_URL": "postgres://x"}}); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	if err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: stageTarget, Vars: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	events := f.auditEvents(t)
	want := []auditRow{
		{"token", "env.replace", "app:auditfy/staging/api", `{"keys":["DATABASE_URL","JWT_KEY"]}`},
		{"token", "env.replace", "stage:auditfy/staging", `{"keys":[]}`},
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
