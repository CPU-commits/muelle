package docker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeCLI escribe un "docker" de mentira que guarda en dir lo que recibe:
// los argumentos, su entorno, los permisos y el config.json de DOCKER_CONFIG,
// y el stdin. Sale con el código que diga dir/exit (0 si no existe) y, si
// existe dir/sleep, se queda esperando.
func fakeCLI(t *testing.T) (cli, dir string) {
	t.Helper()
	dir = t.TempDir()
	cli = filepath.Join(dir, "docker")
	script := `#!/bin/sh
d='` + dir + `'
printf '%s\n' "$@" > "$d/args"
env > "$d/env"
printf '%s' "$DOCKER_CONFIG" > "$d/config_path"
stat -c %a "$DOCKER_CONFIG" > "$d/config_perm"
stat -c %a "$DOCKER_CONFIG/config.json" > "$d/config_json_perm"
cat "$DOCKER_CONFIG/config.json" > "$d/config.json"
cat > "$d/stdin"
if [ -f "$d/sleep" ]; then sleep 30; fi
echo "Creating service auditfy_staging_api_api"
echo "aviso en stderr" >&2
if [ -f "$d/exit" ]; then exit "$(cat "$d/exit")"; fi
`
	if err := os.WriteFile(cli, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return cli, dir
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func newStackClient(t *testing.T, cli string) *Client {
	t.Helper()
	c, err := New(Options{CLI: cli, TempDir: t.TempDir(), Host: "unix:///run/fake-docker.sock"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestDeployStack(t *testing.T) {
	cli, dir := fakeCLI(t)
	c := newStackClient(t, cli)
	t.Setenv("MUELLE_TEST_SECRETO", "no-debe-llegar")

	compose := "services:\n  api:\n    image: ghcr.io/x/api@sha256:abc\n    command: echo $$HOME\n"
	out, err := c.DeployStack(context.Background(), StackDeploy{
		Name:       "auditfy_staging_api",
		Compose:    []byte(compose),
		Registries: []RegistryAuth{{Host: "ghcr.io", Username: "bot", Password: "s3cr3t"}},
	})
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}

	wantArgs := "stack\ndeploy\n--with-registry-auth\n--prune\n--detach=true\n--resolve-image=never\n-c\n-\nauditfy_staging_api\n"
	if got := read(t, filepath.Join(dir, "args")); got != wantArgs {
		t.Fatalf("argumentos:\n%s\nesperaba:\n%s", got, wantArgs)
	}
	if got := read(t, filepath.Join(dir, "stdin")); got != compose {
		t.Fatalf("el compose va por stdin tal cual: %q", got)
	}
	if got := read(t, filepath.Join(dir, "config_perm")); strings.TrimSpace(got) != "700" {
		t.Fatalf("DOCKER_CONFIG con permisos %s, esperaba 700", got)
	}
	if got := read(t, filepath.Join(dir, "config_json_perm")); strings.TrimSpace(got) != "600" {
		t.Fatalf("config.json con permisos %s, esperaba 600", got)
	}
	if cfg := read(t, filepath.Join(dir, "config.json")); !strings.Contains(cfg, `"ghcr.io"`) || !strings.Contains(cfg, `"auth"`) {
		t.Fatalf("config.json sin las credenciales: %s", cfg)
	}
	// El directorio temporal se borra al terminar.
	if _, err := os.Stat(read(t, filepath.Join(dir, "config_path"))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DOCKER_CONFIG temporal sigue ahí: %v", err)
	}
	// El CLI no recibe el entorno de muelle, solo PATH, HOME y DOCKER_*.
	env := read(t, filepath.Join(dir, "env"))
	if strings.Contains(env, "MUELLE_TEST_SECRETO") || !strings.Contains(env, "PATH=") {
		t.Fatalf("entorno del CLI:\n%s", env)
	}
	if !strings.Contains(env, "DOCKER_HOST=unix:///run/fake-docker.sock") {
		t.Fatalf("el CLI debe usar el mismo daemon que el SDK:\n%s", env)
	}
	// La salida tiene stdout y stderr, para el log del deploy.
	if !strings.Contains(out, "Creating service") || !strings.Contains(out, "aviso en stderr") {
		t.Fatalf("salida: %q", out)
	}
}

func TestDeployStackFailure(t *testing.T) {
	cli, dir := fakeCLI(t)
	if err := os.WriteFile(filepath.Join(dir, "exit"), []byte("3"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newStackClient(t, cli)
	out, err := c.DeployStack(context.Background(), StackDeploy{Name: "auditfy_staging_api", Compose: []byte("services: {}\n")})
	if !errors.Is(err, ErrCommand) || !strings.Contains(err.Error(), "3") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out, "aviso en stderr") {
		t.Fatalf("la salida va también cuando falla: %q", out)
	}
	// Se borra aunque el comando falle.
	if _, err := os.Stat(read(t, filepath.Join(dir, "config_path"))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DOCKER_CONFIG temporal sigue ahí tras el fallo: %v", err)
	}
}

func TestDeployStackCancel(t *testing.T) {
	cli, dir := fakeCLI(t)
	if err := os.WriteFile(filepath.Join(dir, "sleep"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	c := newStackClient(t, cli)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.DeployStack(ctx, StackDeploy{Name: "auditfy_staging_api", Compose: []byte("services: {}\n")})
	if err == nil || time.Since(start) > 10*time.Second {
		t.Fatalf("cancelar debe cortar el comando: err=%v en %v", err, time.Since(start))
	}
}

func TestStackNameIsChecked(t *testing.T) {
	cli, dir := fakeCLI(t)
	c := newStackClient(t, cli)
	for _, name := range []string{"", "-x", "--prune", "Auditfy", "a b", "a/b", strings.Repeat("a", 64)} {
		if _, err := c.DeployStack(context.Background(), StackDeploy{Name: name}); err == nil {
			t.Errorf("DeployStack(%q) debió fallar", name)
		}
		if _, err := c.RemoveStack(context.Background(), name); err == nil {
			t.Errorf("RemoveStack(%q) debió fallar", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "args")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("con un nombre inválido no se ejecuta el CLI")
	}
}

func TestRemoveStack(t *testing.T) {
	cli, dir := fakeCLI(t)
	c := newStackClient(t, cli)
	if _, err := c.RemoveStack(context.Background(), "auditfy_staging_api"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(dir, "args")); got != "stack\nrm\nauditfy_staging_api\n" {
		t.Fatalf("argumentos: %q", got)
	}
}

func TestLimitedOutput(t *testing.T) {
	var b limitedBuffer
	b.max = 10
	n, err := b.Write([]byte("0123456789abcdef"))
	if err != nil || n != 16 {
		t.Fatalf("Write devuelve todo lo recibido aunque lo corte: %d %v", n, err)
	}
	if got := b.String(); got != "0123456789\n[… salida cortada]" {
		t.Fatalf("String = %q", got)
	}
}
