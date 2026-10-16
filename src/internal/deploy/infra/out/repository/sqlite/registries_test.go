package sqlite

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

func testCipher(t *testing.T) *crypto.Cipher {
	t.Helper()
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRegistriesRoundTrip(t *testing.T) {
	db := sqlitetest.New(t)
	ciph := testCipher(t)
	r := NewRegistryRepository(db, ciph)
	ctx := context.Background()

	if list, err := r.List(ctx); err != nil || list == nil || len(list) != 0 {
		t.Fatalf("List vacía: %#v, %v", list, err)
	}

	ghcr := entity.Registry{Host: "ghcr.io", Username: "bot", Password: "s3cr3t-ghcr", CreatedAt: t0, UpdatedAt: t0}
	if err := r.Create(ctx, &ghcr); err != nil || ghcr.ID == 0 {
		t.Fatalf("Create: %+v, %v", ghcr, err)
	}
	local := entity.Registry{Host: "registry.example.com:5000", Username: "ci", Password: "s3cr3t-local", CreatedAt: t0, UpdatedAt: t0}
	if err := r.Create(ctx, &local); err != nil {
		t.Fatal(err)
	}
	hub := entity.Registry{Host: "docker.io", Username: "yo", Password: "s3cr3t-hub", CreatedAt: t0, UpdatedAt: t0}
	if err := r.Create(ctx, &hub); err != nil {
		t.Fatal(err)
	}

	// La columna guarda el cifrado, con la AAD de su fila.
	var stored []byte
	if err := db.Conn(ctx).QueryRowContext(ctx, `SELECT password FROM registries WHERE id = ?`, ghcr.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte("s3cr3t-ghcr")) || len(stored) == 0 {
		t.Fatalf("la columna no está cifrada: %q", stored)
	}
	if pt, err := ciph.Decrypt(stored, entity.PasswordAAD(ghcr.ID)); err != nil || string(pt) != "s3cr3t-ghcr" {
		t.Fatalf("descifrar con su AAD: %q, %v", pt, err)
	}
	if _, err := ciph.Decrypt(stored, entity.PasswordAAD(local.ID)); err == nil {
		t.Fatal("se descifró con la AAD de otra fila")
	}

	// Host repetido.
	dup := entity.Registry{Host: "ghcr.io", Username: "otro", Password: "x", CreatedAt: t0, UpdatedAt: t0}
	if err := r.Create(ctx, &dup); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("host repetido: %v", err)
	}

	// List por host, sin la contraseña.
	list, err := r.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []entity.Registry{
		{ID: hub.ID, Host: "docker.io", Username: "yo", CreatedAt: t0, UpdatedAt: t0},
		{ID: ghcr.ID, Host: "ghcr.io", Username: "bot", CreatedAt: t0, UpdatedAt: t0},
		{ID: local.ID, Host: "registry.example.com:5000", Username: "ci", CreatedAt: t0, UpdatedAt: t0},
	}
	if !reflect.DeepEqual(list, want) {
		t.Fatalf("List:\n got %+v\nwant %+v", list, want)
	}
	if g, err := r.Get(ctx, "ghcr.io"); err != nil || !reflect.DeepEqual(g, want[1]) {
		t.Fatalf("Get: %+v, %v", g, err)
	}
	if _, err := r.Get(ctx, "quay.io"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Get inexistente: %v", err)
	}

	// Credentials: solo los hosts pedidos, con la contraseña descifrada.
	creds, err := r.Credentials(ctx, []string{"ghcr.io", "quay.io", "registry.example.com:5000"})
	if err != nil {
		t.Fatal(err)
	}
	if len(creds) != 2 || creds["ghcr.io"].Password != "s3cr3t-ghcr" || creds["ghcr.io"].Username != "bot" ||
		creds["registry.example.com:5000"].Password != "s3cr3t-local" {
		t.Fatalf("Credentials: %v", creds)
	}
	if creds, err := r.Credentials(ctx, nil); err != nil || len(creds) != 0 {
		t.Fatalf("Credentials sin hosts: %v, %v", creds, err)
	}

	// Update reemplaza usuario y contraseña, y completa id y created_at.
	t1 := t0.Add(time.Hour)
	upd := entity.Registry{Host: "ghcr.io", Username: "bot2", Password: "nueva-pass", UpdatedAt: t1}
	if err := r.Update(ctx, &upd); err != nil {
		t.Fatal(err)
	}
	if upd.ID != ghcr.ID || !upd.CreatedAt.Equal(t0) {
		t.Fatalf("Update no completó id y created_at: %+v", upd)
	}
	creds, err = r.Credentials(ctx, []string{"ghcr.io"})
	if err != nil || creds["ghcr.io"].Username != "bot2" || creds["ghcr.io"].Password != "nueva-pass" || !creds["ghcr.io"].UpdatedAt.Equal(t1) {
		t.Fatalf("después de Update: %v, %v", creds, err)
	}
	missing := entity.Registry{Host: "quay.io", Username: "x", Password: "y", UpdatedAt: t1}
	if err := r.Update(ctx, &missing); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Update inexistente: %v", err)
	}

	// Delete.
	if err := r.Delete(ctx, "ghcr.io"); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(ctx, "ghcr.io"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Delete otra vez: %v", err)
	}
	if creds, err := r.Credentials(ctx, []string{"ghcr.io"}); err != nil || len(creds) != 0 {
		t.Fatalf("Credentials de un borrado: %v, %v", creds, err)
	}
}

// Create cifra en la misma transacción que el INSERT: si la transacción de
// afuera hace rollback, no queda nada (ni una fila con la contraseña vacía).
func TestRegistryCreateInTx(t *testing.T) {
	db := sqlitetest.New(t)
	r := NewRegistryRepository(db, testCipher(t))
	ctx := context.Background()
	boom := errors.New("boom")
	err := db.InTx(ctx, func(ctx context.Context) error {
		reg := entity.Registry{Host: "ghcr.io", Username: "bot", Password: "x", CreatedAt: t0, UpdatedAt: t0}
		if err := r.Create(ctx, &reg); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if list, _ := r.List(ctx); len(list) != 0 {
		t.Fatalf("quedó %+v", list)
	}
}
