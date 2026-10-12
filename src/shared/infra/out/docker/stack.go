package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// maxCommandOutput es lo máximo de la salida del CLI que se guarda (va al log
// del deploy, que tiene su propio límite).
const maxCommandOutput = 64 << 10

// stackNameRe es un nombre de stack que el CLI no puede confundir con un flag:
// los que arma naming.Stack (slugs separados por "_"), de hasta 63 caracteres.
var stackNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

func checkStackName(name string) error {
	if !stackNameRe.MatchString(name) {
		return fmt.Errorf("docker: %q no es un nombre de stack válido", name)
	}
	return nil
}

func (c *Client) DeployStack(ctx context.Context, d StackDeploy) (string, error) {
	if err := checkStackName(d.Name); err != nil {
		return "", err
	}
	return c.runCLI(ctx, d.Registries, bytes.NewReader(d.Compose),
		"stack", "deploy", "--with-registry-auth", "--prune", "--detach=true", "--resolve-image=never", "-c", "-", d.Name)
}

func (c *Client) RemoveStack(ctx context.Context, name string) (string, error) {
	if err := checkStackName(name); err != nil {
		return "", err
	}
	return c.runCLI(ctx, nil, nil, "stack", "rm", name)
}

// runCLI ejecuta el CLI de docker con un DOCKER_CONFIG temporal propio (0700,
// con un config.json 0600 que solo tiene las credenciales de auths), que se
// borra al terminar aunque el comando falle. Así el CLI nunca lee el
// ~/.docker del usuario ni deja credenciales en disco. El entorno es solo
// PATH, HOME y las variables DOCKER_* de conexión: el CLI interpola ${VAR}
// del compose con su entorno, y el de muelle no tiene nada que hacer ahí.
func (c *Client) runCLI(ctx context.Context, auths []RegistryAuth, stdin io.Reader, args ...string) (string, error) {
	dir, err := os.MkdirTemp(c.tempDir, "muelle-docker-config-") // MkdirTemp crea con 0700
	if err != nil {
		return "", fmt.Errorf("docker: creando el DOCKER_CONFIG temporal: %w", err)
	}
	defer os.RemoveAll(dir)
	cfg, err := dockerConfig(auths)
	if err != nil {
		return "", fmt.Errorf("docker: armando config.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), cfg, 0o600); err != nil {
		return "", fmt.Errorf("docker: escribiendo config.json: %w", err)
	}

	cmd := exec.CommandContext(ctx, c.cli, args...)
	cmd.Env = c.cliEnv(dir)
	cmd.Stdin = stdin
	out := &limitedBuffer{max: maxCommandOutput}
	cmd.Stdout, cmd.Stderr = out, out
	// Si se cancela ctx, el proceso recibe SIGKILL; WaitDelay acota lo que se
	// espera a que cierre stdout y stderr (un hijo suyo podría heredarlos).
	cmd.WaitDelay = 2 * time.Second
	runErr := cmd.Run()
	what := "docker " + strings.Join(args[:2], " ")
	switch {
	case runErr == nil:
		return out.String(), nil
	case ctx.Err() != nil:
		return out.String(), fmt.Errorf("%s: %w", what, ctx.Err())
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return out.String(), fmt.Errorf("%w: %s terminó con código %d", ErrCommand, what, exitErr.ExitCode())
	}
	return out.String(), fmt.Errorf("%w: %s: %w", ErrCommand, what, runErr)
}

// cliEnvKeys son las variables del entorno de muelle que recibe el CLI.
var cliEnvKeys = []string{"PATH", "HOME", "DOCKER_HOST", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH", "DOCKER_API_VERSION"}

func (c *Client) cliEnv(configDir string) []string {
	env := []string{"DOCKER_CONFIG=" + configDir}
	for _, k := range cliEnvKeys {
		if k == "DOCKER_HOST" && c.host != "" {
			env = append(env, "DOCKER_HOST="+c.host) // el mismo daemon que el SDK
			continue
		}
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// limitedBuffer guarda hasta max bytes y descarta el resto, avisando al final.
// exec.Cmd escribe en él desde una sola goroutine a la vez (Stdout == Stderr).
type limitedBuffer struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.buf.Len(); room < len(p) {
		b.truncated = true
		b.buf.Write(p[:max(room, 0)])
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *limitedBuffer) String() string {
	if b.truncated {
		return b.buf.String() + "\n[… salida cortada]"
	}
	return b.buf.String()
}
