//go:build integration

package service

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRenderGoldensAcceptedByDockerCLI pasa cada golden por docker stack
// config, que lo valida con el esquema del CLI de docker stack deploy y lo
// vuelve a interpolar. No crea nada en Docker.
func TestRenderGoldensAcceptedByDockerCLI(t *testing.T) {
	bin, err := exec.LookPath("docker")
	if err != nil {
		t.Skip("docker no está en el PATH")
	}
	goldens, err := filepath.Glob("testdata/render/*.golden.json")
	if err != nil || len(goldens) == 0 {
		t.Fatalf("sin golden: %v", err)
	}
	for _, golden := range goldens {
		name := strings.TrimSuffix(filepath.Base(golden), ".golden.json")
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(golden)
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
			cmd.Stdin = bytes.NewReader(src)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("docker stack config: %v\n%s", err, stderr.String())
			}
			got := stdout.String()
			// Un $$ del JSON sale como $ (docker stack deploy volvió a interpolar).
			if strings.Contains(got, "$$") {
				t.Errorf("quedó un $$ en la salida:\n%s", got)
			}
			switch name {
			case "form-command-dollar":
				if !strings.Contains(got, "echo $HOME") {
					t.Errorf("falta echo $HOME:\n%s", got)
				}
			case "form-http":
				if !strings.Contains(got, "$5 por mes") {
					t.Errorf("falta $5 por mes:\n%s", got)
				}
			case "compose-basic":
				for _, want := range []string{"p$ss@db", "--price=$5", "$10"} {
					if !strings.Contains(got, want) {
						t.Errorf("falta %q:\n%s", want, got)
					}
				}
			}
		})
	}
}
