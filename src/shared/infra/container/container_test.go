package container

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/cmd/cron"
	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/internal/admin"
	"github.com/CPU-commits/muelle/src/settings"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
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

// newTestContainer arma el contenedor completo, con un Docker en memoria
// (dockertest: los tests nunca tocan el Docker real), y lo cierra al terminar el test.
func newTestContainer(t *testing.T) *Container {
	t.Helper()
	c, err := New(context.Background(), testSettings(t), testLogger(t), WithDocker(dockertest.New()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestNew(t *testing.T) {
	s := testSettings(t)
	c, err := New(context.Background(), s, testLogger(t), WithDocker(dockertest.New()))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if c.DB == nil || c.Identity == nil || c.Catalog == nil || c.Secrets == nil || c.Admin == nil || c.Server == nil ||
		c.Deploy == nil || c.Ingress == nil || c.Docker == nil {
		t.Fatalf("faltan piezas: %+v", c)
	}
	if _, err := os.Stat(filepath.Join(s.DataDir, "muelle.db")); err != nil {
		t.Fatalf("la base no está en <DataDir>/muelle.db: %v", err)
	}
	// identity (sesiones vencidas) + admin (retención de auditoría y backup diario) + ingress (proxy).
	names := make([]string, 0, len(c.Jobs))
	for _, j := range c.Jobs {
		names = append(names, j.Name)
	}
	if got := strings.Join(names, ", "); got != "sesiones vencidas, retención de auditoría, backup diario, proxy" {
		t.Fatalf("jobs=%s", got)
	}

	w := httptest.NewRecorder()
	c.Server.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/auth/me", nil))
	httptestutil.Expect(t, w, http.StatusUnauthorized)
	if code := httptestutil.ErrorCode(t, w); code != "unauthenticated" {
		t.Fatalf("code=%s", code)
	}
}

// Las rutas de admin están registradas: GET /api/v1/settings con sesión responde 200.
func TestAdminRoutesWired(t *testing.T) {
	e := api{t: t, c: newTestContainer(t)}
	s := e.setup()
	w := e.do("GET", "/api/v1/settings", nil, s)
	httptestutil.Expect(t, w, http.StatusOK)
	if m := httptestutil.Decode[map[string]any](t, w); len(m) == 0 {
		t.Fatalf("settings vacío: %s", w.Body.String())
	}
}

// identity recibe PanelOrigin: el link de una invitación empieza con él.
func TestInviteURLUsesPanelOrigin(t *testing.T) {
	s := testSettings(t)
	c, err := New(context.Background(), s, testLogger(t), WithDocker(dockertest.New()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	e := api{t: t, c: c}

	w := e.do("POST", "/api/v1/invites", nil, e.setup())
	httptestutil.Expect(t, w, http.StatusCreated)
	url, _ := httptestutil.Decode[map[string]any](t, w)["url"].(string)
	if prefix := s.PanelOrigin + "/invite/"; !strings.HasPrefix(url, prefix) || len(url) == len(prefix) {
		t.Fatalf("url=%q, esperaba %s<token>", url, prefix)
	}
}

// admin recibe DataDir: el job "backup diario" deja la copia en <DataDir>/backups.
func TestBackupJobWritesUnderDataDir(t *testing.T) {
	s := testSettings(t)
	c, err := New(context.Background(), s, testLogger(t), WithDocker(dockertest.New()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })

	i := slices.IndexFunc(c.Jobs, func(j cron.Job) bool { return j.Name == "backup diario" })
	if i < 0 {
		t.Fatal(`falta el job "backup diario" en c.Jobs`)
	}
	if err := c.Jobs[i].Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(s.DataDir, "backups", "muelle-*.db"))
	if err != nil || len(files) != 1 {
		t.Fatalf("backups en %s: %v (err=%v)", filepath.Join(s.DataDir, "backups"), files, err)
	}
}

// Con otra clave maestra sobre la misma base, muelle no arranca (§12 del spec).
func TestNewWrongMasterKey(t *testing.T) {
	s := testSettings(t)
	c, err := New(context.Background(), s, testLogger(t), WithDocker(dockertest.New()))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}

	other := s
	other.MasterKeyFile = writeKey(t, t.TempDir(), "otra")
	c, err = New(context.Background(), other, testLogger(t), WithDocker(dockertest.New()))
	if err == nil {
		c.Close()
		t.Fatal("con otra clave maestra New no debió armar el contenedor")
	}
	if !strings.Contains(err.Error(), "clave maestra incorrecta") || !errors.Is(err, admin.ErrWrongMasterKey) {
		t.Fatalf("err=%v", err)
	}

	// El intento fallido no cambia nada: la clave original sigue sirviendo.
	c, err = New(context.Background(), s, testLogger(t), WithDocker(dockertest.New()))
	if err != nil {
		t.Fatalf("con la clave original: %v", err)
	}
	c.Close()
}

// Sin PanelOrigin o DataDir, New devuelve un error, en vez de dejar que un
// módulo haga panic, y no toca nada.
func TestNewRequiresPanelOriginAndDataDir(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*settings.Settings)
		want string
	}{
		{"sin PanelOrigin", func(s *settings.Settings) { s.PanelOrigin = "" }, "MUELLE_PANEL_ORIGIN"},
		{"sin DataDir", func(s *settings.Settings) { s.DataDir = "" }, "MUELLE_DATA_DIR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testSettings(t)
			dataDir := s.DataDir
			tc.edit(&s)
			c, err := New(context.Background(), s, testLogger(t), WithDocker(dockertest.New()))
			if err == nil {
				c.Close()
				t.Fatal("New debió fallar")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v, esperaba que nombre %s", err, tc.want)
			}
			if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
				t.Fatalf("New no debió crear DataDir: %v", err)
			}
		})
	}
}

// Si la clave maestra falta o no sirve, el error dice qué variable revisar y
// cómo crear una clave, y New no crea DataDir.
func TestNewBadMasterKey(t *testing.T) {
	for _, tc := range []struct{ name, content string }{
		{"no existe", ""},
		{"no es base64", "no-es-base64!"},
		{"tamaño incorrecto", base64.StdEncoding.EncodeToString([]byte("corta"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testSettings(t)
			if tc.content == "" {
				if err := os.Remove(s.MasterKeyFile); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(s.MasterKeyFile, []byte(tc.content+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			c, err := New(context.Background(), s, testLogger(t), WithDocker(dockertest.New()))
			if err == nil {
				c.Close()
				t.Fatal("New debió fallar")
			}
			for _, want := range []string{"MUELLE_MASTER_KEY_FILE", s.MasterKeyFile, "muelle genkey"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err=%v, esperaba que diga %q", err, want)
				}
			}
			if _, err := os.Stat(s.DataDir); !os.IsNotExist(err) {
				t.Fatalf("con la clave mal no se debió crear DataDir: %v", err)
			}
		})
	}
}
