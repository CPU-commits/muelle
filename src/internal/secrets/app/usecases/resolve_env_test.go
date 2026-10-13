package usecases

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/secrets/app/dto"
	"github.com/CPU-commits/muelle/src/internal/secrets/seedtest"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// Portado de TestResolveMergesStageAndApp (Tarea 15 ref).
func TestResolveEnvMergesStageAndApp(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: stageTarget, Vars: map[string]string{"NATS": "nats:4222", "LEVEL": "info"}, Secret: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: appTarget, Vars: map[string]string{"LEVEL": "debug", "DB": "pg"}}); err != nil {
		t.Fatal(err)
	}
	got, err := f.resolve.Execute(ctx, dto.ResolveEnv{AppID: f.appID})
	if err != nil {
		t.Fatal(err)
	}
	// La app gana, y los secretos sí se descifran: esto es lo que recibe el deploy.
	if want := map[string]string{"NATS": "nats:4222", "LEVEL": "debug", "DB": "pg"}; !maps.Equal(got.Vars, want) {
		t.Fatalf("got=%v", got.Vars)
	}
}

// El stage sale de la app, así que nunca se mezclan las variables de otro stage.
func TestResolveEnvUsesTheAppsStage(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	production := seedtest.Stage(t, f.db, seedtest.ProjectID(t, f.db, "auditfy"), "production")
	prodApp := seedtest.App(t, f.db, production, "api")
	prodStage := dto.EnvTarget{Project: "auditfy", Stage: "production"}
	if err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: stageTarget, Vars: map[string]string{"WHERE": "staging"}}); err != nil {
		t.Fatal(err)
	}
	if err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: prodStage, Vars: map[string]string{"WHERE": "production"}}); err != nil {
		t.Fatal(err)
	}
	for appID, want := range map[int64]string{f.appID: "staging", prodApp: "production"} {
		got, err := f.resolve.Execute(ctx, dto.ResolveEnv{AppID: appID})
		if err != nil || !maps.Equal(got.Vars, map[string]string{"WHERE": want}) {
			t.Errorf("app %d: got=%v err=%v", appID, got, err)
		}
	}
}

func TestResolveEnvUnknownApp(t *testing.T) {
	f := newFixture(t)
	if got, err := f.resolve.Execute(context.Background(), dto.ResolveEnv{AppID: 999}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestResolveEnvEmpty(t *testing.T) {
	f := newFixture(t)
	got, err := f.resolve.Execute(context.Background(), dto.ResolveEnv{AppID: f.appID})
	if err != nil || got.Vars == nil || len(got.Vars) != 0 || len(got.SecretValues) != 0 {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

// SecretValues son los valores de las variables secretas del stage y de la
// app, también los del stage que la app pisa: el deploy los reemplaza por ***
// en su log (§7). Sin repetidos ni vacíos, y los más largos primero, para que
// un secreto que contiene a otro se enmascare entero.
func TestResolveEnvSecretValues(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: stageTarget, Vars: map[string]string{"TOKEN": "stage-token", "SHARED": "abc", "EMPTY": ""}}); err != nil {
		t.Fatal(err)
	}
	if err := f.patch.Execute(ctx, dto.PatchEnv{Target: stageTarget, Set: map[string]string{"LEVEL": "info"}, Secret: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if err := f.replace.Execute(ctx, dto.ReplaceEnv{Target: appTarget, Vars: map[string]string{"TOKEN": "app-token-largo", "OTHER": "abc"}}); err != nil {
		t.Fatal(err)
	}
	got, err := f.resolve.Execute(ctx, dto.ResolveEnv{AppID: f.appID})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"app-token-largo", "stage-token", "abc"}; !slices.Equal(got.SecretValues, want) {
		t.Fatalf("SecretValues=%q, esperaba %q", got.SecretValues, want)
	}
	if got.Vars["TOKEN"] != "app-token-largo" || got.Vars["LEVEL"] != "info" {
		t.Fatalf("Vars=%v", got.Vars)
	}
}
