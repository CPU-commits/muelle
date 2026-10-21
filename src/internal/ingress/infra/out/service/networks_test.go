package service

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
)

// List devuelve todas las redes con sus etiquetas (el job "proxy" busca las
// huérfanas por ellas), y Inspect también las trae.
func TestDockerNetworksList(t *testing.T) {
	f := dockertest.New()
	ctx := context.Background()
	labels := naming.StageLabels("auditfy", "staging")
	if _, err := f.CreateNetwork(ctx, docker.NetworkSpec{Name: "auditfy_staging", Driver: "overlay", Attachable: true, Labels: labels}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.CreateNetwork(ctx, docker.NetworkSpec{Name: "local", Driver: "bridge"}); err != nil {
		t.Fatal(err)
	}
	n := NewDockerNetworks(f)

	list, err := n.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("redes: %+v", list)
	}
	byName := map[string]int{}
	for i, net := range list {
		byName[net.Name] = i
	}
	stage, local := list[byName["auditfy_staging"]], list[byName["local"]]
	if !stage.Overlay || !stage.Attachable || !maps.Equal(stage.Labels, labels) {
		t.Fatalf("la red del stage: %+v", stage)
	}
	if local.Overlay || len(local.Labels) != 0 {
		t.Fatalf("la red local: %+v", local)
	}

	got, found, err := n.Inspect(ctx, "auditfy_staging")
	if err != nil || !found || !maps.Equal(got.Labels, labels) {
		t.Fatalf("Inspect: %+v %v %v", got, found, err)
	}

	f.SetUnavailable(true)
	if _, err := n.List(ctx); !errors.Is(err, entity.ErrDockerUnavailable) {
		t.Fatalf("Docker caído: %v", err)
	}
}
