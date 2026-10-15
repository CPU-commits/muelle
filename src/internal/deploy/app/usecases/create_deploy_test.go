package usecases

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
)

func (f *fixture) deps() Deps {
	return Deps{Tx: f.db, Apps: f.apps, Deploys: f.deploys, Queue: f.queue, Hub: f.hub, Clock: f.clock, Audit: f.audit}
}

var (
	apiRef   = dto.AppRef{Project: "auditfy", Stage: "staging", App: "api"}
	infraRef = dto.AppRef{Project: "auditfy", Stage: "staging", App: "infra"}
)

func tokenCtx() context.Context {
	return shared.WithPrincipal(context.Background(),
		shared.Principal{Token: &shared.TokenScope{ID: 4, Name: "github-actions", ProjectSlug: "auditfy"}})
}

func countDeploys(t *testing.T, f *fixture) int {
	t.Helper()
	n := 0
	for _, id := range []int64{f.apiID, f.infraID, f.otherID} {
		list, err := f.deploys.List(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		n += len(list)
	}
	return n
}

// Los 422 de contrato §2.2, con sus mensajes exactos; ninguno inserta nada.
func TestCreateDeployValidation(t *testing.T) {
	f := newFixture(t)
	u := NewCreateDeploy(f.deps())
	ctx := sessionCtx()
	for _, c := range []struct {
		in   dto.CreateDeploy
		want string
	}{
		{dto.CreateDeploy{AppRef: apiRef, Image: ptr("ghcr.io/x/api:2"), Images: map[string]string{"api": "ghcr.io/x/api:2"}}, "image: manda image o images, no los dos"},
		{dto.CreateDeploy{AppRef: apiRef, Image: ptr("ghcr.io/x/api:2"), Images: map[string]string{}}, "image: manda image o images, no los dos"},
		{dto.CreateDeploy{AppRef: infraRef, Image: ptr("ghcr.io/x/web:2")}, "image: una app compose recibe images, por servicio"},
		{dto.CreateDeploy{AppRef: apiRef, Image: ptr("-mala")}, "image: no es una referencia de imagen válida"},
		{dto.CreateDeploy{AppRef: apiRef, Image: ptr("")}, "image: es obligatoria y de hasta 512 caracteres"},
		{dto.CreateDeploy{AppRef: apiRef, Image: ptr("Ghcr.io/X/api")}, "image: no es una referencia válida de Docker"},
		{dto.CreateDeploy{AppRef: apiRef, Images: map[string]string{"api": "x$y"}}, "images.api: no es una referencia de imagen válida"},
		{dto.CreateDeploy{AppRef: apiRef, Images: map[string]string{"web": "nginx"}}, "images.web: el servicio no existe en la app"},
		{dto.CreateDeploy{AppRef: infraRef, Images: map[string]string{"db": "postgres"}}, "images.db: el servicio no existe en la app"},
		{dto.CreateDeploy{AppRef: infraRef, Images: map[string]string{"web": "nginx", "api": "x"}}, "images.api: el servicio no existe en la app"},
		{dto.CreateDeploy{AppRef: infraRef, Images: map[string]string{"web": ""}}, "images.web: es obligatoria y de hasta 512 caracteres"},
	} {
		_, err := u.Execute(ctx, c.in)
		if msg := validationMessage(t, err); !strings.HasPrefix(msg, c.want) {
			t.Errorf("%+v: %q, esperaba %q", c.in, msg, c.want)
		}
	}
	if n := countDeploys(t, f); n != 0 {
		t.Fatalf("la validación insertó %d deploys", n)
	}
	if _, err := u.Execute(ctx, dto.CreateDeploy{AppRef: dto.AppRef{Project: "auditfy", Stage: "staging", App: "nada"}}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("app que no existe: %v", err)
	}
	if err := u.CheckApp(ctx, dto.AppRef{Project: "auditfy", Stage: "staging", App: "nada"}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("CheckApp de una app que no existe: %v", err)
	}
	if err := u.CheckApp(ctx, apiRef); err != nil {
		t.Fatalf("CheckApp: %v", err)
	}

	// Lo que pasa: los overrides quedan en images mientras está queued.
	for _, c := range []struct {
		in   dto.CreateDeploy
		want map[string]string
	}{
		{dto.CreateDeploy{AppRef: apiRef}, map[string]string{}},
		{dto.CreateDeploy{AppRef: apiRef, Images: map[string]string{}}, map[string]string{}},
		{dto.CreateDeploy{AppRef: apiRef, Image: ptr("ghcr.io/x/api:sha-3f9c2e1")}, map[string]string{"api": "ghcr.io/x/api:sha-3f9c2e1"}},
		{dto.CreateDeploy{AppRef: apiRef, Images: map[string]string{"api": "ghcr.io/x/api@sha256:" + strings.Repeat("a", 64)}}, map[string]string{"api": "ghcr.io/x/api@sha256:" + strings.Repeat("a", 64)}},
		{dto.CreateDeploy{AppRef: infraRef, Images: map[string]string{"web": "ghcr.io/x/web:2"}}, map[string]string{"web": "ghcr.io/x/web:2"}},
	} {
		q, err := u.Execute(ctx, c.in)
		if err != nil {
			t.Fatalf("%+v: %v", c.in, err)
		}
		d, err := f.deploys.GetByID(context.Background(), q.ID)
		if err != nil || !reflect.DeepEqual(d.Images, c.want) || d.Status != entity.StatusQueued {
			t.Fatalf("%+v: %+v, %v", c.in, d, err)
		}
	}
}

// Encolar deja superseded los queued de la app (contrato §2.2) y les avisa a
// sus streams después del commit; el running no se toca.
func TestCreateDeploySupersedes(t *testing.T) {
	f := newFixture(t)
	u := NewCreateDeploy(f.deps())
	ctx := sessionCtx()

	running, err := u.Execute(ctx, dto.CreateDeploy{AppRef: apiRef})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := f.deploys.Claim(ctx, running.ID, t0); err != nil || !ok {
		t.Fatal(ok, err)
	}
	old, err := u.Execute(ctx, dto.CreateDeploy{AppRef: apiRef})
	if err != nil {
		t.Fatal(err)
	}
	other, err := u.Execute(ctx, dto.CreateDeploy{AppRef: infraRef})
	if err != nil {
		t.Fatal(err)
	}
	waitOld, waitNew := f.hub.Wait(old.ID), f.hub.Wait(other.ID+1)
	waitRunning, waitOther := f.hub.Wait(running.ID), f.hub.Wait(other.ID)

	q, err := u.Execute(ctx, dto.CreateDeploy{AppRef: apiRef})
	if err != nil {
		t.Fatal(err)
	}
	want := dto.Queued{ID: other.ID + 1, Status: entity.StatusQueued,
		StatusURL: fmt.Sprintf("/api/v1/projects/auditfy/stages/staging/apps/api/deploys/%d", other.ID+1)}
	if q != want {
		t.Fatalf("Execute = %+v, esperaba %+v", q, want)
	}
	d, err := f.deploys.GetByID(ctx, old.ID)
	if err != nil || d.Status != entity.StatusSuperseded || d.SupersededBy == nil || *d.SupersededBy != q.ID || d.FinishedAt == nil {
		t.Fatalf("el viejo: %+v, %v", d, err)
	}
	if d, _ := f.deploys.GetByID(ctx, running.ID); d.Status != entity.StatusRunning {
		t.Fatalf("el running: %+v", d)
	}
	if d, _ := f.deploys.GetByID(ctx, other.ID); d.Status != entity.StatusQueued {
		t.Fatalf("el de otra app: %+v", d)
	}
	if !closed(waitOld) {
		t.Fatal("el stream del reemplazado no se enteró")
	}
	if closed(waitNew) || closed(waitRunning) || closed(waitOther) {
		t.Fatal("avisó a un deploy que no cambió")
	}
	if got := f.queue.wakes(); !slices.Equal(got, []int64{f.apiID, f.apiID, f.infraID, f.apiID}) {
		t.Fatalf("Wake: %v", got)
	}
}

// El actor sale del principal: una sesión es panel/user, un token es api/token
// (contrato §2.1); el nombre se copia al crear el deploy.
func TestCreateDeployActor(t *testing.T) {
	f := newFixture(t)
	u := NewCreateDeploy(f.deps())
	for _, c := range []struct {
		ctx                 context.Context
		trigger             entity.Trigger
		actorType, actorNam string
		actorID             int64
	}{
		{sessionCtx(), entity.TriggerPanel, entity.ActorUser, "felipe", 1},
		{tokenCtx(), entity.TriggerAPI, entity.ActorToken, "github-actions", 4},
	} {
		q, err := u.Execute(c.ctx, dto.CreateDeploy{AppRef: apiRef})
		if err != nil {
			t.Fatal(err)
		}
		d, _ := f.deploys.GetByID(context.Background(), q.ID)
		if d.Trigger != c.trigger || d.ActorType != c.actorType || d.ActorName != c.actorNam || d.ActorID == nil || *d.ActorID != c.actorID {
			t.Fatalf("%+v", d)
		}
	}
	// Sin principal no hay a quién atribuirlo: es un error interno, no un deploy de nadie.
	if _, err := u.Execute(context.Background(), dto.CreateDeploy{AppRef: apiRef}); err == nil {
		t.Fatal("un deploy sin principal")
	}
}

func TestCreateDeployAudit(t *testing.T) {
	f := newFixture(t)
	u := NewCreateDeploy(f.deps())
	q, err := u.Execute(tokenCtx(), dto.CreateDeploy{AppRef: infraRef, Images: map[string]string{"web": "ghcr.io/x/web:2"}})
	if err != nil {
		t.Fatal(err)
	}
	q2, err := u.Execute(sessionCtx(), dto.CreateDeploy{AppRef: apiRef})
	if err != nil {
		t.Fatal(err)
	}
	events := auditEvents(t, f.db)
	if len(events) != 2 {
		t.Fatalf("eventos: %+v", events)
	}
	e := events[0]
	if e.ActorType != "token" || e.ActorID == nil || *e.ActorID != 4 || e.Action != "deploy.create" || e.Target != "app:auditfy/staging/infra" ||
		e.Details != fmt.Sprintf(`{"deploy":%d,"images":{"web":"ghcr.io/x/web:2"},"trigger":"api"}`, q.ID) {
		t.Fatalf("evento: %+v", e)
	}
	if e := events[1]; e.Details != fmt.Sprintf(`{"deploy":%d,"images":{},"trigger":"panel"}`, q2.ID) {
		t.Fatalf("evento sin overrides: %+v", e)
	}
}

// Mientras la app se está borrando, ni deploys ni rollbacks (409 conflict).
func TestCreateDeployRejectedWhileRemoving(t *testing.T) {
	f := newFixture(t)
	f.queue.setRemoving(f.apiID)
	if _, err := NewCreateDeploy(f.deps()).Execute(sessionCtx(), dto.CreateDeploy{AppRef: apiRef}); !errors.Is(err, service.ErrAppRemoving) {
		t.Fatalf("deploy: %v", err)
	}
	if _, err := NewRollback(f.deps()).Execute(sessionCtx(), apiRef); !errors.Is(err, service.ErrAppRemoving) {
		t.Fatalf("rollback: %v", err)
	}
	if n := countDeploys(t, f); n != 0 || len(f.queue.wakes()) != 0 {
		t.Fatalf("insertó %d deploys, Wake %v", n, f.queue.wakes())
	}
	// Las otras apps siguen.
	if _, err := NewCreateDeploy(f.deps()).Execute(sessionCtx(), dto.CreateDeploy{AppRef: infraRef}); err != nil {
		t.Fatal(err)
	}
}

// succeed lleva un deploy de la app a succeeded con esas imágenes resueltas.
func (f *fixture) succeed(t *testing.T, appID int64, images map[string]string) entity.Deploy {
	t.Helper()
	ctx := context.Background()
	d := entity.Deploy{AppID: appID, Trigger: entity.TriggerAPI, ActorType: entity.ActorToken, ActorName: "ci", CreatedAt: t0}
	if err := f.deploys.Create(ctx, &d); err != nil {
		t.Fatal(err)
	}
	if ok, err := f.deploys.Claim(ctx, d.ID, t0); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if err := f.deploys.SetImages(ctx, d.ID, images, images, images); err != nil {
		t.Fatal(err)
	}
	if err := f.deploys.Finish(ctx, d.ID, entity.StatusSucceeded, nil, t0); err != nil {
		t.Fatal(err)
	}
	d, _ = f.deploys.GetByID(ctx, d.ID)
	return d
}

// El rollback (contrato §2.7): las imágenes, con digest, del succeeded
// anterior al actual; sin los dos, 409 no_previous_deploy.
func TestRollback(t *testing.T) {
	f := newFixture(t)
	u := NewRollback(f.deps())
	ctx := sessionCtx()
	digest := func(c string) string { return "@sha256:" + strings.Repeat(c, 64) }

	if _, err := u.Execute(ctx, apiRef); !errors.Is(err, service.ErrNoPreviousDeploy) {
		t.Fatalf("sin deploys: %v", err)
	}
	d1 := f.succeed(t, f.apiID, map[string]string{"api": "ghcr.io/x/api:staging" + digest("a")})
	if _, err := u.Execute(ctx, apiRef); !errors.Is(err, service.ErrNoPreviousDeploy) {
		t.Fatalf("con uno solo: %v", err)
	}
	// Un failed después del actual no cambia el destino.
	f.succeed(t, f.apiID, map[string]string{"api": "ghcr.io/x/api:staging" + digest("b")})
	failed := entity.Deploy{AppID: f.apiID, Trigger: entity.TriggerAPI, ActorType: entity.ActorToken, ActorName: "ci", CreatedAt: t0}
	if err := f.deploys.Create(ctx, &failed); err != nil {
		t.Fatal(err)
	}
	if err := f.deploys.Finish(ctx, failed.ID, entity.StatusFailed, ptr("x"), t0); err != nil {
		t.Fatal(err)
	}
	queued, err := NewCreateDeploy(f.deps()).Execute(ctx, dto.CreateDeploy{AppRef: apiRef})
	if err != nil {
		t.Fatal(err)
	}
	wait := f.hub.Wait(queued.ID)
	if err := u.CheckApp(ctx, apiRef); err != nil {
		t.Fatal(err)
	}

	q, err := u.Execute(ctx, apiRef)
	if err != nil {
		t.Fatal(err)
	}
	if q.Status != entity.StatusQueued || !strings.HasSuffix(q.StatusURL, fmt.Sprintf("/apps/api/deploys/%d", q.ID)) {
		t.Fatalf("Execute: %+v", q)
	}
	r, _ := f.deploys.GetByID(ctx, q.ID)
	if r.Trigger != entity.TriggerRollback || !maps.Equal(r.Images, d1.Images) || r.ActorName != "felipe" || r.ActorType != entity.ActorUser {
		t.Fatalf("el rollback: %+v", r)
	}
	if d, _ := f.deploys.GetByID(ctx, queued.ID); d.Status != entity.StatusSuperseded || *d.SupersededBy != q.ID || !closed(wait) {
		t.Fatalf("el queued que reemplazó: %+v", d)
	}
	events := auditEvents(t, f.db)
	last := events[len(events)-1]
	if last.Action != "deploy.create" || last.Target != "app:auditfy/staging/api" ||
		last.Details != fmt.Sprintf(`{"deploy":%d,"images":{"api":"ghcr.io/x/api:staging%s"},"trigger":"rollback"}`, q.ID, digest("a")) {
		t.Fatalf("auditoría: %+v", last)
	}
	if w := f.queue.wakes(); w[len(w)-1] != f.apiID {
		t.Fatalf("Wake: %v", w)
	}
	if err := u.CheckApp(ctx, dto.AppRef{Project: "auditfy", Stage: "staging", App: "nada"}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("CheckApp: %v", err)
	}
}

func TestListAndGetDeploys(t *testing.T) {
	f := newFixture(t)
	create := NewCreateDeploy(f.deps())
	ctx := tokenCtx()
	list, err := NewListDeploys(f.deps()).Execute(ctx, apiRef)
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("sin deploys: %#v, %v", list, err)
	}
	a, _ := create.Execute(ctx, dto.CreateDeploy{AppRef: apiRef})
	b, _ := create.Execute(ctx, dto.CreateDeploy{AppRef: apiRef})
	x, _ := create.Execute(ctx, dto.CreateDeploy{AppRef: infraRef})
	list, err = NewListDeploys(f.deps()).Execute(ctx, apiRef)
	if err != nil || len(list) != 2 || list[0].ID != b.ID || list[1].ID != a.ID {
		t.Fatalf("List: %+v, %v", list, err)
	}
	get := NewGetDeploy(f.deps())
	if d, err := get.Execute(ctx, dto.DeployRef{AppRef: apiRef, ID: a.ID}); err != nil || d.ID != a.ID {
		t.Fatalf("Get: %+v, %v", d, err)
	}
	if _, err := get.Execute(ctx, dto.DeployRef{AppRef: apiRef, ID: x.ID}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("un deploy de otra app: %v", err)
	}
	nada := dto.AppRef{Project: "auditfy", Stage: "staging", App: "nada"}
	if _, err := get.Execute(ctx, dto.DeployRef{AppRef: nada, ID: a.ID}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("app que no existe: %v", err)
	}
	if _, err := NewListDeploys(f.deps()).Execute(ctx, nada); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("List de una app que no existe: %v", err)
	}
}

func TestReadLogUseCase(t *testing.T) {
	f := newFixture(t)
	ctx := tokenCtx()
	q, err := NewCreateDeploy(f.deps()).Execute(ctx, dto.CreateDeploy{AppRef: apiRef})
	if err != nil {
		t.Fatal(err)
	}
	l := NewLogWriter(f.deploys, f.hub, f.clock).For(q.ID, service.Masker{})
	if err := l.Line("resolviendo imágenes"); err != nil {
		t.Fatal(err)
	}
	u := NewReadLog(f.deps())
	ref := dto.DeployRef{AppRef: apiRef, ID: q.ID}
	page, err := u.Execute(ctx, dto.ReadLog{DeployRef: ref})
	want := "2026-10-10T14:00:00.000Z resolviendo imágenes\n"
	if err != nil || page.Chunk != want || page.NextOffset != len(want) || page.Status != entity.StatusQueued || page.SupersededBy != nil {
		t.Fatalf("ReadLog: %+v, %v", page, err)
	}
	page, err = u.Execute(ctx, dto.ReadLog{DeployRef: ref, Offset: len(want)})
	if err != nil || page.Chunk != "" || page.NextOffset != len(want) {
		t.Fatalf("al final: %+v, %v", page, err)
	}
	_, err = u.Execute(ctx, dto.ReadLog{DeployRef: ref, Offset: len(want) + 1})
	if msg := validationMessage(t, err); msg != "offset: pasa el largo del log" {
		t.Fatalf("offset más allá: %q", msg)
	}
	_, err = u.Execute(ctx, dto.ReadLog{DeployRef: ref, Offset: -1})
	if msg := validationMessage(t, err); msg != "offset: tiene que ser un entero ≥ 0" {
		t.Fatalf("offset negativo: %q", msg)
	}
	if _, err := u.Execute(ctx, dto.ReadLog{DeployRef: dto.DeployRef{AppRef: infraRef, ID: q.ID}}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("de otra app: %v", err)
	}
	// El 404 gana al 422 del offset.
	if _, err := u.Execute(ctx, dto.ReadLog{DeployRef: dto.DeployRef{AppRef: infraRef, ID: q.ID}, Offset: -1}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("de otra app con un offset inválido: %v", err)
	}
}
