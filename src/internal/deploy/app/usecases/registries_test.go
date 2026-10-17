package usecases

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
)

func (f *fixture) registries() (*ListRegistries, *CreateRegistry, *UpdateRegistry, *DeleteRegistry) {
	return NewListRegistries(f.regs),
		NewCreateRegistry(f.db, f.regs, f.clock, f.audit),
		NewUpdateRegistry(f.db, f.regs, f.clock, f.audit),
		NewDeleteRegistry(f.db, f.regs, f.audit)
}

func sessionCtx() context.Context {
	return entity.WithPrincipal(context.Background(), entity.Principal{UserID: 1, Username: "felipe"})
}

func TestRegistryCRUD(t *testing.T) {
	f := newFixture(t)
	list, create, update, del := f.registries()
	ctx := sessionCtx()

	got, err := create.Execute(ctx, dto.RegistryInput{Host: "ghcr.io", Username: "bot", Password: "s3cr3t-pass"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Host != "ghcr.io" || got.Username != "bot" || got.Password != "" || !got.CreatedAt.Equal(t0) || !got.UpdatedAt.Equal(t0) {
		t.Fatalf("Create: %+v", got)
	}
	if _, err := create.Execute(ctx, dto.RegistryInput{Host: "docker.io", Username: "yo", Password: "otra-pass"}); err != nil {
		t.Fatal(err)
	}
	if _, err := create.Execute(ctx, dto.RegistryInput{Host: "ghcr.io", Username: "x", Password: "y"}); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("host repetido: %v", err)
	}

	regs, err := list.Execute(ctx)
	if err != nil || len(regs) != 2 || regs[0].Host != "docker.io" || regs[1].Host != "ghcr.io" || regs[0].Password != "" {
		t.Fatalf("List: %+v, %v", regs, err)
	}

	f.clock.Advance(time.Hour)
	got, err = update.Execute(ctx, dto.RegistryInput{Host: "ghcr.io", Username: "bot2", Password: "nueva-pass"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "bot2" || !got.CreatedAt.Equal(t0) || !got.UpdatedAt.Equal(t0.Add(time.Hour)) || got.Password != "" {
		t.Fatalf("Update: %+v", got)
	}
	creds, err := f.regs.Credentials(ctx, []string{"ghcr.io"})
	if err != nil || creds["ghcr.io"].Password != "nueva-pass" {
		t.Fatalf("la contraseña nueva: %v, %v", creds, err)
	}
	if _, err := update.Execute(ctx, dto.RegistryInput{Host: "quay.io", Username: "x", Password: "y"}); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Update de uno que no existe: %v", err)
	}

	if err := del.Execute(ctx, "ghcr.io"); err != nil {
		t.Fatal(err)
	}
	if err := del.Execute(ctx, "ghcr.io"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Delete otra vez: %v", err)
	}
}

// Los 422 de contrato §2.10, con el campo al principio; ninguno trae el valor.
func TestRegistryValidation(t *testing.T) {
	f := newFixture(t)
	_, create, update, _ := f.registries()
	ctx := sessionCtx()
	if _, err := create.Execute(ctx, dto.RegistryInput{Host: "ghcr.io", Username: "bot", Password: "ok-pass"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		in   dto.RegistryInput
		want string
	}{
		{dto.RegistryInput{Host: "", Username: "bot", Password: "s3cr3t-pass"}, "host: "},
		{dto.RegistryInput{Host: "GHCR.io", Username: "bot", Password: "s3cr3t-pass"}, "host: no es un host de registry válido"},
		{dto.RegistryInput{Host: "https://ghcr.io", Username: "bot", Password: "s3cr3t-pass"}, "host: "},
		{dto.RegistryInput{Host: "index.docker.io", Username: "bot", Password: "s3cr3t-pass"}, "host: para Docker Hub usa docker.io"},
		{dto.RegistryInput{Host: "quay.io", Username: "", Password: "s3cr3t-pass"}, "username: "},
		{dto.RegistryInput{Host: "quay.io", Username: "a:b", Password: "s3cr3t-pass"}, "username: no puede contener ':'"},
		{dto.RegistryInput{Host: "quay.io", Username: "bot", Password: ""}, "password: "},
		{dto.RegistryInput{Host: "quay.io", Username: "bot", Password: "sec\nreto-largo"}, "password: no puede contener saltos de línea"},
	} {
		_, err := create.Execute(ctx, c.in)
		msg := validationMessage(t, err)
		if !strings.HasPrefix(msg, c.want) {
			t.Errorf("Create(%v): %q, esperaba que empiece con %q", c.in, msg, c.want)
		}
		if c.in.Password != "" && strings.Contains(msg, c.in.Password) {
			t.Errorf("el mensaje trae la contraseña: %q", msg)
		}
	}
	// El PATCH valida usuario y contraseña (el host sale de la ruta).
	if _, err := update.Execute(ctx, dto.RegistryInput{Host: "ghcr.io", Username: "", Password: "p"}); !strings.HasPrefix(validationMessage(t, err), "username: ") {
		t.Errorf("Update sin usuario: %v", err)
	}
	if _, err := update.Execute(ctx, dto.RegistryInput{Host: "ghcr.io", Username: "bot", Password: ""}); !strings.HasPrefix(validationMessage(t, err), "password: ") {
		t.Errorf("Update sin contraseña: %v", err)
	}
}

// La auditoría (contrato §2.10): registry.create, update y delete con target
// registry:<host>; details con el usuario, nunca la contraseña.
func TestRegistryAuditNoPassword(t *testing.T) {
	f := newFixture(t)
	_, create, update, del := f.registries()
	ctx := sessionCtx()
	if _, err := create.Execute(ctx, dto.RegistryInput{Host: "ghcr.io", Username: "bot", Password: "s3cr3t-uno"}); err != nil {
		t.Fatal(err)
	}
	if _, err := update.Execute(ctx, dto.RegistryInput{Host: "ghcr.io", Username: "bot2", Password: "s3cr3t-dos"}); err != nil {
		t.Fatal(err)
	}
	if err := del.Execute(ctx, "ghcr.io"); err != nil {
		t.Fatal(err)
	}
	events := auditEvents(t, f.db)
	want := []auditEvent{
		{ActorType: "user", ActorID: ptr(int64(1)), ActorName: "felipe", Action: "registry.create", Target: "registry:ghcr.io", Details: `{"username":"bot"}`},
		{ActorType: "user", ActorID: ptr(int64(1)), ActorName: "felipe", Action: "registry.update", Target: "registry:ghcr.io", Details: `{"username":"bot2"}`},
		{ActorType: "user", ActorID: ptr(int64(1)), ActorName: "felipe", Action: "registry.delete", Target: "registry:ghcr.io", Details: `{}`},
	}
	if len(events) != len(want) {
		t.Fatalf("eventos: %+v", events)
	}
	for i, e := range events {
		w := want[i]
		if e.ActorType != w.ActorType || e.ActorID == nil || *e.ActorID != *w.ActorID || e.ActorName != w.ActorName ||
			e.Action != w.Action || e.Target != w.Target || e.Details != w.Details {
			t.Errorf("evento %d: %+v, esperaba %+v", i, e, w)
		}
		if strings.Contains(e.Details, "s3cr3t") {
			t.Errorf("la auditoría trae la contraseña: %s", e.Details)
		}
	}
	// Un error (409) no deja evento.
	if _, err := create.Execute(ctx, dto.RegistryInput{Host: "docker.io", Username: "x", Password: "y"}); err != nil {
		t.Fatal(err)
	}
	if _, err := create.Execute(ctx, dto.RegistryInput{Host: "docker.io", Username: "x", Password: "y"}); !errors.Is(err, repository.ErrConflict) {
		t.Fatal(err)
	}
	if n := len(auditEvents(t, f.db)); n != 4 {
		t.Fatalf("hay %d eventos", n)
	}
}
