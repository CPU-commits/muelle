package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

var t0 = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

func exec(t *testing.T, db *sqlite.DB, query string, args ...any) int64 {
	t.Helper()
	ctx := context.Background()
	res, err := db.Conn(ctx).ExecContext(ctx, query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// seedApps siembra, con SQL directo (las tablas son de catalog), el proyecto
// p, el stage s y dos apps: api (formulario) y infra (compose).
func seedApps(t *testing.T, db *sqlite.DB) (api, infra int64) {
	t.Helper()
	p := exec(t, db, `INSERT INTO projects (slug, name, created_at) VALUES ('p', 'P', ?)`, sqlite.Millis(t0))
	s := exec(t, db, `INSERT INTO stages (project_id, slug, created_at) VALUES (?, 's', ?)`, p, sqlite.Millis(t0))
	api = exec(t, db, `INSERT INTO apps (stage_id, slug, kind, spec, deploy_timeout_s, config_changed_at, created_at)
		VALUES (?, 'api', 'service', '{"image":"x:1","port":80}', 120, ?, ?)`, s, sqlite.Millis(t0), sqlite.Millis(t0))
	infra = exec(t, db, `INSERT INTO apps (stage_id, slug, kind, compose_yaml, config_changed_at, created_at)
		VALUES (?, 'infra', 'compose', 'services: {}', ?, ?)`, s, sqlite.Millis(t0.Add(time.Minute)), sqlite.Millis(t0))
	return api, infra
}

func TestCatalogAppsGet(t *testing.T) {
	db := sqlitetest.New(t)
	apiID, infraID := seedApps(t, db)
	apps := NewCatalogApps(db)
	ctx := context.Background()

	got, err := apps.Get(ctx, "p", "s", "api")
	if err != nil {
		t.Fatal(err)
	}
	want := service.App{
		ID: apiID, Project: "p", Stage: "s", Slug: "api", Kind: service.KindService,
		Spec: []byte(`{"image":"x:1","port":80}`), DeployTimeoutS: 120, ConfigChangedAt: t0,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Get api:\n got %+v\nwant %+v", got, want)
	}

	got, err = apps.GetByID(ctx, infraID)
	if err != nil {
		t.Fatal(err)
	}
	want = service.App{
		ID: infraID, Project: "p", Stage: "s", Slug: "infra", Kind: service.KindCompose,
		ComposeYAML: "services: {}", DeployTimeoutS: 300, ConfigChangedAt: t0.Add(time.Minute),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetByID infra:\n got %+v\nwant %+v", got, want)
	}

	if _, err := apps.Get(ctx, "p", "s", "nada"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("app que no existe: %v", err)
	}
	if _, err := apps.GetByID(ctx, 999); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("id que no existe: %v", err)
	}
}

// storage_node_id se fija una sola vez (§6.2): el primero gana.
func TestCatalogAppsSetStorageNode(t *testing.T) {
	db := sqlitetest.New(t)
	apiID, _ := seedApps(t, db)
	apps := NewCatalogApps(db)
	ctx := context.Background()

	node, err := apps.SetStorageNode(ctx, apiID, "node-1")
	if err != nil || node != "node-1" {
		t.Fatalf("primera vez: %q %v", node, err)
	}
	node, err = apps.SetStorageNode(ctx, apiID, "node-2")
	if err != nil || node != "node-1" {
		t.Fatalf("segunda vez: %q %v (tiene que quedar node-1)", node, err)
	}
	a, err := apps.GetByID(ctx, apiID)
	if err != nil || a.StorageNodeID != "node-1" {
		t.Fatalf("GetByID: %+v %v", a, err)
	}
	if _, err := apps.SetStorageNode(ctx, 999, "node-1"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("app que no existe: %v", err)
	}
}

// last_deployed_at solo avanza, y nunca toca config_changed_at.
func TestCatalogAppsMarkDeployed(t *testing.T) {
	db := sqlitetest.New(t)
	apiID, _ := seedApps(t, db)
	apps := NewCatalogApps(db)
	ctx := context.Background()

	t1 := t0.Add(time.Hour)
	if err := apps.MarkDeployed(ctx, apiID, t1); err != nil {
		t.Fatal(err)
	}
	if err := apps.MarkDeployed(ctx, apiID, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	a, err := apps.GetByID(ctx, apiID)
	if err != nil {
		t.Fatal(err)
	}
	if a.LastDeployedAt == nil || !a.LastDeployedAt.Equal(t1) || !a.ConfigChangedAt.Equal(t0) {
		t.Fatalf("después de MarkDeployed: %+v", a)
	}
	if err := apps.MarkDeployed(ctx, 999, t1); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("app que no existe: %v", err)
	}
}

// Los destinos de los dominios salen de la tabla domains (de ingress).
func TestDomainTargetsByApp(t *testing.T) {
	db := sqlitetest.New(t)
	apiID, infraID := seedApps(t, db)
	for _, d := range []struct {
		app  int64
		host string
		svc  string
		port int
	}{
		{infraID, "b.example.com", "web", 8080},
		{infraID, "a.example.com", "web", 80},
		{apiID, "api.example.com", "api", 3000},
	} {
		exec(t, db, `INSERT INTO domains (app_id, hostname, target_service, target_port, tls_mode, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'http', ?, ?)`, d.app, d.host, d.svc, d.port, sqlite.Millis(t0), sqlite.Millis(t0))
	}
	targets := NewDomainTargets(db)
	got, err := targets.ByApp(context.Background(), infraID)
	if err != nil {
		t.Fatal(err)
	}
	want := []service.DomainTarget{
		{Hostname: "a.example.com", Service: "web", Port: 80},
		{Hostname: "b.example.com", Service: "web", Port: 8080},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ByApp:\n got %+v\nwant %+v", got, want)
	}
	got, err = targets.ByApp(context.Background(), 999)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("app sin dominios: %#v %v (tiene que ser [] sin error)", got, err)
	}
}
