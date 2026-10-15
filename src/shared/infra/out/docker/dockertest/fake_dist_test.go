package dockertest

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

var _ docker.ServiceRestarter = (*Fake)(nil)

func TestFakeForceUpdateService(t *testing.T) {
	f := New()
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	f.Now = func() time.Time { return now }
	ctx := context.Background()
	f.PutService(docker.Service{Name: "muelle_muelle", Version: 7})

	if err := f.ForceUpdateService(ctx, "muelle_muelle"); err != nil {
		t.Fatal(err)
	}
	s, err := f.InspectService(ctx, "muelle_muelle")
	if err != nil {
		t.Fatal(err)
	}
	if s.Version != 8 || !s.UpdatedAt.Equal(now) {
		t.Fatalf("servicio: Version=%d UpdatedAt=%s, esperaba 8 y %s", s.Version, s.UpdatedAt, now)
	}
	// También por id, como el daemon.
	if err := f.ForceUpdateService(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	if s, _ := f.InspectService(ctx, "muelle_muelle"); s.Version != 9 {
		t.Fatalf("por id: Version=%d", s.Version)
	}
	if !slices.Contains(f.Calls(), "ForceUpdateService muelle_muelle") {
		t.Fatalf("calls=%q", f.Calls())
	}

	if err := f.ForceUpdateService(ctx, "no-existe"); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("servicio que no existe: %v", err)
	}

	f.SetUnavailable(true)
	if err := f.ForceUpdateService(ctx, "muelle_muelle"); !errors.Is(err, docker.ErrUnavailable) {
		t.Fatalf("Docker caído: %v", err)
	}
	f.SetUnavailable(false)

	active, err := f.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.SetInfo(docker.Info{SwarmState: docker.SwarmInactive})
	if err := f.ForceUpdateService(ctx, "muelle_muelle"); !errors.Is(err, docker.ErrUnavailable) {
		t.Fatalf("sin Swarm: %v", err)
	}
	f.SetInfo(active)
	if s, _ := f.InspectService(ctx, "muelle_muelle"); s.Version != 9 {
		t.Fatalf("los que fallaron cambiaron Version: %d", s.Version)
	}
}
