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

// PutNetwork deja una red tal cual, con sus subredes: así un test de install
// pone una muelle_panel que ya existía (con una subred pública, o local).
func TestFakePutNetwork(t *testing.T) {
	f := New()
	ctx := context.Background()
	want := docker.Network{
		ID: "net-propia", Name: "muelle_panel", Driver: "overlay", Scope: "swarm", Attachable: true,
		Labels: map[string]string{"a": "b"}, Subnets: []string{"203.0.113.0/24"},
	}
	f.PutNetwork(want)
	got, err := f.InspectNetwork(ctx, "muelle_panel")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.Driver != want.Driver || got.Scope != want.Scope || !got.Attachable ||
		!slices.Equal(got.Subnets, want.Subnets) || got.Labels["a"] != "b" {
		t.Fatalf("InspectNetwork = %+v, esperaba %+v", got, want)
	}
	// También por id, y la copia del test no cambia la red del fake.
	want.Subnets[0] = "10.9.9.0/24"
	if got, err := f.InspectNetwork(ctx, "net-propia"); err != nil || got.Subnets[0] != "203.0.113.0/24" {
		t.Fatalf("por id: %+v, %v", got, err)
	}
	if _, err := f.CreateNetwork(ctx, docker.NetworkSpec{Name: "muelle_panel", Driver: "overlay", Attachable: true}); !errors.Is(err, docker.ErrConflict) {
		t.Fatalf("CreateNetwork con el mismo nombre: %v, esperaba ErrConflict", err)
	}

	// Sin ID se le asigna uno; una red local sirve con el Swarm inactivo.
	f.SetInfo(docker.Info{SwarmState: docker.SwarmInactive})
	f.PutNetwork(docker.Network{Name: "local", Driver: "bridge", Scope: "local"})
	if got, err := f.InspectNetwork(ctx, "local"); err != nil || got.ID == "" || got.Driver != "bridge" || got.Subnets != nil {
		t.Fatalf("red sin id: %+v, %v", got, err)
	}
}
