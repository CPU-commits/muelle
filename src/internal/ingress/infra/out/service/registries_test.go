package service

import (
	"context"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

func TestRegistriesCredential(t *testing.T) {
	db := sqlitetest.New(t)
	c := newCipher(t)
	ingresstest.Registry(t, db, c, "docker.io", "otro", "otra-clave")
	ingresstest.Registry(t, db, c, "ghcr.io", "bot", "ghp_secreto")
	r := NewRegistriesReader(db, c)
	ctx := context.Background()

	got, err := r.Credential(ctx, "ghcr.io")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || *got != (service.Credential{Host: "ghcr.io", Username: "bot", Password: "ghp_secreto"}) {
		t.Fatalf("credencial: %+v", got)
	}
	if got, err := r.Credential(ctx, "registry.gitlab.com"); err != nil || got != nil {
		t.Fatalf("otro host: %+v %v", got, err)
	}

	// Una contraseña copiada de otra fila (otra AAD) no se descifra, y el
	// error no la trae.
	if _, err := db.Conn(ctx).ExecContext(ctx,
		`UPDATE registries SET password = (SELECT password FROM registries WHERE host = 'docker.io') WHERE host = 'ghcr.io'`); err != nil {
		t.Fatal(err)
	}
	got, err = r.Credential(ctx, "ghcr.io")
	if err == nil || got != nil {
		t.Fatalf("contraseña con otra AAD: %+v %v", got, err)
	}
	if msg := err.Error(); strings.Contains(msg, "otra-clave") || strings.Contains(msg, "ghp_secreto") || !strings.Contains(msg, "ghcr.io") {
		t.Fatalf("el error no puede traer la contraseña y nombra el host: %q", msg)
	}
}
