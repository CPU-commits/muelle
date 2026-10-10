package container

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/internal/admin"
	"github.com/CPU-commits/muelle/src/settings"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
)

// testSettings devuelve la configuración de un muelle de prueba: la base en
// un DataDir temporal y una clave maestra recién generada.
func testSettings(t *testing.T) settings.Settings {
	t.Helper()
	dir := t.TempDir()
	return settings.Settings{
		DataDir:       filepath.Join(dir, "data"),
		MasterKeyFile: writeKey(t, dir, "key"),
		PanelOrigin:   httptestutil.PanelOrigin,
		SecureCookies: true,
	}
}

func writeKey(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(crypto.GenerateKey()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func testLogger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(t.Output(), nil))
}

// newTestContainer arma el contenedor completo y lo cierra al terminar el test.
func newTestContainer(t *testing.T) *Container {
	t.Helper()
	c, err := New(context.Background(), testSettings(t), testLogger(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestNew(t *testing.T) {
	s := testSettings(t)
	c, err := New(context.Background(), s, testLogger(t))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if c.DB == nil || c.Identity == nil || c.Catalog == nil || c.Secrets == nil || c.Admin == nil || c.Server == nil {
		t.Fatalf("faltan piezas: %+v", c)
	}
	if _, err := os.Stat(filepath.Join(s.DataDir, "muelle.db")); err != nil {
		t.Fatalf("la base no está en <DataDir>/muelle.db: %v", err)
	}
	// identity (sesiones vencidas) + admin (retención de auditoría y backup diario).
	names := make([]string, 0, len(c.Jobs))
	for _, j := range c.Jobs {
		names = append(names, j.Name)
	}
	if got := strings.Join(names, ", "); got != "sesiones vencidas, retención de auditoría, backup diario" {
		t.Fatalf("jobs=%s", got)
	}

	w := httptest.NewRecorder()
	c.Server.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/auth/me", nil))
	httptestutil.Expect(t, w, http.StatusUnauthorized)
	if code := httptestutil.ErrorCode(t, w); code != "unauthenticated" {
		t.Fatalf("code=%s", code)
	}
}

// Con otra clave maestra sobre la misma base, muelle no arranca (§12 del spec).
func TestNewWrongMasterKey(t *testing.T) {
	s := testSettings(t)
	c, err := New(context.Background(), s, testLogger(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	other := s
	other.MasterKeyFile = writeKey(t, t.TempDir(), "otra")
	c, err = New(context.Background(), other, testLogger(t))
	if err == nil {
		c.Close()
		t.Fatal("con otra clave maestra New no debió armar el contenedor")
	}
	if !strings.Contains(err.Error(), "clave maestra incorrecta") || !errors.Is(err, admin.ErrWrongMasterKey) {
		t.Fatalf("err=%v", err)
	}

	// El intento fallido no cambia nada: la clave original sigue sirviendo.
	c, err = New(context.Background(), s, testLogger(t))
	if err != nil {
		t.Fatalf("con la clave original: %v", err)
	}
	c.Close()
}
