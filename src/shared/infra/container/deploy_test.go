package container

import (
	"bytes"
	"context"
	"testing"

	"github.com/CPU-commits/muelle/src/settings"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
)

// El registry inicial de install (MUELLE_INIT_REGISTRY_FILE, contrato §2.10)
// se guarda solo en el primer arranque de la base: uno que el admin borró no
// vuelve con el próximo arranque, aunque el secreto siga montado.
func TestInitRegistrySeededOnce(t *testing.T) {
	const password = "ghp_s3cr3t-de-install"
	s := testSettings(t)
	s.Init.Registry = &settings.InitRegistry{Host: "ghcr.io", Username: "bot", Password: password}
	ctx := context.Background()

	c, err := New(ctx, s, testLogger(t), WithDocker(dockertest.New()))
	if err != nil {
		t.Fatal(err)
	}
	var host, username string
	var stored []byte
	if err := c.DB.Conn(ctx).QueryRowContext(ctx, `SELECT host, username, password FROM registries`).Scan(&host, &username, &stored); err != nil {
		c.Close()
		t.Fatalf("el primer arranque no guardó el registry: %v", err)
	}
	if host != "ghcr.io" || username != "bot" || bytes.Contains(stored, []byte(password)) {
		c.Close()
		t.Fatalf("registry: %s %s %q", host, username, stored)
	}
	// Lo borra el admin (como el DELETE del panel).
	if _, err := c.DB.Conn(ctx).ExecContext(ctx, `DELETE FROM registries`); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	c, err = New(ctx, s, testLogger(t), WithDocker(dockertest.New()))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var n int
	if err := c.DB.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM registries`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("el registry borrado volvió: %d", n)
	}
}
