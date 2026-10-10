package usecases

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/catalog/app/dto"
	"github.com/CPU-commits/muelle/src/internal/catalog/domain/service"
	catsqlite "github.com/CPU-commits/muelle/src/internal/catalog/infra/out/repository/sqlite"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

type env struct {
	t     *testing.T
	db    *sqlitedb.DB
	clock *clock.Manual
	d     Deps
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := sqlitetest.New(t)
	clk := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	return &env{t: t, db: db, clock: clk, d: Deps{
		Tx:       db,
		Projects: catsqlite.NewProjectRepository(db),
		Stages:   catsqlite.NewStageRepository(db),
		Apps:     catsqlite.NewAppRepository(db),
		Clock:    clk,
		Audit:    sqlitedb.NewAuditRecorder(db, clk, slog.New(slog.NewTextHandler(io.Discard, nil))),
	}}
}

// ctx trae una sesión de usuario, como la de una petición real.
func (e *env) ctx() context.Context {
	return entity.WithPrincipal(context.Background(), entity.Principal{UserID: 1, Username: "ana"})
}

func (e *env) seedStage() {
	e.t.Helper()
	if _, err := NewCreateProject(e.d).Execute(e.ctx(), dto.CreateProjectInput{Slug: "auditfy", Name: "Auditfy"}); err != nil {
		e.t.Fatal(err)
	}
	if _, err := NewCreateStage(e.d).Execute(e.ctx(), dto.CreateStageInput{Project: "auditfy", Slug: "staging"}); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) createService(slug, image string) dto.AppOutput {
	e.t.Helper()
	out, err := NewCreateApp(e.d).Execute(e.ctx(), dto.CreateAppInput{
		Project: "auditfy", Stage: "staging", Slug: slug, Kind: "service",
		Spec: jsontext.Value(`{"image":"` + image + `","port":80}`),
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return out
}

func (e *env) createCompose(slug, yaml string) dto.AppOutput {
	e.t.Helper()
	out, err := NewCreateApp(e.d).Execute(e.ctx(), dto.CreateAppInput{
		Project: "auditfy", Stage: "staging", Slug: slug, Kind: "compose", ComposeYAML: &yaml,
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return out
}

// audit devuelve las acciones registradas, de la más reciente a la más antigua.
func (e *env) audit() []string {
	e.t.Helper()
	rows, err := e.db.Conn(context.Background()).QueryContext(context.Background(),
		`SELECT actor_type || '/' || action FROM audit_events ORDER BY id DESC`)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func wantErr(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("esperaba %v, got %v", target, err)
	}
}

func wantInvalid(t *testing.T, err error) {
	t.Helper()
	if _, ok := errors.AsType[*validate.Error](err); !ok {
		t.Fatalf("esperaba *validate.Error, got %v", err)
	}
}

func TestProjectsUseCases(t *testing.T) {
	e := newEnv(t)
	ctx := e.ctx()

	_, err := NewCreateProject(e.d).Execute(ctx, dto.CreateProjectInput{Slug: "Auditfy", Name: "Auditfy"})
	wantInvalid(t, err)
	_, err = NewCreateProject(e.d).Execute(ctx, dto.CreateProjectInput{Slug: "auditfy", Name: ""})
	wantInvalid(t, err)
	p, err := NewCreateProject(e.d).Execute(ctx, dto.CreateProjectInput{Slug: "auditfy", Name: "Auditfy"})
	if err != nil || p.Slug != "auditfy" || p.CreatedAt.IsZero() {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	_, err = NewCreateProject(e.d).Execute(ctx, dto.CreateProjectInput{Slug: "auditfy", Name: "X"})
	wantErr(t, err, srepo.ErrConflict)
	if _, err := NewCreateProject(e.d).Execute(ctx, dto.CreateProjectInput{Slug: "beta", Name: "Beta"}); err != nil {
		t.Fatal(err)
	}

	list, err := NewListProjects(e.d).Execute(ctx)
	if err != nil || len(list) != 2 || list[0].Slug != "auditfy" {
		t.Fatalf("list=%+v err=%v", list, err)
	}

	p, err = NewUpdateProject(e.d).Execute(ctx, dto.UpdateProjectInput{Slug: "auditfy", Name: "Auditfy SpA"})
	if err != nil || p.Name != "Auditfy SpA" {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	_, err = NewUpdateProject(e.d).Execute(ctx, dto.UpdateProjectInput{Slug: "nope", Name: "x"})
	wantErr(t, err, srepo.ErrNotFound)
	_, err = NewUpdateProject(e.d).Execute(ctx, dto.UpdateProjectInput{Slug: "auditfy", Name: ""})
	wantInvalid(t, err)

	got, err := NewGetProject(e.d).Execute(ctx, "auditfy")
	if err != nil || got.Name != "Auditfy SpA" {
		t.Fatalf("got=%+v err=%v", got, err)
	}

	if err := NewDeleteProject(e.d).Execute(ctx, "beta"); err != nil {
		t.Fatal(err)
	}
	_, err = NewGetProject(e.d).Execute(ctx, "beta")
	wantErr(t, err, srepo.ErrNotFound)
	wantErr(t, NewDeleteProject(e.d).Execute(ctx, "beta"), srepo.ErrNotFound)
}

func TestStagesUseCases(t *testing.T) {
	e := newEnv(t)
	ctx := e.ctx()

	_, err := NewCreateStage(e.d).Execute(ctx, dto.CreateStageInput{Project: "nope", Slug: "staging"})
	wantErr(t, err, srepo.ErrNotFound)
	NewCreateProject(e.d).Execute(ctx, dto.CreateProjectInput{Slug: "auditfy", Name: "Auditfy"})
	NewCreateProject(e.d).Execute(ctx, dto.CreateProjectInput{Slug: "beta", Name: "Beta"})

	_, err = NewCreateStage(e.d).Execute(ctx, dto.CreateStageInput{Project: "auditfy", Slug: "staging_1"})
	wantInvalid(t, err)
	s, err := NewCreateStage(e.d).Execute(ctx, dto.CreateStageInput{Project: "auditfy", Slug: "staging"})
	if err != nil || s.Project != "auditfy" || s.Slug != "staging" {
		t.Fatalf("s=%+v err=%v", s, err)
	}
	_, err = NewCreateStage(e.d).Execute(ctx, dto.CreateStageInput{Project: "auditfy", Slug: "staging"})
	wantErr(t, err, srepo.ErrConflict)
	// El mismo slug en otro proyecto sí se permite.
	if _, err := NewCreateStage(e.d).Execute(ctx, dto.CreateStageInput{Project: "beta", Slug: "staging"}); err != nil {
		t.Fatal(err)
	}
	NewCreateStage(e.d).Execute(ctx, dto.CreateStageInput{Project: "auditfy", Slug: "production"})

	list, err := NewListStages(e.d).Execute(ctx, "auditfy")
	if err != nil || len(list) != 2 || list[0].Slug != "production" {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	_, err = NewListStages(e.d).Execute(ctx, "nope")
	wantErr(t, err, srepo.ErrNotFound)

	got, err := NewGetStage(e.d).Execute(ctx, "auditfy", "staging")
	if err != nil || got.Slug != "staging" {
		t.Fatalf("got=%+v err=%v", got, err)
	}

	if err := NewDeleteStage(e.d).Execute(ctx, "auditfy", "production"); err != nil {
		t.Fatal(err)
	}
	wantErr(t, NewDeleteStage(e.d).Execute(ctx, "auditfy", "production"), srepo.ErrNotFound)
}

func TestDeleteProjectWithStagesFails(t *testing.T) {
	e := newEnv(t)
	e.seedStage()
	wantErr(t, NewDeleteProject(e.d).Execute(e.ctx(), "auditfy"), srepo.ErrNotEmpty)
	// Sin el stage, ya se puede.
	if err := NewDeleteStage(e.d).Execute(e.ctx(), "auditfy", "staging"); err != nil {
		t.Fatal(err)
	}
	if err := NewDeleteProject(e.d).Execute(e.ctx(), "auditfy"); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteStageWithAppsFails(t *testing.T) {
	e := newEnv(t)
	e.seedStage()
	e.createService("api", "x")
	wantErr(t, NewDeleteStage(e.d).Execute(e.ctx(), "auditfy", "staging"), srepo.ErrNotEmpty)
	if err := NewDeleteApp(e.d).Execute(e.ctx(), "auditfy", "staging", "api"); err != nil {
		t.Fatal(err)
	}
	if err := NewDeleteStage(e.d).Execute(e.ctx(), "auditfy", "staging"); err != nil {
		t.Fatal(err)
	}
}

func TestCreateServiceApp(t *testing.T) {
	e := newEnv(t)
	e.seedStage()
	ctx := e.ctx()

	out, err := NewCreateApp(e.d).Execute(ctx, dto.CreateAppInput{
		Project: "auditfy", Stage: "staging", Slug: "api", Kind: "service",
		Spec: jsontext.Value(`{"image":"ghcr.io/aumi-app/backend-app:staging","port":3000,"healthcheck":{"type":"http","path":"/health"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Kind != "service" || out.PendingChanges || out.DeployTimeoutS != 300 || out.LastDeployedAt != nil {
		t.Fatalf("out=%+v", out)
	}
	// El spec guardado viene normalizado (con los valores por defecto).
	if !strings.Contains(string(out.Spec), `"replicas":1`) || !strings.Contains(string(out.Spec), `"interval_s":10`) {
		t.Fatalf("spec no normalizado: %s", out.Spec)
	}

	spec := jsontext.Value(`{"image":"x","port":1}`)
	_, err = NewCreateApp(e.d).Execute(ctx, dto.CreateAppInput{Project: "auditfy", Stage: "staging", Slug: "api", Kind: "service", Spec: spec})
	wantErr(t, err, srepo.ErrConflict)
	_, err = NewCreateApp(e.d).Execute(ctx, dto.CreateAppInput{Project: "auditfy", Stage: "staging", Slug: "bad", Kind: "service",
		Spec: jsontext.Value(`{"image":"x","port":0}`)})
	wantInvalid(t, err)
	_, err = NewCreateApp(e.d).Execute(ctx, dto.CreateAppInput{Project: "auditfy", Stage: "staging", Slug: "bad", Kind: "service"})
	wantInvalid(t, err)
	yaml := "x"
	_, err = NewCreateApp(e.d).Execute(ctx, dto.CreateAppInput{Project: "auditfy", Stage: "staging", Slug: "bad", Kind: "service",
		Spec: spec, ComposeYAML: &yaml})
	wantInvalid(t, err)
	_, err = NewCreateApp(e.d).Execute(ctx, dto.CreateAppInput{Project: "auditfy", Stage: "staging", Slug: "bad", Kind: "lambda"})
	wantInvalid(t, err)
	_, err = NewCreateApp(e.d).Execute(ctx, dto.CreateAppInput{Project: "auditfy", Stage: "nope", Slug: "api", Kind: "service", Spec: spec})
	wantErr(t, err, srepo.ErrNotFound)
}

func TestCreateComposeApp(t *testing.T) {
	e := newEnv(t)
	e.seedStage()
	ctx := e.ctx()
	base := dto.CreateAppInput{Project: "auditfy", Stage: "staging", Slug: "nats", Kind: "compose"}

	in := base
	yaml, timeout := "services:\n  nats:\n    image: nats:2\n", 60
	in.ComposeYAML, in.DeployTimeoutS = &yaml, &timeout
	out, err := NewCreateApp(e.d).Execute(ctx, in)
	if err != nil || out.ComposeYAML != yaml || out.Spec != nil || out.DeployTimeoutS != 60 {
		t.Fatalf("out=%+v err=%v", out, err)
	}

	blank := "  "
	in = base
	in.Slug, in.ComposeYAML = "x", &blank
	_, err = NewCreateApp(e.d).Execute(ctx, in)
	wantInvalid(t, err)
	in = base
	in.Slug = "x"
	_, err = NewCreateApp(e.d).Execute(ctx, in) // sin compose_yaml
	wantInvalid(t, err)

	a1 := "a: 1"
	in = base
	in.Slug, in.ComposeYAML, in.Spec = "x", &a1, jsontext.Value(`{"image":"x"}`)
	_, err = NewCreateApp(e.d).Execute(ctx, in)
	wantInvalid(t, err)
	// spec null equivale a no enviarlo.
	in.Spec = jsontext.Value(`null`)
	if _, err := NewCreateApp(e.d).Execute(ctx, in); err != nil {
		t.Fatal(err)
	}

	five := 5
	in = base
	in.Slug, in.ComposeYAML, in.DeployTimeoutS = "y", &a1, &five
	_, err = NewCreateApp(e.d).Execute(ctx, in)
	wantInvalid(t, err)
}

func TestUpdateApp(t *testing.T) {
	e := newEnv(t)
	e.seedStage()
	ctx := e.ctx()
	api := e.createService("api", "x:1")
	yaml := "a: 1"
	e.createCompose("nats", yaml)

	key := dto.UpdateAppInput{Project: "auditfy", Stage: "staging", Slug: "api"}

	// Cambiar el spec mueve config_changed_at.
	e.clock.Advance(time.Minute)
	in := key
	in.Spec = jsontext.Value(`{"image":"x:2","port":80}`)
	out, err := NewUpdateApp(e.d).Execute(ctx, in)
	if err != nil || !strings.Contains(string(out.Spec), `"x:2"`) || !out.ConfigChangedAt.After(api.ConfigChangedAt) {
		t.Fatalf("out=%+v err=%v", out, err)
	}

	// Cambiar solo el timeout no.
	e.clock.Advance(time.Minute)
	in = key
	timeout := 120
	in.DeployTimeoutS = &timeout
	out2, err := NewUpdateApp(e.d).Execute(ctx, in)
	if err != nil || out2.DeployTimeoutS != 120 || !out2.ConfigChangedAt.Equal(out.ConfigChangedAt) {
		t.Fatalf("out2=%+v err=%v", out2, err)
	}

	// Campo de otro tipo.
	in = key
	in.ComposeYAML = &yaml
	_, err = NewUpdateApp(e.d).Execute(ctx, in)
	wantErr(t, err, service.ErrKindMismatch)
	in = key
	in.Slug = "nats"
	in.Spec = jsontext.Value(`{"image":"x","port":80}`)
	_, err = NewUpdateApp(e.d).Execute(ctx, in)
	wantErr(t, err, service.ErrKindMismatch)

	// Spec inválido, compose vacío y timeout fuera de rango.
	in = key
	in.Spec = jsontext.Value(`{"image":"x","port":-1}`)
	_, err = NewUpdateApp(e.d).Execute(ctx, in)
	wantInvalid(t, err)
	empty := ""
	in = key
	in.Slug, in.ComposeYAML = "nats", &empty
	_, err = NewUpdateApp(e.d).Execute(ctx, in)
	wantInvalid(t, err)
	in = key
	low := 1
	in.DeployTimeoutS = &low
	_, err = NewUpdateApp(e.d).Execute(ctx, in)
	wantInvalid(t, err)

	// Un update que falla no deja cambios a medias.
	got, _ := NewGetApp(e.d).Execute(ctx, "auditfy", "staging", "api")
	if got.DeployTimeoutS != 120 || !strings.Contains(string(got.Spec), `"x:2"`) {
		t.Fatalf("got=%+v", got)
	}

	// Sin campos: no cambia nada y no se audita.
	before := len(e.audit())
	out3, err := NewUpdateApp(e.d).Execute(ctx, key)
	if err != nil || !out3.ConfigChangedAt.Equal(out2.ConfigChangedAt) || len(e.audit()) != before {
		t.Fatalf("out3=%+v err=%v", out3, err)
	}
	_, err = NewUpdateApp(e.d).Execute(ctx, dto.UpdateAppInput{Project: "auditfy", Stage: "staging", Slug: "nope"})
	wantErr(t, err, srepo.ErrNotFound)
}

func TestListGetDeleteApp(t *testing.T) {
	e := newEnv(t)
	e.seedStage()
	ctx := e.ctx()
	e.createService("api", "x:1")
	yaml := "a: 1"
	e.createCompose("nats", yaml)

	list, err := NewListApps(e.d).Execute(ctx, "auditfy", "staging")
	if err != nil || len(list) != 2 || list[0].Slug != "api" {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	_, err = NewListApps(e.d).Execute(ctx, "auditfy", "nope")
	wantErr(t, err, srepo.ErrNotFound)

	if err := NewDeleteApp(e.d).Execute(ctx, "auditfy", "staging", "nats"); err != nil {
		t.Fatal(err)
	}
	_, err = NewGetApp(e.d).Execute(ctx, "auditfy", "staging", "nats")
	wantErr(t, err, srepo.ErrNotFound)
	wantErr(t, NewDeleteApp(e.d).Execute(ctx, "auditfy", "staging", "nats"), srepo.ErrNotFound)
}

func TestPendingChangesAfterDeploy(t *testing.T) {
	e := newEnv(t)
	e.seedStage()
	ctx := e.ctx()
	e.createService("api", "x:1")

	// Simula un deploy de la Fase 2: last_deployed_at = ahora.
	deployed := e.clock.Now().UnixMilli()
	if _, err := e.db.Conn(ctx).ExecContext(ctx, `UPDATE apps SET last_deployed_at = ?`, deployed); err != nil {
		t.Fatal(err)
	}
	got, _ := NewGetApp(e.d).Execute(ctx, "auditfy", "staging", "api")
	if got.PendingChanges || got.LastDeployedAt == nil {
		t.Fatalf("got=%+v", got)
	}

	e.clock.Advance(time.Minute)
	in := dto.UpdateAppInput{Project: "auditfy", Stage: "staging", Slug: "api", Spec: jsontext.Value(`{"image":"x:2","port":80}`)}
	if _, err := NewUpdateApp(e.d).Execute(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, _ = NewGetApp(e.d).Execute(ctx, "auditfy", "staging", "api")
	if !got.PendingChanges {
		t.Fatalf("cambiar el spec después del deploy deja cambios pendientes: %+v", got)
	}
	list, _ := NewListApps(e.d).Execute(ctx, "auditfy", "staging")
	if !list[0].PendingChanges {
		t.Fatalf("ListApps también informa pending_changes: %+v", list[0])
	}
}

func (e *env) auditDetails(action string) string {
	e.t.Helper()
	var d string
	err := e.db.Conn(context.Background()).QueryRowContext(context.Background(),
		`SELECT details FROM audit_events WHERE action = ? ORDER BY id DESC LIMIT 1`, action).Scan(&d)
	if err != nil {
		e.t.Fatal(err)
	}
	return d
}

func TestAuditDetails(t *testing.T) {
	e := newEnv(t)
	e.seedStage()
	ctx := e.ctx()
	e.createService("api", "x")
	yaml := "a: 1"
	e.createCompose("nats", yaml)
	if got := e.auditDetails("app.create"); got != `{"kind":"compose"}` {
		t.Errorf("app.create details = %s", got)
	}

	timeout := 90
	y2 := "b: 2"
	_, err := NewUpdateApp(e.d).Execute(ctx, dto.UpdateAppInput{Project: "auditfy", Stage: "staging", Slug: "nats",
		ComposeYAML: &y2, DeployTimeoutS: &timeout})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.auditDetails("app.update"); got != `{"fields":["compose_yaml","deploy_timeout_s"]}` {
		t.Errorf("app.update details = %s", got)
	}
	_, err = NewUpdateApp(e.d).Execute(ctx, dto.UpdateAppInput{Project: "auditfy", Stage: "staging", Slug: "api",
		Spec: jsontext.Value(`{"image":"y","port":80}`)})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.auditDetails("app.update"); got != `{"fields":["spec"]}` {
		t.Errorf("app.update details = %s", got)
	}
	// Los valores nunca van en los details: solo los nombres de los campos.
	if strings.Contains(e.auditDetails("app.update"), "image") {
		t.Error("details no debe incluir el contenido del spec")
	}
}

func TestUpdateComposeSucceeds(t *testing.T) {
	e := newEnv(t)
	e.seedStage()
	ctx := e.ctx()
	yaml := "a: 1"
	created := e.createCompose("nats", yaml)

	e.clock.Advance(time.Minute)
	next := "services: {}\n"
	out, err := NewUpdateApp(e.d).Execute(ctx, dto.UpdateAppInput{Project: "auditfy", Stage: "staging", Slug: "nats", ComposeYAML: &next})
	if err != nil || out.ComposeYAML != next || !out.ConfigChangedAt.After(created.ConfigChangedAt) {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	got, _ := NewGetApp(e.d).Execute(ctx, "auditfy", "staging", "nats")
	if got.ComposeYAML != next {
		t.Fatalf("got=%+v", got)
	}
}

// Un token limitado a un stage solo ve ese stage en la lista; uno del proyecto los ve todos.
func TestListStagesFiltersStageScopedTokens(t *testing.T) {
	e := newEnv(t)
	e.seedStage()
	NewCreateStage(e.d).Execute(e.ctx(), dto.CreateStageInput{Project: "auditfy", Slug: "production"})

	asToken := func(stage string) context.Context {
		return entity.WithPrincipal(context.Background(), entity.Principal{
			Token: &entity.TokenScope{ID: 1, ProjectSlug: "auditfy", StageSlug: stage}})
	}
	list, err := NewListStages(e.d).Execute(asToken("staging"), "auditfy")
	if err != nil || len(list) != 1 || list[0].Slug != "staging" {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	list, _ = NewListStages(e.d).Execute(asToken(""), "auditfy")
	if len(list) != 2 {
		t.Fatalf("un token de proyecto ve todos: %+v", list)
	}
	list, _ = NewListStages(e.d).Execute(e.ctx(), "auditfy")
	if len(list) != 2 {
		t.Fatalf("una sesión ve todos: %+v", list)
	}
	// Un token de otro stage del que no existe: lista vacía, no nil.
	list, _ = NewListStages(e.d).Execute(asToken("qa"), "auditfy")
	if list == nil || len(list) != 0 {
		t.Fatalf("list=%#v", list)
	}
}

func TestAuditTrail(t *testing.T) {
	e := newEnv(t)
	ctx := e.ctx()
	NewCreateProject(e.d).Execute(ctx, dto.CreateProjectInput{Slug: "auditfy", Name: "Auditfy"})
	NewUpdateProject(e.d).Execute(ctx, dto.UpdateProjectInput{Slug: "auditfy", Name: "Auditfy SpA"})
	NewCreateStage(e.d).Execute(ctx, dto.CreateStageInput{Project: "auditfy", Slug: "staging"})
	e.createService("api", "x")
	NewUpdateApp(e.d).Execute(ctx, dto.UpdateAppInput{Project: "auditfy", Stage: "staging", Slug: "api",
		Spec: jsontext.Value(`{"image":"y","port":80}`)})
	// Los fallos no se auditan.
	NewDeleteProject(e.d).Execute(ctx, "auditfy")
	NewDeleteApp(e.d).Execute(ctx, "auditfy", "staging", "api")
	NewDeleteStage(e.d).Execute(ctx, "auditfy", "staging")
	NewDeleteProject(e.d).Execute(ctx, "auditfy")

	want := []string{
		"user/project.delete", "user/stage.delete", "user/app.delete", "user/app.update",
		"user/app.create", "user/stage.create", "user/project.update", "user/project.create",
	}
	got := e.audit()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}
