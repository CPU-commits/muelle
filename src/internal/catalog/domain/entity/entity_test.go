package entity

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/catalog/domain/vo"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func wantInvalid(t *testing.T, err error, field string) {
	t.Helper()
	ve, ok := errors.AsType[*validate.Error](err)
	if !ok {
		t.Fatalf("esperaba *validate.Error, got %v", err)
	}
	if ve.Field != field {
		t.Fatalf("field = %q, esperaba %q", ve.Field, field)
	}
}

func TestNewProject(t *testing.T) {
	p, err := NewProject("auditfy", "Auditfy", t0)
	if err != nil || p.Slug != "auditfy" || p.Name != "Auditfy" || !p.CreatedAt.Equal(t0) {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	for _, slug := range []string{"Auditfy", "muelle", "docker", "", "a_b"} {
		if _, err := NewProject(slug, "X", t0); err == nil {
			t.Errorf("slug %q debería rechazarse", slug)
		}
	}
	_, err = NewProject("auditfy", "", t0)
	wantInvalid(t, err, "name")
	_, err = NewProject("auditfy", strings.Repeat("a", 65), t0)
	wantInvalid(t, err, "name")

	if err := p.Rename(" "); err == nil {
		t.Error("Rename con nombre vacío debe fallar")
	}
	if err := p.Rename("Otro"); err != nil || p.Name != "Otro" {
		t.Errorf("p=%+v err=%v", p, err)
	}
}

func TestNewStage(t *testing.T) {
	p := Project{ID: 7, Slug: "auditfy"}
	s, err := NewStage(p, "staging", t0)
	if err != nil || s.ProjectID != 7 || s.ProjectSlug != "auditfy" || s.Slug != "staging" {
		t.Fatalf("s=%+v err=%v", s, err)
	}
	for _, slug := range []string{"staging_1", "Staging", "", "a-very-long-stage"} {
		if _, err := NewStage(p, slug, t0); err == nil {
			t.Errorf("slug %q debería rechazarse", slug)
		}
	}
}

func specOf(t *testing.T, image string) vo.AppSpec {
	t.Helper()
	s, err := vo.ParseAppSpec([]byte(`{"image":"` + image + `","port":80}`))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNewApps(t *testing.T) {
	st := Stage{ID: 3, ProjectSlug: "auditfy", Slug: "staging"}
	spec := specOf(t, "x")

	a, err := NewServiceApp(st, "api", spec, 0, t0)
	if err != nil {
		t.Fatal(err)
	}
	if a.Kind != KindService || a.StageID != 3 || a.DeployTimeoutS != DefaultDeployTimeoutS ||
		a.ProjectSlug != "auditfy" || a.StageSlug != "staging" ||
		!a.ConfigChangedAt.Equal(t0) || a.LastDeployedAt != nil || string(a.Spec) != string(spec.JSON()) {
		t.Fatalf("a=%+v", a)
	}

	c, err := NewComposeApp(st, "nats", "services: {}\n", 60, t0)
	if err != nil || c.Kind != KindCompose || c.DeployTimeoutS != 60 || c.Spec != nil {
		t.Fatalf("c=%+v err=%v", c, err)
	}

	_, err = NewServiceApp(st, "Bad", spec, 0, t0)
	wantInvalid(t, err, "slug")
	_, err = NewServiceApp(st, "api", spec, 5, t0)
	wantInvalid(t, err, "deploy_timeout_s")
	_, err = NewServiceApp(st, "api", spec, 3601, t0)
	wantInvalid(t, err, "deploy_timeout_s")
	// Un spec sin validar (cero) no llega a la base.
	_, err = NewServiceApp(st, "api", vo.AppSpec{}, 0, t0)
	wantInvalid(t, err, "image")
	_, err = NewComposeApp(st, "x", "  ", 0, t0)
	wantInvalid(t, err, "compose_yaml")
	_, err = NewComposeApp(st, "x", strings.Repeat("a", MaxComposeBytes+1), 0, t0)
	wantInvalid(t, err, "compose_yaml")
	if _, err := NewComposeApp(st, "x", strings.Repeat("a", MaxComposeBytes), 0, t0); err != nil {
		t.Errorf("256 KiB exactos deben aceptarse: %v", err)
	}
}

func TestAppChanges(t *testing.T) {
	st := Stage{ID: 3, ProjectSlug: "p", Slug: "s"}
	a, _ := NewServiceApp(st, "api", specOf(t, "x"), 0, t0)
	t1 := t0.Add(time.Minute)

	y := specOf(t, "y")
	if err := a.ChangeSpec(y, t1); err != nil || string(a.Spec) != string(y.JSON()) || !a.ConfigChangedAt.Equal(t1) {
		t.Fatalf("a=%+v err=%v", a, err)
	}
	// Un spec sin validar no se aplica.
	wantInvalid(t, a.ChangeSpec(vo.AppSpec{}, t1.Add(time.Hour)), "image")
	if string(a.Spec) != string(y.JSON()) || !a.ConfigChangedAt.Equal(t1) {
		t.Fatalf("un cambio inválido no debe aplicarse: %+v", a)
	}
	// El timeout no cuenta como cambio de configuración.
	if err := a.SetDeployTimeout(120); err != nil || a.DeployTimeoutS != 120 || !a.ConfigChangedAt.Equal(t1) {
		t.Fatalf("a=%+v err=%v", a, err)
	}
	wantInvalid(t, a.SetDeployTimeout(1), "deploy_timeout_s")

	c, _ := NewComposeApp(st, "nats", "a: 1", 0, t0)
	wantInvalid(t, c.ChangeCompose("", t1), "compose_yaml")
	if c.ComposeYAML != "a: 1" || !c.ConfigChangedAt.Equal(t0) {
		t.Fatalf("un cambio inválido no debe aplicarse: %+v", c)
	}
	if err := c.ChangeCompose("b: 2", t1); err != nil || c.ComposeYAML != "b: 2" || !c.ConfigChangedAt.Equal(t1) {
		t.Fatalf("c=%+v err=%v", c, err)
	}
}

func TestChangesEnforceKind(t *testing.T) {
	st := Stage{ID: 3, ProjectSlug: "p", Slug: "s"}
	svc, _ := NewServiceApp(st, "api", specOf(t, "x"), 0, t0)
	cmp, _ := NewComposeApp(st, "nats", "a: 1", 0, t0)
	t1 := t0.Add(time.Minute)

	if err := svc.ChangeCompose("b: 2", t1); !errors.Is(err, ErrWrongKind) {
		t.Fatalf("esperaba ErrWrongKind, got %v", err)
	}
	if svc.ComposeYAML != "" || !svc.ConfigChangedAt.Equal(t0) {
		t.Fatalf("no debe cambiar nada: %+v", svc)
	}
	if err := cmp.ChangeSpec(specOf(t, "y"), t1); !errors.Is(err, ErrWrongKind) {
		t.Fatalf("esperaba ErrWrongKind, got %v", err)
	}
	if cmp.Spec != nil || !cmp.ConfigChangedAt.Equal(t0) {
		t.Fatalf("no debe cambiar nada: %+v", cmp)
	}
}

func TestPendingChanges(t *testing.T) {
	deployed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := App{ConfigChangedAt: deployed.Add(-time.Second)}
	if a.PendingChanges() {
		t.Error("sin deploy no hay cambios pendientes")
	}
	a.LastDeployedAt = &deployed
	if a.PendingChanges() {
		t.Error("config anterior al deploy no es pendiente")
	}
	a.ConfigChangedAt = deployed.Add(time.Millisecond)
	if !a.PendingChanges() {
		t.Error("config posterior al deploy es pendiente")
	}
}
