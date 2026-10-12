package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/catalog/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/catalog/domain/vo"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

type repos struct {
	db       *sqlitedb.DB
	projects *ProjectRepository
	stages   *StageRepository
	apps     *AppRepository
}

func newRepos(t *testing.T) repos {
	t.Helper()
	db := sqlitetest.New(t)
	return repos{db, NewProjectRepository(db), NewStageRepository(db), NewAppRepository(db)}
}

func specOf(t *testing.T, image string) vo.AppSpec {
	t.Helper()
	s, err := vo.ParseAppSpec([]byte(`{"image":"` + image + `","port":80}`))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (r repos) project(t *testing.T, slug, name string) entity.Project {
	t.Helper()
	p, err := entity.NewProject(slug, name, t0)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.projects.Create(context.Background(), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func (r repos) stage(t *testing.T, p entity.Project, slug string) entity.Stage {
	t.Helper()
	s, err := entity.NewStage(p, slug, t0)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.stages.Create(context.Background(), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (r repos) serviceApp(t *testing.T, s entity.Stage, slug string) entity.App {
	t.Helper()
	a, err := entity.NewServiceApp(s, slug, specOf(t, "x"), 0, t0)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.apps.Create(context.Background(), &a); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestProjectsCRUD(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()

	p := r.project(t, "auditfy", "Auditfy")
	if p.ID == 0 || p.Slug != "auditfy" || p.CreatedAt.IsZero() {
		t.Fatalf("got %+v", p)
	}
	dup, _ := entity.NewProject("auditfy", "Otro", t0)
	if err := r.projects.Create(ctx, &dup); !errors.Is(err, srepo.ErrConflict) {
		t.Fatalf("esperaba ErrConflict, got %v", err)
	}
	r.project(t, "beta", "Beta")

	list, err := r.projects.List(ctx)
	if err != nil || len(list) != 2 || list[0].Slug != "auditfy" {
		t.Fatalf("list=%+v err=%v", list, err)
	}

	got, err := r.projects.GetBySlug(ctx, "auditfy")
	if err != nil || got != p {
		t.Fatalf("got=%+v want=%+v err=%v", got, p, err)
	}
	p.Name = "Auditfy SpA"
	if err := r.projects.Update(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, _ = r.projects.GetBySlug(ctx, "auditfy")
	if got.Name != "Auditfy SpA" {
		t.Fatalf("got %+v", got)
	}
	if err := r.projects.Update(ctx, entity.Project{ID: 999, Name: "x"}); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}

	beta, _ := r.projects.GetBySlug(ctx, "beta")
	if err := r.projects.Delete(ctx, beta.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.projects.GetBySlug(ctx, "beta"); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
	if err := r.projects.Delete(ctx, beta.ID); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
}

func TestProjectsListEmptyIsNotNil(t *testing.T) {
	r := newRepos(t)
	list, err := r.projects.List(context.Background())
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("list=%#v err=%v", list, err)
	}
}

func TestStagesCRUD(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	p := r.project(t, "auditfy", "Auditfy")
	beta := r.project(t, "beta", "Beta")

	s := r.stage(t, p, "staging")
	if s.ID == 0 || s.ProjectSlug != "auditfy" || s.Slug != "staging" {
		t.Fatalf("s=%+v", s)
	}
	dup, _ := entity.NewStage(p, "staging", t0)
	if err := r.stages.Create(ctx, &dup); !errors.Is(err, srepo.ErrConflict) {
		t.Fatalf("esperaba ErrConflict, got %v", err)
	}
	// El mismo slug en otro proyecto sí se permite.
	r.stage(t, beta, "staging")
	r.stage(t, p, "production")

	list, err := r.stages.ListByProject(ctx, p.ID)
	if err != nil || len(list) != 2 || list[0].Slug != "production" {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if n, err := r.stages.CountByProject(ctx, p.ID); err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}

	got, err := r.stages.Get(ctx, "auditfy", "staging")
	if err != nil || got != s {
		t.Fatalf("got=%+v want=%+v err=%v", got, s, err)
	}
	if _, err := r.stages.Get(ctx, "auditfy", "nope"); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
	if _, err := r.stages.Get(ctx, "beta", "production"); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("un stage de otro proyecto no se encuentra: %v", err)
	}

	prod, _ := r.stages.Get(ctx, "auditfy", "production")
	if err := r.stages.Delete(ctx, prod.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.stages.Delete(ctx, prod.ID); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
}

// Si el padre se borra entre la lectura y el insert, el resultado es ErrNotFound (404), no un 500.
func TestCreateWithMissingParentIsNotFound(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	s := entity.Stage{ProjectID: 999, ProjectSlug: "x", Slug: "staging", CreatedAt: t0}
	if err := r.stages.Create(ctx, &s); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("stage: esperaba ErrNotFound, got %v", err)
	}
	a, _ := entity.NewServiceApp(entity.Stage{ID: 999, ProjectSlug: "x", Slug: "y"}, "api", specOf(t, "x"), 0, t0)
	if err := r.apps.Create(ctx, &a); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("app: esperaba ErrNotFound, got %v", err)
	}
}

func TestDeleteProjectWithStagesFails(t *testing.T) {
	r := newRepos(t)
	p := r.project(t, "auditfy", "Auditfy")
	r.stage(t, p, "staging")
	if err := r.projects.Delete(context.Background(), p.ID); !errors.Is(err, srepo.ErrNotEmpty) {
		t.Fatalf("esperaba ErrNotEmpty, got %v", err)
	}
}

func TestAppsCRUD(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	s := r.stage(t, r.project(t, "auditfy", "Auditfy"), "staging")

	api := r.serviceApp(t, s, "api")
	got, err := r.apps.Get(ctx, "auditfy", "staging", "api")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != api.ID || got.Kind != entity.KindService || string(got.Spec) != string(specOf(t, "x").JSON()) ||
		got.DeployTimeoutS != entity.DefaultDeployTimeoutS || got.ProjectSlug != "auditfy" ||
		got.StageSlug != "staging" || got.LastDeployedAt != nil || got.ComposeYAML != "" {
		t.Fatalf("got=%+v", got)
	}
	dup, _ := entity.NewServiceApp(s, "api", specOf(t, "x"), 0, t0)
	if err := r.apps.Create(ctx, &dup); !errors.Is(err, srepo.ErrConflict) {
		t.Fatalf("esperaba ErrConflict, got %v", err)
	}

	nats, _ := entity.NewComposeApp(s, "nats", "services: {}\n", 60, t0)
	if err := r.apps.Create(ctx, &nats); err != nil {
		t.Fatal(err)
	}
	gotNats, _ := r.apps.Get(ctx, "auditfy", "staging", "nats")
	if gotNats.ComposeYAML != "services: {}\n" || gotNats.Spec != nil || gotNats.DeployTimeoutS != 60 || gotNats.Kind != entity.KindCompose {
		t.Fatalf("nats=%+v", gotNats)
	}

	list, err := r.apps.ListByStage(ctx, s.ID)
	if err != nil || len(list) != 2 || list[0].Slug != "api" {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if n, err := r.apps.CountByStage(ctx, s.ID); err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}

	// Update guarda spec, timeout y config_changed_at.
	t1 := t0.Add(time.Minute)
	if err := got.ChangeSpec(specOf(t, "y"), t1); err != nil {
		t.Fatal(err)
	}
	if err := got.SetDeployTimeout(120); err != nil {
		t.Fatal(err)
	}
	if err := r.apps.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	again, _ := r.apps.Get(ctx, "auditfy", "staging", "api")
	if string(again.Spec) != string(specOf(t, "y").JSON()) || again.DeployTimeoutS != 120 || !again.ConfigChangedAt.Equal(t1) {
		t.Fatalf("again=%+v", again)
	}
	if err := r.apps.Update(ctx, entity.App{ID: 999, Kind: entity.KindCompose, ComposeYAML: "a"}); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}

	if err := r.apps.Delete(ctx, nats.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.apps.Get(ctx, "auditfy", "staging", "nats"); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
	if err := r.apps.Delete(ctx, nats.ID); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
}

func TestLastDeployedAtRoundTrip(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	s := r.stage(t, r.project(t, "auditfy", "Auditfy"), "staging")
	a, _ := entity.NewServiceApp(s, "api", specOf(t, "x"), 0, t0)
	deployed := t0.Add(time.Hour)
	a.LastDeployedAt = &deployed
	if err := r.apps.Create(ctx, &a); err != nil {
		t.Fatal(err)
	}
	got, _ := r.apps.Get(ctx, "auditfy", "staging", "api")
	if got.LastDeployedAt == nil || !got.LastDeployedAt.Equal(deployed) {
		t.Fatalf("got=%+v", got)
	}
	if got.PendingChanges() {
		t.Error("la config es anterior al deploy")
	}
}

func TestDeleteStageWithAppsFails(t *testing.T) {
	r := newRepos(t)
	s := r.stage(t, r.project(t, "auditfy", "Auditfy"), "staging")
	r.serviceApp(t, s, "api")
	if err := r.stages.Delete(context.Background(), s.ID); !errors.Is(err, srepo.ErrNotEmpty) {
		t.Fatalf("esperaba ErrNotEmpty, got %v", err)
	}
}

func TestRepositoriesJoinTheTransaction(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	boom := errors.New("boom")
	err := r.db.InTx(ctx, func(ctx context.Context) error {
		p, _ := entity.NewProject("auditfy", "Auditfy", t0)
		if err := r.projects.Create(ctx, &p); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if _, err := r.projects.GetBySlug(ctx, "auditfy"); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("el rollback debe deshacer el proyecto: %v", err)
	}
}
