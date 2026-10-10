package usecases

import (
	"context"
	"maps"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/secrets/app/dto"
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
	got, err := f.resolve.Execute(ctx, dto.ResolveEnv{StageID: f.stageID, AppID: f.appID})
	if err != nil {
		t.Fatal(err)
	}
	// La app gana, y los secretos sí se descifran: esto es lo que recibe el deploy.
	if want := map[string]string{"NATS": "nats:4222", "LEVEL": "debug", "DB": "pg"}; !maps.Equal(got, want) {
		t.Fatalf("got=%v", got)
	}
}

func TestResolveEnvEmpty(t *testing.T) {
	f := newFixture(t)
	got, err := f.resolve.Execute(context.Background(), dto.ResolveEnv{StageID: f.stageID, AppID: f.appID})
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got=%v err=%v", got, err)
	}
}
