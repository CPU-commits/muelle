package usecases

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/catalog/app/dto"
	outservice "github.com/CPU-commits/muelle/src/internal/catalog/infra/out/service"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// fakeDocker hace de ingress (StageNetworks) y de deploy (AppRuntime) y
// registra las llamadas. err, si no es nil, es lo que devuelven todas.
type fakeDocker struct {
	mu    sync.Mutex
	calls []string
	err   error
}

func (f *fakeDocker) record(call string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	return f.err
}

func (f *fakeDocker) EnsureStageNetwork(ctx context.Context, project, stage string) error {
	return f.record("ensure " + project + "/" + stage)
}

func (f *fakeDocker) RemoveStageNetwork(ctx context.Context, project, stage string) error {
	return f.record("remove " + project + "/" + stage)
}

func (f *fakeDocker) RemoveApp(ctx context.Context, project, stage, app string) error {
	return f.record("rm " + project + "/" + stage + "/" + app)
}

func (f *fakeDocker) Calls() []string { f.mu.Lock(); defer f.mu.Unlock(); return slices.Clone(f.calls) }

func newEnvWithDocker(t *testing.T) (*env, *fakeDocker) {
	e := newEnv(t)
	f := &fakeDocker{}
	e.d.Networks, e.d.Runtime = f, f
	e.d.Domains = outservice.NewDomainCounter(e.db)
	return e, f
}

// §3.2: crear un stage crea su red (y conecta Caddy). Si Docker no responde,
// el stage no se crea: 503 y se puede reintentar.
func TestCreateStageEnsuresNetwork(t *testing.T) {
	e, f := newEnvWithDocker(t)
	ctx := e.ctx()
	if _, err := NewCreateProject(e.d).Execute(ctx, dto.CreateProjectInput{Slug: "auditfy", Name: "Auditfy"}); err != nil {
		t.Fatal(err)
	}

	f.err = entity.ErrDockerUnavailable
	_, err := NewCreateStage(e.d).Execute(ctx, dto.CreateStageInput{Project: "auditfy", Slug: "staging"})
	wantErr(t, err, entity.ErrDockerUnavailable)
	_, err = NewGetStage(e.d).Execute(ctx, "auditfy", "staging")
	wantErr(t, err, srepo.ErrNotFound)

	f.err = nil
	if _, err := NewCreateStage(e.d).Execute(ctx, dto.CreateStageInput{Project: "auditfy", Slug: "staging"}); err != nil {
		t.Fatal(err)
	}
	// Un stage repetido, un slug inválido o un proyecto que no existe no llegan a Docker.
	_, err = NewCreateStage(e.d).Execute(ctx, dto.CreateStageInput{Project: "auditfy", Slug: "staging"})
	wantErr(t, err, srepo.ErrConflict)
	_, err = NewCreateStage(e.d).Execute(ctx, dto.CreateStageInput{Project: "auditfy", Slug: "Mal"})
	wantInvalid(t, err)
	_, err = NewCreateStage(e.d).Execute(ctx, dto.CreateStageInput{Project: "nope", Slug: "staging"})
	wantErr(t, err, srepo.ErrNotFound)
	if got := f.Calls(); !slices.Equal(got, []string{"ensure auditfy/staging", "ensure auditfy/staging"}) {
		t.Fatalf("llamadas: %v", got)
	}
}

// §3.2: borrar un stage (vacío) desconecta Caddy y borra su red. Con apps no
// llega a Docker; si Docker no responde, el stage queda.
func TestDeleteStageRemovesNetwork(t *testing.T) {
	e, f := newEnvWithDocker(t)
	e.seedStage()
	ctx := e.ctx()
	e.createService("api", "x:1")
	wantErr(t, NewDeleteStage(e.d).Execute(ctx, "auditfy", "staging"), srepo.ErrNotEmpty)
	if err := NewDeleteApp(e.d).Execute(ctx, "auditfy", "staging", "api"); err != nil {
		t.Fatal(err)
	}

	f.err = entity.ErrDockerUnavailable
	wantErr(t, NewDeleteStage(e.d).Execute(ctx, "auditfy", "staging"), entity.ErrDockerUnavailable)
	if _, err := NewGetStage(e.d).Execute(ctx, "auditfy", "staging"); err != nil {
		t.Fatalf("el stage debe seguir: %v", err)
	}
	f.err = nil
	if err := NewDeleteStage(e.d).Execute(ctx, "auditfy", "staging"); err != nil {
		t.Fatal(err)
	}
	want := []string{"ensure auditfy/staging", "rm auditfy/staging/api", "remove auditfy/staging", "remove auditfy/staging"}
	if got := f.Calls(); !slices.Equal(got, want) {
		t.Fatalf("llamadas: %v, esperaba %v", got, want)
	}
}

// Borrar una app ejecuta docker stack rm (deploy) antes de borrar la fila. Si
// Docker no responde, la app queda.
func TestDeleteAppRemovesStack(t *testing.T) {
	e, f := newEnvWithDocker(t)
	e.seedStage()
	ctx := e.ctx()
	e.createService("api", "x:1")

	f.err = entity.ErrDockerUnavailable
	wantErr(t, NewDeleteApp(e.d).Execute(ctx, "auditfy", "staging", "api"), entity.ErrDockerUnavailable)
	if _, err := NewGetApp(e.d).Execute(ctx, "auditfy", "staging", "api"); err != nil {
		t.Fatalf("la app debe seguir: %v", err)
	}
	f.err = nil
	if err := NewDeleteApp(e.d).Execute(ctx, "auditfy", "staging", "api"); err != nil {
		t.Fatal(err)
	}
	_, err := NewGetApp(e.d).Execute(ctx, "auditfy", "staging", "api")
	wantErr(t, err, srepo.ErrNotFound)
	// Una app que no existe no llega a Docker.
	wantErr(t, NewDeleteApp(e.d).Execute(ctx, "auditfy", "staging", "api"), srepo.ErrNotFound)
	if got := f.Calls(); !slices.Equal(got, []string{"ensure auditfy/staging", "rm auditfy/staging/api", "rm auditfy/staging/api"}) {
		t.Fatalf("llamadas: %v", got)
	}
}

// Una app con dominios no se borra (409 not_empty): primero se borran sus
// dominios, que limpian sus registros de Cloudflare (§9). Su stack no se toca.
func TestDeleteAppWithDomainsFails(t *testing.T) {
	e, f := newEnvWithDocker(t)
	e.seedStage()
	ctx := e.ctx()
	app := e.createService("api", "x:1")
	if _, err := e.db.Conn(ctx).ExecContext(ctx, `INSERT INTO domains (app_id, hostname, target_service, target_port, tls_mode, created_at, updated_at)
		VALUES ((SELECT id FROM apps WHERE slug = ?), 'api.auditfy.cl', 'api', 80, 'http', 0, 0)`, app.Slug); err != nil {
		t.Fatal(err)
	}
	wantErr(t, NewDeleteApp(e.d).Execute(ctx, "auditfy", "staging", "api"), srepo.ErrNotEmpty)
	if slices.Contains(f.Calls(), "rm auditfy/staging/api") {
		t.Fatal("no debió tocar el stack")
	}
	// Sin el chequeo (Domains nil), la clave foránea igual lo impide.
	e.d.Domains, e.d.Runtime = nil, nil
	err := NewDeleteApp(e.d).Execute(ctx, "auditfy", "staging", "api")
	if !errors.Is(err, srepo.ErrNotEmpty) {
		t.Fatalf("sin chequeo: %v", err)
	}
}
