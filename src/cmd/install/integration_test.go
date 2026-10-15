//go:build integration

// Tests de muelle install contra el Docker real (contrato §5.4):
//
//	MUELLE_IT_IMAGE=it5-muelle:test go test -tags integration -count=1 -timeout 15m \
//	  -run '^TestIntegrationInstall' -v ./src/cmd/install/
//
// Necesitan la imagen de muelle de esta rama (por defecto it5-muelle:test, ver
// las convenciones del plan de la Fase 5) y un Swarm activo con este nodo de
// manager: si falta algo, se saltan. Todo lo que crean empieza con "it5"
// (--name-prefix) y usa puertos del host del 22000 al 22999; se borra al
// terminar, también si el test falla. Nunca crean ni borran los nombres fijos
// de muelle (muelle_panel, muelle-caddy…): solo miran que no aparezcan.
package install

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

const (
	itCaddyImage = "it5-muelle-caddy:test"
	itRelease    = 60 * time.Second // lo que se espera a que Docker suelte algo
	itConverge   = 90 * time.Second
	itInstall    = 4 * time.Minute
)

var (
	keyRe  = regexp.MustCompile(`(?m)^    ([A-Za-z0-9+/]{43}=)$`)
	linkRe = regexp.MustCompile(`(?m)^    (https://127\.0\.0\.1:[0-9]+/setup\?token=[^\s]+)$`)
)

func itImage() string { return cmp.Or(os.Getenv("MUELLE_IT_IMAGE"), "it5-muelle:test") }

// itDocker es el cliente real; salta el test si no hay un Swarm activo con
// este nodo de manager o si falta la imagen (nunca inicia un Swarm ni baja nada).
func itDocker(t *testing.T, images ...string) *docker.Client {
	t.Helper()
	c, err := docker.New(docker.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	ctx := context.Background()
	info, err := c.Info(ctx)
	if err != nil {
		t.Skipf("sin Docker: %v", err)
	}
	if info.SwarmState != docker.SwarmActive || !info.IsManager {
		t.Skipf("el nodo no es manager de un Swarm activo (%s)", info.SwarmState)
	}
	for _, ref := range images {
		if _, err := c.InspectImage(ctx, ref); err != nil {
			t.Skipf("falta la imagen %s (%v): constrúyela como dicen las convenciones del plan de la Fase 5", ref, err)
		}
	}
	return c
}

// itPrefix es un --name-prefix único: "it5" y 6 hex.
func itPrefix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return "it5" + hex.EncodeToString(b)
}

// eventually repite check cada 500 ms hasta que devuelve true o pasa timeout.
func eventually(t *testing.T, timeout time.Duration, what string, check func() (bool, error)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		ok, err := check()
		if ok {
			return
		}
		last = err
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("%s: no pasó en %s (último error: %v)", what, timeout, last)
}

// untilGone repite remove mientras dé ErrConflict, hasta itRelease; ErrNotFound es éxito.
func untilGone(ctx context.Context, remove func() error) error {
	deadline := time.Now().Add(itRelease)
	for {
		err := remove()
		if err == nil || errors.Is(err, docker.ErrNotFound) {
			return nil
		}
		if !errors.Is(err, docker.ErrConflict) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(time.Second)
	}
}

// cleanupInstall borra, al terminar el test, todo lo que install pudo crear
// con estos nombres (y la red centinela de guardForeign). Ignora lo que no
// existe; cualquier otro error es del test.
func cleanupInstall(t *testing.T, c *docker.Client, n naming.InstallNames) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		if out, err := c.RemoveStack(ctx, n.Stack); err != nil {
			t.Errorf("limpieza: docker stack rm %s: %v\n%s", n.Stack, err, out)
		}
		deadline := time.Now().Add(itRelease)
		for {
			_, err := c.InspectService(ctx, n.Service)
			if errors.Is(err, docker.ErrNotFound) {
				break
			}
			if time.Now().After(deadline) {
				t.Errorf("limpieza: el servicio %s sigue ahí (%v)", n.Service, err)
				break
			}
			time.Sleep(time.Second)
		}
		if err := c.RemoveContainer(ctx, n.ProbeContainer, true); err != nil && !errors.Is(err, docker.ErrNotFound) {
			t.Errorf("limpieza: contenedor %s: %v", n.ProbeContainer, err)
		}
		for _, net := range []string{n.PanelNetwork, naming.Network(n.Stack, "sentinel")} {
			if err := untilGone(ctx, func() error { return c.RemoveNetwork(ctx, net) }); err != nil {
				t.Errorf("limpieza: red %s: %v", net, err)
			}
		}
		for _, s := range []string{n.MasterKeySecret, n.InitCloudflareTokenSecret, n.InitRegistrySecret} {
			if err := c.RemoveSecret(ctx, s); err != nil && !errors.Is(err, docker.ErrNotFound) {
				t.Errorf("limpieza: secreto %s: %v", s, err)
			}
		}
		for _, v := range []string{n.DataVolume, n.CaddyAdminVolume} {
			if err := untilGone(ctx, func() error { return c.RemoveVolume(ctx, v, false) }); err != nil {
				t.Errorf("limpieza: volumen %s: %v", v, err)
			}
		}
	})
}

// guardForeign (decisión 5 del plan): el muelle del test tiene docker.sock y
// corre con ingress disabled (el origen es una IP), así que no debería hacer
// nada con Docker por su cuenta. Mira solo dos nombres fijos y una red
// centinela propia con la forma de una red de stage huérfana; nunca crea ni
// borra los nombres fijos. Se llama después de cleanupInstall: su Cleanup
// corre antes, con el muelle todavía arriba.
func guardForeign(t *testing.T, c *docker.Client, p string) {
	t.Helper()
	ctx := context.Background()
	present := func(err error) bool { return err == nil }
	_, err := c.InspectNetwork(ctx, naming.PanelNetwork)
	panelBefore := present(err)
	_, err = c.InspectContainer(ctx, naming.CaddyContainer)
	caddyBefore := present(err)
	sentinel := naming.Network(p, "sentinel")
	if _, err := c.CreateNetwork(ctx, docker.NetworkSpec{Name: sentinel, Driver: "overlay", Attachable: true, Labels: naming.StageLabels(p, "sentinel")}); err != nil {
		t.Fatalf("creando la red centinela: %v", err)
	}
	t.Cleanup(func() {
		if _, err := c.InspectNetwork(ctx, naming.PanelNetwork); !panelBefore && present(err) {
			t.Errorf("apareció la red %s durante el test: el muelle del test tocó Docker con ingress disabled", naming.PanelNetwork)
		}
		if _, err := c.InspectContainer(ctx, naming.CaddyContainer); !caddyBefore && present(err) {
			t.Errorf("apareció el contenedor %s durante el test: el muelle del test tocó Docker con ingress disabled", naming.CaddyContainer)
		}
		if _, err := c.InspectNetwork(ctx, sentinel); errors.Is(err, docker.ErrNotFound) {
			t.Errorf("alguien borró la red centinela %s: el muelle del test limpió redes con ingress disabled", sentinel)
		}
	})
}

// itRun corre install en el proceso del test, con el cliente real.
func itRun(c *docker.Client, o Options) (stdout, stderr string, err error) {
	var out, errOut bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), itInstall)
	defer cancel()
	err = Run(ctx, o, Config{
		Version: "dev", Docker: c, Stdout: &out, Stderr: &errOut,
		// Nunca el config.json de quien corre el test: sus credenciales irían a un secreto.
		DockerConfig: filepath.Join(os.TempDir(), "it5-no-existe", "config.json"),
	})
	return out.String(), errOut.String(), err
}

// checkKeyAndLink verifica la clave (base64 de 32 bytes) y el link en stdout,
// y devuelve el link.
func checkKeyAndLink(t *testing.T, stdout, origin string, wantKey bool) string {
	t.Helper()
	m := keyRe.FindStringSubmatch(stdout)
	if wantKey {
		if m == nil {
			t.Fatalf("stdout sin la clave:\n%s", stdout)
		}
		if raw, err := base64.StdEncoding.DecodeString(m[1]); err != nil || len(raw) != 32 {
			t.Fatalf("clave %q: %v", m[1], err)
		}
	} else if m != nil || strings.Contains(stdout, "Clave maestra") {
		t.Fatalf("stdout con una clave:\n%s", stdout)
	}
	l := linkRe.FindStringSubmatch(stdout)
	if l == nil || !strings.HasPrefix(l[1], origin+"/setup?token=") {
		t.Fatalf("stdout sin el link de %s:\n%s", origin, stdout)
	}
	return l[1]
}

// runningTask es la tarea que corre del servicio, con su contenedor.
func runningTask(ctx context.Context, c *docker.Client, service string) (docker.Task, bool, error) {
	tasks, err := c.ListTasks(ctx, docker.TaskFilter{Service: service, DesiredState: docker.TaskRunning})
	if err != nil {
		return docker.Task{}, false, err
	}
	for _, task := range tasks {
		if task.State == docker.TaskRunning && task.ContainerID != "" {
			return task, true, nil
		}
	}
	return docker.Task{}, false, nil
}

func checkNothingCreated(t *testing.T, c *docker.Client, n naming.InstallNames) {
	t.Helper()
	ctx := context.Background()
	if _, err := c.InspectSecret(ctx, n.MasterKeySecret); !errors.Is(err, docker.ErrNotFound) {
		t.Errorf("el secreto %s existe (%v)", n.MasterKeySecret, err)
	}
	if _, err := c.InspectNetwork(ctx, n.PanelNetwork); !errors.Is(err, docker.ErrNotFound) {
		t.Errorf("la red %s existe (%v)", n.PanelNetwork, err)
	}
	if _, err := c.InspectService(ctx, n.Service); !errors.Is(err, docker.ErrNotFound) {
		t.Errorf("el servicio %s existe (%v)", n.Service, err)
	}
	if _, err := c.InspectContainer(ctx, n.ProbeContainer); !errors.Is(err, docker.ErrNotFound) {
		t.Errorf("el contenedor de prueba %s existe (%v)", n.ProbeContainer, err)
	}
}

func dockerCLI(ctx context.Context, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestIntegrationInstall(t *testing.T) {
	image := itImage()
	c := itDocker(t, image)
	ctx := context.Background()
	p := itPrefix(t)
	n := naming.InstallNamesFor(p)
	cleanupInstall(t, c, n)
	guardForeign(t, c, p)
	const origin = "https://127.0.0.1:22443"
	// Sin token: con uno, el muelle del test podría llamar a Cloudflare.
	opts := Options{PanelOrigin: origin, Email: "it5@example.com", PublicIP: "203.0.113.10",
		HTTPPort: 22080, HTTPSPort: 22443, Image: image, NamePrefix: p}

	var firstLink string
	if !t.Run("install", func(t *testing.T) {
		stdout, stderr, err := itRun(c, opts)
		if err != nil {
			t.Fatalf("%v\nstderr:\n%s", err, stderr)
		}
		firstLink = checkKeyAndLink(t, stdout, origin, true)
		if _, err := c.InspectSecret(ctx, n.MasterKeySecret); err != nil {
			t.Fatalf("secreto: %v", err)
		}
		net, err := c.InspectNetwork(ctx, n.PanelNetwork)
		if err != nil || net.Driver != "overlay" || !net.Attachable || net.Scope != "swarm" || len(net.Subnets) == 0 {
			t.Fatalf("red=%+v err=%v", net, err)
		}
		for _, s := range net.Subnets {
			if pre, err := netip.ParsePrefix(s); err != nil || !pre.Addr().IsPrivate() {
				t.Fatalf("subred no privada: %s", s)
			}
		}
		if _, ok, err := runningTask(ctx, c, n.Service); !ok {
			t.Fatalf("sin una tarea corriendo (%v)", err)
		}
		if _, err := c.InspectContainer(ctx, n.ProbeContainer); !errors.Is(err, docker.ErrNotFound) {
			t.Fatalf("quedó el contenedor de prueba (%v)", err)
		}
	}) {
		return
	}

	// reset-panel-ips dentro del contenedor de verdad (T3), con su entorno.
	t.Run("reset-panel-ips --no-restart", func(t *testing.T) {
		task, ok, err := runningTask(ctx, c, n.Service)
		if !ok {
			t.Fatalf("sin tarea (%v)", err)
		}
		out, err := dockerCLI(ctx, nil, "exec", task.ContainerID, "muelle", "settings", "reset-panel-ips", "--no-restart")
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		// Con --no-restart no toca Docker: el comando nombra el servicio fijo (T3).
		if !strings.Contains(out, "ya estaba vacía") || !strings.Contains(out, "docker service update --force "+naming.MuelleService) {
			t.Fatalf("salida:\n%s", out)
		}
	})

	t.Run("reset-panel-ips reinicia", func(t *testing.T) {
		before, ok, err := runningTask(ctx, c, n.Service)
		if !ok {
			t.Fatalf("sin tarea (%v)", err)
		}
		out, err := dockerCLI(ctx, nil, "exec", before.ContainerID, "muelle", "settings", "reset-panel-ips")
		// Swarm puede cortar el exec al detener la tarea: vale si ya dijo que reiniciaba.
		if err != nil && !strings.Contains(out, "reiniciando") {
			t.Fatalf("%v\n%s", err, out)
		}
		if !strings.Contains(out, "reiniciando "+n.Service) {
			t.Fatalf("salida:\n%s", out)
		}
		eventually(t, itConverge, "una tarea nueva corre", func() (bool, error) {
			task, ok, err := runningTask(ctx, c, n.Service)
			return ok && task.ID != before.ID, err
		})
	})

	t.Run("ya instalado", func(t *testing.T) {
		_, stderr, err := itRun(c, opts)
		if !errors.Is(err, ErrAlreadyInstalled) {
			t.Fatalf("err=%v\nstderr:\n%s", err, stderr)
		}
		if _, err := c.InspectContainer(ctx, n.ProbeContainer); !errors.Is(err, docker.ErrNotFound) {
			t.Fatalf("creó el contenedor de prueba (%v)", err)
		}
	})

	t.Run("reinstall", func(t *testing.T) {
		// Los contenedores de las tareas: docker stack rm vuelve antes de que
		// terminen, y un install en ese momento tendría dos muelle sobre la misma base.
		tasks, err := c.ListTasks(ctx, docker.TaskFilter{Service: n.Service})
		if err != nil {
			t.Fatal(err)
		}
		var containers []string
		for _, task := range tasks {
			if task.ContainerID != "" {
				containers = append(containers, task.ContainerID)
			}
		}
		if out, err := c.RemoveStack(ctx, n.Stack); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		eventually(t, itRelease, "el servicio se borra", func() (bool, error) {
			_, err := c.InspectService(ctx, n.Service)
			return errors.Is(err, docker.ErrNotFound), err
		})
		for _, id := range containers {
			eventually(t, itRelease, "el contenedor "+id[:12]+" termina", func() (bool, error) {
				ctr, err := c.InspectContainer(ctx, id)
				return errors.Is(err, docker.ErrNotFound) || (err == nil && !ctr.Running), err
			})
		}

		_, stderr, err := itRun(c, opts)
		if !errors.Is(err, ErrPreviousData) {
			t.Fatalf("sin --reinstall: err=%v\nstderr:\n%s", err, stderr)
		}
		re := opts
		re.Reinstall = true
		stdout, stderr, err := itRun(c, re)
		if err != nil {
			t.Fatalf("%v\nstderr:\n%s", err, stderr)
		}
		link := checkKeyAndLink(t, stdout, origin, false)
		if link == firstLink {
			t.Fatalf("el link no cambió: %s", link)
		}
		if !strings.Contains(stderr, "la base ya existe") {
			t.Fatalf("stderr:\n%s", stderr)
		}
	})
}

func TestIntegrationInstallPortsBusy(t *testing.T) {
	image := itImage()
	c := itDocker(t, image)
	p := itPrefix(t)
	n := naming.InstallNamesFor(p)
	cleanupInstall(t, c, n)
	ln, err := net.Listen("tcp", "0.0.0.0:22180")
	if err != nil {
		t.Skipf("el puerto 22180 ya está tomado: %v", err)
	}
	defer ln.Close()
	_, stderr, err := itRun(c, Options{PanelOrigin: "https://127.0.0.1:22543", Email: "it5@example.com", PublicIP: "203.0.113.10",
		HTTPPort: 22180, HTTPSPort: 22543, Image: image, NamePrefix: p})
	if !errors.Is(err, ErrPortsBusy) {
		t.Fatalf("err=%v\nstderr:\n%s", err, stderr)
	}
	checkNothingCreated(t, c, n)
}

// Como lo corre un usuario: docker run de la imagen, con el CLI de docker de
// la imagen, su /tmp y sus certificados.
func TestIntegrationInstallFromImage(t *testing.T) {
	image := itImage()
	c := itDocker(t, image, itCaddyImage)
	p := itPrefix(t)
	n := naming.InstallNamesFor(p)
	cleanupInstall(t, c, n)
	guardForeign(t, c, p)
	installer := p + "-installer"
	t.Cleanup(func() {
		if err := c.RemoveContainer(context.Background(), installer, true); err != nil && !errors.Is(err, docker.ErrNotFound) {
			t.Errorf("limpieza: %s: %v", installer, err)
		}
	})
	const origin = "https://127.0.0.1:22444"
	ctx, cancel := context.WithTimeout(context.Background(), itInstall)
	defer cancel()
	// La imagen tiene una versión (v0.0.0-it5) sin imagen de Caddy publicada:
	// --caddy-image apunta a la de prueba, que ya está en el nodo.
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "--name", installer,
		"-v", "/var/run/docker.sock:/var/run/docker.sock", image, "install",
		"--name-prefix", p, "--image", image, "--panel-origin", origin, "--email", "it5@example.com",
		"--public-ip", "203.0.113.10", "--http-port", "22081", "--https-port", "22444", "--caddy-image", itCaddyImage)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	checkKeyAndLink(t, stdout.String(), origin, true)
	if strings.Contains(stdout.String(), "Docker ") || !strings.Contains(stderr.String(), "puertos 22081 y 22444 libres") {
		t.Fatalf("el avance tiene que ir a stderr:\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

// Las imágenes de muelle son privadas: un registry con htpasswd en
// 127.0.0.1:22500 (Docker acepta 127.0.0.0/8 sin TLS) con la imagen de
// prueba. Sin credenciales, install falla antes de crear nada y dice qué
// hacer; con --registry-user, la baja, guarda el secreto del registry y
// despliega.
func TestIntegrationInstallPrivateRegistry(t *testing.T) {
	const (
		registryImage = "registry:2"
		registryAddr  = "127.0.0.1:22500"
		user          = "it5user"
	)
	image := itImage()
	c := itDocker(t, image, registryImage)
	ctx := context.Background()
	p := itPrefix(t)
	n := naming.InstallNamesFor(p)
	password := "it5-" + hex.EncodeToString([]byte(p)) + "-pass"
	private := registryAddr + "/it5-muelle:" + p

	// El registry, con un htpasswd de bcrypt.
	dir := t.TempDir()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "htpasswd"), []byte(user+":"+string(hash)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry := p + "-registry"
	t.Cleanup(func() {
		if out, err := dockerCLI(context.Background(), nil, "rm", "-f", "-v", registry); err != nil && !strings.Contains(out, "No such container") {
			t.Errorf("limpieza: %s: %v\n%s", registry, err, out)
		}
	})
	if out, err := dockerCLI(ctx, nil, "run", "-d", "--name", registry, "-p", registryAddr+":5000",
		"-v", filepath.Join(dir, "htpasswd")+":/auth/htpasswd:ro",
		"-e", "REGISTRY_AUTH=htpasswd", "-e", "REGISTRY_AUTH_HTPASSWD_REALM=it5", "-e", "REGISTRY_AUTH_HTPASSWD_PATH=/auth/htpasswd",
		registryImage); err != nil {
		t.Fatalf("docker run %s: %v\n%s", registryImage, err, out)
	}
	eventually(t, 30*time.Second, "el registry responde", func() (bool, error) {
		res, err := http.Get("http://" + registryAddr + "/v2/")
		if err != nil {
			return false, err
		}
		res.Body.Close()
		return res.StatusCode == http.StatusUnauthorized, fmt.Errorf("status %d", res.StatusCode)
	})

	// La imagen sube al registry con un DOCKER_CONFIG propio (nunca el de
	// quien corre el test), y se le quita el tag en el nodo: install tiene
	// que bajarla.
	t.Cleanup(func() {
		if out, err := dockerCLI(context.Background(), nil, "image", "rm", "-f", private); err != nil && !strings.Contains(out, "No such image") {
			t.Errorf("limpieza: imagen %s: %v\n%s", private, err, out)
		}
	})
	cfg := t.TempDir()
	auth := base64.StdEncoding.EncodeToString([]byte(user + ":" + password))
	if err := os.WriteFile(filepath.Join(cfg, "config.json"), []byte(`{"auths":{"`+registryAddr+`":{"auth":"`+auth+`"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"tag", image, private}, {"push", "-q", private}, {"image", "rm", private}} {
		if out, err := dockerCLI(ctx, []string{"DOCKER_CONFIG=" + cfg}, args...); err != nil {
			t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	cleanupInstall(t, c, n)
	guardForeign(t, c, p)
	const origin = "https://127.0.0.1:22445"
	opts := Options{PanelOrigin: origin, Email: "it5@example.com", PublicIP: "203.0.113.10",
		HTTPPort: 22082, HTTPSPort: 22445, Image: private, NamePrefix: p}

	t.Run("sin credenciales", func(t *testing.T) {
		_, stderr, err := itRun(c, opts)
		if err == nil || !strings.Contains(err.Error(), "docker login ghcr.io") || !strings.Contains(err.Error(), "--registry-user") {
			t.Fatalf("err=%v\nstderr:\n%s", err, stderr)
		}
		checkNothingCreated(t, c, n)
		if _, err := c.InspectSecret(ctx, n.InitRegistrySecret); !errors.Is(err, docker.ErrNotFound) {
			t.Fatalf("el secreto del registry existe (%v)", err)
		}
	})

	t.Run("con credenciales", func(t *testing.T) {
		o := opts
		o.RegistryUser, o.RegistryPassword = user, password
		stdout, stderr, err := itRun(c, o)
		if err != nil {
			t.Fatalf("%v\nstderr:\n%s", err, stderr)
		}
		checkKeyAndLink(t, stdout, origin, true)
		if strings.Contains(stdout+stderr, password) {
			t.Fatal("la contraseña salió en la salida")
		}
		if _, err := c.InspectImage(ctx, private); err != nil {
			t.Fatalf("no bajó la imagen: %v", err)
		}
		if _, err := c.InspectSecret(ctx, n.InitRegistrySecret); err != nil {
			t.Fatalf("sin el secreto del registry: %v", err)
		}
		svc, err := c.InspectService(ctx, n.Service)
		if err != nil || !strings.HasPrefix(svc.Image, private) {
			t.Fatalf("servicio=%+v err=%v", svc, err)
		}
		if task, ok, err := runningTask(ctx, c, n.Service); !ok || !strings.HasPrefix(task.Image, private) {
			t.Fatalf("tarea=%+v ok=%v err=%v", task, ok, err)
		}
	})
}
