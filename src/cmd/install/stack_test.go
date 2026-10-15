package install

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/shared/domain/naming"
)

var update = flag.Bool("update", false, "regenera los golden de testdata/")

// golden compara got con testdata/<name>; con -update lo reescribe.
func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (corre con -update para crearlo)", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s no coincide (corre con -update si el cambio es a propósito)\n--- got\n%s\n--- want\n%s", name, got, want)
	}
}

// stackCases son los tres compose de los golden: el de producción sin token
// y con las opcionales, el de un test (prefijo) con token, y el de las
// imágenes privadas, con el secreto del registry.
func stackCases() map[string]stackInput {
	return map[string]stackInput{
		"stack.golden.json": {
			Names:  naming.InstallNamesFor(naming.MuelleStack),
			Image:  "ghcr.io/cpu-commits/muelle:v0.1.0",
			Origin: "https://panel.tudominio.cl", PublicIP: "203.0.113.24", NodeID: "node-1",
			Opts: Options{Email: "tu@correo.cl", PublicIPv6: "2001:db8::24", HTTPPort: 80, HTTPSPort: 443,
				CaddyImage: "ghcr.io/cpu-commits/muelle-caddy:v0.1.0"},
		},
		"stack_token.golden.json": {
			Names:  naming.InstallNamesFor("it5abc"),
			Image:  "it5-muelle:test",
			Origin: "https://127.0.0.1:22443", NodeID: "node-1", Token: true,
			Opts: Options{Email: "it5@example.com", HTTPPort: 22080, HTTPSPort: 22443},
		},
		"stack_registry.golden.json": {
			Names:  naming.InstallNamesFor(naming.MuelleStack),
			Image:  "ghcr.io/cpu-commits/muelle:v0.1.0",
			Origin: "https://panel.tudominio.cl:8443", PublicIP: "203.0.113.24", NodeID: "abc123", Registry: true,
			Opts: Options{Email: "tu@correo.cl", HTTPPort: 8080, HTTPSPort: 8443},
		},
	}
}

func TestComposeGolden(t *testing.T) {
	for name, in := range stackCases() {
		t.Run(name, func(t *testing.T) {
			got, err := composeStack(in)
			if err != nil {
				t.Fatal(err)
			}
			if !json.Valid(got) {
				t.Fatalf("no es JSON: %s", got)
			}
			golden(t, name, got)
		})
	}
}

// Las variables (contrato §5.2): las fijas, los puertos siempre, y las
// opcionales y las rutas de los secretos solo con valor.
func TestServiceEnv(t *testing.T) {
	in := stackCases()["stack_token.golden.json"]
	env := serviceEnv(in)
	want := map[string]string{
		"MUELLE_PANEL_ORIGIN":               "https://127.0.0.1:22443",
		"MUELLE_LISTEN":                     ":8080",
		"MUELLE_DATA_DIR":                   "/data",
		"MUELLE_MASTER_KEY_FILE":            "/run/secrets/muelle_master_key",
		"MUELLE_INIT_ACME_EMAIL":            "it5@example.com",
		"MUELLE_INIT_PROXY_HTTP_PORT":       "22080",
		"MUELLE_INIT_PROXY_HTTPS_PORT":      "22443",
		"MUELLE_INIT_CLOUDFLARE_TOKEN_FILE": "/run/secrets/muelle_init_cloudflare_token",
	}
	if len(env) != len(want) {
		t.Fatalf("env=%v", env)
	}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("%s=%q, esperaba %q", k, env[k], v)
		}
	}
	if _, ok := env["MUELLE_SECURE_COOKIES"]; ok {
		t.Error("MUELLE_SECURE_COOKIES va sin definir (true)")
	}

	in = stackCases()["stack_registry.golden.json"]
	env = serviceEnv(in)
	if env["MUELLE_INIT_REGISTRY_FILE"] != "/run/secrets/muelle_init_registry" || env["MUELLE_INIT_PUBLIC_IP"] != "203.0.113.24" {
		t.Errorf("env=%v", env)
	}
	for _, k := range []string{"MUELLE_INIT_CLOUDFLARE_TOKEN_FILE", "MUELLE_INIT_PUBLIC_IPV6", "MUELLE_INIT_CADDY_IMAGE"} {
		if _, ok := env[k]; ok {
			t.Errorf("%s no debería ir: %v", k, env)
		}
	}
}

// docker stack deploy interpola ${VAR} con su entorno: todo "$" del
// documento va como "$$" (contrato §5.3).
func TestComposeEscapesDollar(t *testing.T) {
	in := stackCases()["stack_token.golden.json"]
	in.Opts.Email = "a$b@example.com"
	in.Opts.CaddyImage = "x/${HOME}:1"
	got, err := composeStack(in)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("a$$b@example.com")) || !bytes.Contains(got, []byte("x/$${HOME}:1")) {
		t.Fatalf("sin escapar: %s", got)
	}
	if rest := strings.ReplaceAll(string(got), "$$", ""); strings.Contains(rest, "$") {
		t.Fatalf("queda un $ suelto: %s", got)
	}
}

// El CLI de docker acepta el compose tal cual (sin version:, como 3.13) y
// deshace el "$$". docker stack config no necesita el daemon.
func TestComposeAcceptedByDockerCLI(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("sin el CLI de docker")
	}
	noDaemon := append(os.Environ(), "DOCKER_HOST=unix:///nonexistent.sock")
	help := exec.Command("docker", "stack", "config", "--help")
	help.Env = noDaemon
	if err := help.Run(); err != nil {
		t.Skipf("el CLI no tiene docker stack config: %v", err)
	}
	in := stackCases()["stack_token.golden.json"]
	in.Registry = true
	in.Opts.Email = "a$b@example.com"
	compose, err := composeStack(in)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("docker", "stack", "config", "-c", "-")
	cmd.Env = noDaemon
	cmd.Stdin = bytes.NewReader(compose)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker stack config: %v\n%s\n---\n%s", err, out, compose)
	}
	for _, want := range []string{"a$b@example.com", "name: it5abc_data", "node.id == node-1", "name: it5abc_init_registry",
		"target: muelle_master_key", "order: stop-first", "failure_action: rollback"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("falta %q en la salida:\n%s", want, out)
		}
	}
}
