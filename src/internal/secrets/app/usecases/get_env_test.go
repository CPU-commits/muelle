package usecases

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/secrets/app/dto"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// Portado de TestListMasksSecrets (Tarea 15 ref).
func TestGetEnvMasksSecrets(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: appTarget, Vars: map[string]string{"DATABASE_URL": "postgres://x"}}); err != nil {
		t.Fatal(err)
	}
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: appTarget, Set: map[string]string{"LOG_LEVEL": "debug"}, Secret: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	vars, err := f.get.Execute(ctx, appTarget)
	if err != nil {
		t.Fatal(err)
	}
	if len(vars) != 2 || vars[0].Key != "DATABASE_URL" || vars[0].Value != nil || !vars[0].Secret {
		t.Fatalf("vars=%+v", vars)
	}
	if vars[1].Value == nil || *vars[1].Value != "debug" || vars[1].Secret || !vars[1].UpdatedAt.Equal(f.clock.Now()) {
		t.Fatalf("vars[1]=%+v", vars[1])
	}

	// Las del stage son otro set.
	if vars, err := f.get.Execute(ctx, stageTarget); err != nil || len(vars) != 0 {
		t.Fatalf("stage: vars=%+v err=%v", vars, err)
	}
}

// Portado de TestValuesAreEncryptedAtRest (Tarea 15 ref).
func TestValuesAreEncryptedAtRest(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: appTarget, Vars: map[string]string{"TOKEN": "super-secreto"}, Secret: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := f.db.Conn(ctx).QueryRowContext(ctx, `SELECT value FROM env_vars WHERE key = 'TOKEN'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "super-secreto") {
		t.Fatalf("el valor quedó en claro: %q", raw)
	}
	// Aunque no sea secreta, se guarda cifrada y se lee descifrada.
	vars, _ := f.get.Execute(ctx, appTarget)
	if len(vars) != 1 || vars[0].Value == nil || *vars[0].Value != "super-secreto" {
		t.Fatalf("vars=%+v", vars)
	}
}

func TestGetEnvNotFound(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, target := range []dto.EnvTarget{
		{Project: "auditfy", Stage: "staging", App: "nope"},
		{Project: "auditfy", Stage: "nope"},
		{Project: "nope", Stage: "staging"},
	} {
		if _, err := f.get.Execute(ctx, target); !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("%+v: got %v", target, err)
		}
	}
}
