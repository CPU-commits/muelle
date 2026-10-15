package install

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	muelleRef = "ghcr.io/cpu-commits/muelle:v0.1.0"
	caddyRef  = "ghcr.io/cpu-commits/muelle-caddy:v0.1.0"
	regPass   = "ghp_s3cr3tRegistryPassw0rd"
)

func writeDockerConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func authOf(user, pass string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
}

func TestRegistryHost(t *testing.T) {
	for ref, want := range map[string]string{
		muelleRef:                         "ghcr.io",
		"it5-muelle:test":                 "docker.io",
		"127.0.0.1:22500/it5-muelle:test": "127.0.0.1:22500",
		"registry.example.com:5000/x/y":   "registry.example.com:5000",
	} {
		if got, err := registryHost(ref); err != nil || got != want {
			t.Errorf("registryHost(%q) = %q, %v; esperaba %q", ref, got, err, want)
		}
	}
}

// Con --registry-user, la credencial es la del host de la imagen de muelle;
// el config.json no se lee.
func TestResolveCredentialsFlags(t *testing.T) {
	o := Options{RegistryUser: "bot", RegistryPassword: regPass}
	creds, err := resolveCredentials(o, "/no/existe/config.json", muelleRef, caddyRef)
	if err != nil {
		t.Fatal(err)
	}
	c := creds["ghcr.io"]
	if len(creds) != 1 || c == nil || c.Host != "ghcr.io" || c.Username != "bot" || c.Password != regPass {
		t.Fatalf("creds=%v", creds)
	}
	// Otra imagen de Caddy, de otro host: sin credencial (anónima).
	creds, err = resolveCredentials(o, "", muelleRef, "docker.io/library/caddy:2")
	if err != nil || len(creds) != 1 || creds["docker.io"] != nil {
		t.Fatalf("creds=%v err=%v", creds, err)
	}
}

func TestResolveCredentialsDockerConfig(t *testing.T) {
	path := writeDockerConfig(t, `{"auths":{"https://ghcr.io":{"auth":"`+authOf("bot", regPass)+`"},
		"https://index.docker.io/v1/":{"auth":"`+authOf("hubuser", "hub:pass")+`"}},"credsStore":"desktop"}`)
	creds, err := resolveCredentials(Options{}, path, muelleRef, "caddy:2")
	if err != nil {
		t.Fatal(err)
	}
	if c := creds["ghcr.io"]; c == nil || c.Username != "bot" || c.Password != regPass || c.Host != "ghcr.io" {
		t.Fatalf("ghcr.io=%v", c)
	}
	// Docker Hub se guarda con su nombre largo, y la contraseña puede tener ":".
	if c := creds["docker.io"]; c == nil || c.Username != "hubuser" || c.Password != "hub:pass" {
		t.Fatalf("docker.io=%v", c)
	}
}

// Sin el archivo (no se montó), no hay credenciales: un pull anónimo.
func TestResolveCredentialsNoFile(t *testing.T) {
	creds, err := resolveCredentials(Options{}, filepath.Join(t.TempDir(), "no-existe.json"), muelleRef, caddyRef)
	if err != nil || len(creds) != 0 {
		t.Fatalf("creds=%v err=%v", creds, err)
	}
}

// Un helper de credenciales (credsStore o credHelpers) no corre dentro del
// contenedor: install pide los flags.
func TestResolveCredentialsHelper(t *testing.T) {
	for name, content := range map[string]string{
		"credsStore":  `{"auths":{"ghcr.io":{}},"credsStore":"pass"}`,
		"credHelpers": `{"auths":{},"credHelpers":{"ghcr.io":"gcloud"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := resolveCredentials(Options{}, writeDockerConfig(t, content), muelleRef)
			if err == nil || !strings.Contains(err.Error(), "--registry-user") || !strings.Contains(err.Error(), "--registry-password-stdin") {
				t.Fatalf("err=%v", err)
			}
		})
	}
	// Con credsStore pero sin haber hecho login en ese host, la imagen puede
	// ser pública: sin credencial y sin error.
	creds, err := resolveCredentials(Options{}, writeDockerConfig(t, `{"auths":{"ghcr.io":{"auth":"`+authOf("bot", regPass)+`"}},"credsStore":"desktop"}`), muelleRef, "caddy:2")
	if err != nil || creds["docker.io"] != nil || creds["ghcr.io"] == nil {
		t.Fatalf("creds=%v err=%v", creds, err)
	}
}

func TestResolveCredentialsBadConfig(t *testing.T) {
	for name, content := range map[string]string{
		"no es json":     `{"auths":`,
		"auth no base64": `{"auths":{"ghcr.io":{"auth":"%%` + regPass + `"}}}`,
		"auth sin :":     `{"auths":{"ghcr.io":{"auth":"` + base64.StdEncoding.EncodeToString([]byte(regPass)) + `"}}}`,
		"con control":    `{"auths":{"ghcr.io":{"auth":"` + authOf("bot", regPass+"\n") + `"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := resolveCredentials(Options{}, writeDockerConfig(t, content), muelleRef)
			if err == nil || !strings.Contains(err.Error(), "config.json") {
				t.Fatalf("err=%v", err)
			}
			if strings.Contains(err.Error(), regPass) {
				t.Fatalf("el error cita la contraseña: %v", err)
			}
		})
	}
}
