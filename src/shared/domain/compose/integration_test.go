//go:build integration

package compose

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestMarshalAcceptedByDockerCLI verifica que docker stack deploy lee el
// compose final: docker stack config lo valida con el esquema del CLI y
// vuelve a interpolar. No crea nada en Docker.
func TestMarshalAcceptedByDockerCLI(t *testing.T) {
	bin, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker no está en el PATH")
	}
	env := map[string]string{"TAG": "2.0", "SECRET": "s3cr$t", "B": "bval"}
	f, err := Load(context.Background(), readKitchenSink(t), "it2x_staging_api", env)
	if err != nil {
		t.Fatal(err)
	}
	f.Networks = map[string]Network{"it2x_staging": {Name: "it2x_staging", External: true}}
	for name, s := range f.Services {
		s.Networks = []string{"it2x_staging"}
		f.Services[name] = s
	}
	out, err := Marshal(f)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "stack", "config", "-c", "-")
	cmd.Dir = t.TempDir()
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "DOCKER_CONFIG=" + t.TempDir()}
	if h := os.Getenv("DOCKER_HOST"); h != "" {
		cmd.Env = append(cmd.Env, "DOCKER_HOST="+h)
	}
	cmd.Stdin = bytes.NewReader(out)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("docker stack config: %v\n%s\ncompose:\n%s", err, stderr.String(), out)
	}
	got := stdout.String()
	// Un $$ del JSON sale como $ (docker stack deploy volvió a interpolar).
	for _, want := range []string{"s3cr$t", "--price=$5", "echo $HOME", "$10"} {
		if !strings.Contains(got, want) {
			t.Errorf("falta %q en la salida:\n%s", want, got)
		}
	}
	if strings.Contains(got, "$$") {
		t.Errorf("quedó un $$ en la salida:\n%s", got)
	}
}
