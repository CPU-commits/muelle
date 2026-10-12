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
		// Un body null o {} no puede pasar por un PATCH que no hace nada.
		{"sin set ni unset", dto.PatchEnv{}, "set"},
		{"set y unset vacíos", dto.PatchEnv{Set: map[string]string{}, Unset: []string{}}, "set"},
		{"set y unset a la vez", dto.PatchEnv{Set: map[string]string{"A": "1"}, Unset: []string{"A"}}, "unset"},
		{"valor que no es UTF-8", dto.PatchEnv{Set: map[string]string{"A": "\xff"}}, "set"},
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

// unset y set tienen un máximo de MaxVars claves, que se revisa antes de
// tocar la base: así un PATCH de 1 MiB con 100k claves en unset no hace 100k
// DELETE con el lock de escritura tomado ni deja 1 MiB en la auditoría.
func TestPatchEnvLimitsBeforeTransaction(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	// Un dueño inexistente daría 404 si la revisión fuera dentro de la transacción.
	missing := dto.EnvTarget{Project: "auditfy", Stage: "staging", App: "nope"}

	unset := make([]string, entity.MaxVars+1)
	for i := range unset {
		unset[i] = fmt.Sprintf("K%d", i)
	}
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: missing, Unset: unset}); validationField(t, err) != "unset" {
		t.Errorf("unset de más: %v", err)
	}
	set := map[string]string{}
	for i := range entity.MaxVars + 1 {
		set[fmt.Sprintf("K%d", i)] = "x"
	}
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: missing, Set: set}); validationField(t, err) != "set" {
		t.Errorf("set de más: %v", err)
	}

	// Justo MaxVars claves en unset sí pasa.
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: missing, Unset: unset[:entity.MaxVars]}); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("%d claves en unset deberían pasar la validación: %v", entity.MaxVars, err)
	}
	if events := f.auditEvents(t); len(events) != 0 {
		t.Fatalf("events=%+v", events)
	}
}

// Las variables de un dueño suman como máximo 1 MiB (claves y valores),
// contando las que ya tiene.
func TestPatchEnvTotalSize(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	big := strings.Repeat("x", entity.MaxValueBytes)
	vars := map[string]string{}
	total := 0
	for i := range 15 {
		k := fmt.Sprintf("K%02d", i)
		vars[k] = big
		total += len(k) + len(big)
	}
	if err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: appTarget, Vars: vars}); err != nil {
		t.Fatal(err)
	}
	rest := strings.Repeat("x", entity.MaxOwnerBytes-total-len("REST"))
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: appTarget, Set: map[string]string{"REST": rest}}); err != nil {
		t.Fatalf("justo 1 MiB debería caber: %v", err)
	}
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: appTarget, Set: map[string]string{"X": "1"}}); validationField(t, err) != "set" {
		t.Errorf("un byte de más: %v", err)
	}
	// Reemplazar una variable no cuenta dos veces, y lo que se borra deja lugar.
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: appTarget, Set: map[string]string{"K00": big[1:], "Y": ""}}); err != nil {
		t.Errorf("reemplazar por un valor un byte más corto y agregar una de 1 byte: %v", err)
	}
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: appTarget, Set: map[string]string{"X": "1"}, Unset: []string{"K01"}}); err != nil {
		t.Errorf("borrar una y agregar otra chica: %v", err)
	}
	got := f.plain(t, entity.AppOwner(f.appID))
	if _, ok := got["K01"]; ok || got["X"] != "1" || len(got["K00"]) != entity.MaxValueBytes-1 {
		t.Fatalf("quedaron %d variables", len(got))
	}
	// Un dueño no limita a otro.
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: stageTarget, Set: map[string]string{"REST": rest}}); err != nil {
		t.Errorf("el stage tiene su propio límite: %v", err)
	}
}

// Un PATCH que no cambia ninguna fila (solo borra claves que no existen)
// responde bien, pero no marca apps ni se audita.
func TestPatchEnvNoopSkipsMarkAndAudit(t *testing.T) {
	// Con un ChangeMarker que falla, llamar a Mark haría fallar el PATCH.
	f := newFixtureWithMarker(t, failingMarker{err: errors.New("Mark no se debía llamar")})
	ctx := context.Background()
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: stageTarget, Unset: []string{"NOPE", "TAMPOCO"}}); err != nil {
		t.Fatalf("got %v", err)
	}
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: appTarget, Unset: []string{"NOPE"}}); err != nil {
		t.Fatalf("got %v", err)
	}
	if !seedtest.ConfigChangedAt(t, f.db, f.appID).Equal(seedtest.T0) {
		t.Fatal("un PATCH sin cambios no marca la app")
	}
	if events := f.auditEvents(t); len(events) != 0 {
		t.Fatalf("un PATCH sin cambios no se audita: %+v", events)
	}
	// Aunque no cambie nada, un dueño inexistente sigue siendo 404.
	err := f.patch.Execute(ctx, dto.PatchEnv{Target: dto.EnvTarget{Project: "auditfy", Stage: "nope"}, Unset: []string{"NOPE"}})
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

// Una entrada de set con el mismo valor y el mismo secret que la guardada no
// es un cambio: no se reescribe, y si no cambia nada más, no se marca ni se audita.
func TestPatchEnvUnchangedSet(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	o := entity.AppOwner(f.appID)
	if err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: appTarget, Vars: map[string]string{"A": "1", "B": "2"}}); err != nil {
		t.Fatal(err)
	}
	putAt := f.clock.Now()
	f.clock.Advance(time.Minute)

	// Mismo valor y secret (true por defecto, como en el PUT).
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: appTarget, Set: map[string]string{"A": "1"}}); err != nil {
		t.Fatal(err)
	}
	// Todo igual, más un unset de una clave que no existe: tampoco cambia nada.
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: appTarget, Set: map[string]string{"A": "1", "B": "2"}, Unset: []string{"NOPE"}}); err != nil {
		t.Fatal(err)
	}
	if !seedtest.ConfigChangedAt(t, f.db, f.appID).Equal(putAt) {
		t.Fatal("un PATCH sin cambios no marca la app")
	}
	if events := f.auditEvents(t); len(events) != 1 {
		t.Fatalf("un PATCH sin cambios no se audita: %+v", events)
	}
	rows, _ := f.repo.List(ctx, o)
	if !rows[0].UpdatedAt.Equal(putAt) || !rows[1].UpdatedAt.Equal(putAt) {
		t.Fatalf("una variable sin cambios no se reescribe: %+v", rows)
	}

	// Cambiar solo secret sí es un cambio.
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: appTarget, Set: map[string]string{"A": "1"}, Secret: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if !seedtest.ConfigChangedAt(t, f.db, f.appID).Equal(f.clock.Now()) {
		t.Fatal("cambiar secret marca la app")
	}
	vars, _ := f.get.Execute(ctx, appTarget)
	if vars[0].Key != "A" || vars[0].Secret || vars[0].Value == nil || *vars[0].Value != "1" || !vars[0].UpdatedAt.Equal(f.clock.Now()) {
		t.Fatalf("A=%+v", vars[0])
	}

	// En un PATCH mixto solo se reescribe lo que cambia; la auditoría lista lo pedido.
	changedSecretAt := f.clock.Now()
	f.clock.Advance(time.Minute)
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: appTarget, Set: map[string]string{"A": "1", "B": "3"}, Secret: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	rows, _ = f.repo.List(ctx, o)
	if !rows[0].UpdatedAt.Equal(changedSecretAt) || !rows[1].UpdatedAt.Equal(f.clock.Now()) {
		t.Fatalf("A no cambió y B sí: %+v", rows)
	}
	if got := f.plain(t, o); !maps.Equal(got, map[string]string{"A": "1", "B": "3"}) {
		t.Fatalf("got=%v", got)
	}
	events := f.auditEvents(t)
	if len(events) != 3 || events[2].details != `{"set":["A","B"],"unset":[]}` {
		t.Fatalf("events=%+v", events)
	}
}

// Una fila que no descifra no bloquea los PATCH del dueño: medir no descifra,
// y sobrescribirla la arregla (no descifrar cuenta como cambio).
func TestPatchEnvBrokenRowDoesNotBlock(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	o := entity.AppOwner(f.appID)
	// BROKEN se cifró con la AAD del stage y se guardó en la app: no descifra.
	sealed, _ := f.patch.crypto.Seal(entity.StageOwner(f.stageID), []entity.EnvVar{{Key: "BROKEN", Value: []byte("x"), Secret: true, UpdatedAt: seedtest.T0}})
	if err := f.repo.Replace(ctx, o, sealed); err != nil {
		t.Fatal(err)
	}

	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: appTarget, Set: map[string]string{"OTHER": "y"}}); err != nil {
		t.Fatalf("agregar otra variable: %v", err)
	}
	// El mismo valor que tenía: si "no descifra" contara como "sin cambios", quedaría rota.
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: appTarget, Set: map[string]string{"BROKEN": "x"}}); err != nil {
		t.Fatalf("sobrescribir la rota: %v", err)
	}
	if got := f.plain(t, o); !maps.Equal(got, map[string]string{"BROKEN": "x", "OTHER": "y"}) {
		t.Fatalf("got=%v", got)
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
	production := seedtest.Stage(t, f.db, seedtest.ProjectID(t, f.db, "auditfy"), "production")
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
	// Un PATCH que solo borra claves que no existen no cambia nada y no se audita.
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: stageTarget, Unset: []string{"NOPE"}}); err != nil {
		t.Fatal(err)
	}
	// Si borra al menos una que existe, se audita con las claves pedidas.
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: appTarget, Unset: []string{"NOPE", "API_KEY"}}); err != nil {
		t.Fatal(err)
	}
	events := f.auditEvents(t)
	want := []auditRow{
		{"system", "env.patch", "app:auditfy/staging/api", `{"set":["API_KEY","LOG_LEVEL"],"unset":["JWT_KEY","Z"]}`},
		{"system", "env.patch", "app:auditfy/staging/api", `{"set":[],"unset":["API_KEY","NOPE"]}`},
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
