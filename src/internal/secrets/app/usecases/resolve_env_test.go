package usecases

import (
	"context"
	"errors"
	"maps"
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
	if want := map[string]string{"NATS": "nats:4222", "LEVEL": "debug", "DB": "pg"}; !maps.Equal(got, want) {
		t.Fatalf("got=%v", got)
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
		if err != nil || !maps.Equal(got, map[string]string{"WHERE": want}) {
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
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got=%v err=%v", got, err)
	}
}
