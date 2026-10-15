package usecases

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"

	outservice "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

type orphanFixture struct {
	docker  *dockertest.Fake
	orphans *RemoveOrphanNetworks
}

func newOrphanFixture(t *testing.T, prefix string) *orphanFixture {
	t.Helper()
	db := sqlitetest.New(t)
	ingresstest.Stage(t, db, "auditfy", "staging")
	d := dockertest.New()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := &orphanFixture{docker: d,
		orphans: NewRemoveOrphanNetworks(outservice.NewDockerNetworks(d), outservice.NewCatalogReader(db), testCaddyNames.Container, prefix, log)}
	ctx := context.Background()
	if _, err := d.CreateNetwork(ctx, docker.NetworkSpec{Name: testCaddyNames.PanelNetwork, Driver: "overlay", Attachable: true}); err != nil {
		t.Fatal(err)
	}
	d.SetImagePresent("caddy:test")
	if _, err := d.CreateContainer(ctx, docker.ContainerSpec{Name: testCaddyNames.Container, Image: "caddy:test", Network: testCaddyNames.PanelNetwork}); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *orphanFixture) network(t *testing.T, name string, labels map[string]string) {
	t.Helper()
	if _, err := f.docker.CreateNetwork(context.Background(), docker.NetworkSpec{Name: name, Driver: "overlay", Attachable: true, Labels: labels}); err != nil {
		t.Fatal(err)
	}
}

func (f *orphanFixture) exists(name string) bool {
	_, err := f.docker.InspectNetwork(context.Background(), name)
	return !errors.Is(err, docker.ErrNotFound)
}

func (f *orphanFixture) pass(t *testing.T) {
	t.Helper()
	if err := f.orphans.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// D13: una red con etiquetas de stage, con el nombre de esas etiquetas y sin
// stage en la base se borra la segunda vez seguida que el job la ve (catalog
// crea la red antes de guardar el stage). Antes se desconecta muelle-caddy.
func TestOrphanNetworks(t *testing.T) {
	f := newOrphanFixture(t, "")
	ctx := context.Background()
	f.network(t, "auditfy_viejo", naming.StageLabels("auditfy", "viejo"))
	f.network(t, "otro_nombre", naming.StageLabels("auditfy", "otro"))
	f.network(t, "sin_etiquetas", nil)
	f.network(t, "auditfy_staging", naming.StageLabels("auditfy", "staging"))
	if err := f.docker.ConnectNetwork(ctx, "auditfy_viejo", testCaddyNames.Container, nil); err != nil {
		t.Fatal(err)
	}

	f.pass(t)
	if !f.exists("auditfy_viejo") {
		t.Fatal("la primera pasada no la borra")
	}
	f.pass(t)
	if f.exists("auditfy_viejo") {
		t.Fatal("la segunda pasada la borra")
	}
	calls := f.docker.Calls()
	disconnect := slices.Index(calls, "DisconnectNetwork auditfy_viejo "+testCaddyNames.Container)
	remove := slices.Index(calls, "RemoveNetwork auditfy_viejo")
	if disconnect < 0 || remove < 0 || disconnect > remove {
		t.Fatalf("primero se desconecta muelle-caddy: %v", calls)
	}

	f.pass(t)
	for _, n := range []string{"otro_nombre", "sin_etiquetas", "auditfy_staging"} {
		if !f.exists(n) {
			t.Errorf("la red %s no se toca", n)
		}
	}
	for _, c := range f.docker.Calls() {
		for _, n := range []string{"otro_nombre", "sin_etiquetas", "auditfy_staging"} {
			if c == "RemoveNetwork "+n || c == "DisconnectNetwork "+n+" "+testCaddyNames.Container {
				t.Errorf("no debería tocar %s: %s", n, c)
			}
		}
	}
}

// La segunda vez tiene que ser seguida: una red que dejó de estar huérfana
// (se creó su stage) vuelve a contar desde cero.
func TestOrphanNetworksConsecutive(t *testing.T) {
	db := sqlitetest.New(t)
	d := dockertest.New()
	o := NewRemoveOrphanNetworks(outservice.NewDockerNetworks(d), outservice.NewCatalogReader(db), testCaddyNames.Container, "",
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()
	if _, err := d.CreateNetwork(ctx, docker.NetworkSpec{Name: "auditfy_nuevo", Driver: "overlay", Attachable: true,
		Labels: naming.StageLabels("auditfy", "nuevo")}); err != nil {
		t.Fatal(err)
	}
	if err := o.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	ingresstest.Stage(t, db, "auditfy", "nuevo")
	if err := o.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Conn(ctx).ExecContext(ctx, `DELETE FROM stages WHERE slug = 'nuevo'`); err != nil {
		t.Fatal(err)
	}
	if err := o.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := d.InspectNetwork(ctx, "auditfy_nuevo"); err != nil {
		t.Fatalf("vista huérfana una sola vez seguida: %v", err)
	}
}

// Una huérfana con algo conectado no es un error: queda para la próxima.
func TestOrphanNetworksInUse(t *testing.T) {
	f := newOrphanFixture(t, "")
	ctx := context.Background()
	f.network(t, "auditfy_viejo", naming.StageLabels("auditfy", "viejo"))
	if _, err := f.docker.CreateContainer(ctx, docker.ContainerSpec{Name: "tarea", Image: "caddy:test", Network: "auditfy_viejo"}); err != nil {
		t.Fatal(err)
	}
	f.pass(t)
	f.pass(t)
	if !f.exists("auditfy_viejo") {
		t.Fatal("en uso, queda")
	}
	if err := f.docker.RemoveContainer(ctx, "tarea", true); err != nil {
		t.Fatal(err)
	}
	f.pass(t)
	if f.exists("auditfy_viejo") {
		t.Fatal("ya libre, se borra en la pasada siguiente")
	}
}

// Con NetworkPrefix (los tests de integración), solo se miran las redes con
// ese prefijo.
func TestOrphanNetworksPrefix(t *testing.T) {
	f := newOrphanFixture(t, "it3x")
	f.network(t, "auditfy_viejo", naming.StageLabels("auditfy", "viejo"))
	f.network(t, "it3x_viejo", naming.StageLabels("it3x", "viejo"))
	f.pass(t)
	f.pass(t)
	if !f.exists("auditfy_viejo") {
		t.Fatal("sin el prefijo no se toca")
	}
	if f.exists("it3x_viejo") {
		t.Fatal("con el prefijo se borra")
	}
}

// Con Docker caído, un error que lo dice.
func TestOrphanNetworksDockerDown(t *testing.T) {
	f := newOrphanFixture(t, "")
	f.docker.SetUnavailable(true)
	if err := f.orphans.Execute(context.Background()); !errors.Is(err, docker.ErrUnavailable) {
		t.Fatalf("Execute: %v", err)
	}
}
