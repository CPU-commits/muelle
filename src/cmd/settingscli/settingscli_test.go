package settingscli

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

const muelleImage = "ghcr.io/cpu-commits/muelle:v0.1.0"

// restartEngine es el Engine del fake, con un gancho justo antes de reiniciar.
type restartEngine struct {
	Engine
	beforeRestart func()
}

func (e restartEngine) ForceUpdateService(ctx context.Context, nameOrID string) error {
	if e.beforeRestart != nil {
		e.beforeRestart()
	}
	return e.Engine.ForceUpdateService(ctx, nameOrID)
}

type fixture struct {
	dbPath      string
	fake        *dockertest.Fake
	ctrID       string // el contenedor de la tarea: lo que devuelve Hostname
	dockerCalls int    // cuántas veces se armó el cliente de Docker
	closed      int
	stdout      bytes.Buffer
	stderr      bytes.Buffer
	// stdoutAtRestart es lo que había en stdout cuando se llamó a ForceUpdateService.
	stdoutAtRestart string
	deps            Deps
}

// newFixture arma un DataDir con muelle.db migrada y panel_allowed_cidrs en
// stored (nil: sin la clave), y un Docker con el servicio y el contenedor de
// su tarea, que lleva la etiqueta de Swarm con ese nombre.
func newFixture(t *testing.T, stored *string, service string) *fixture {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	fx := &fixture{dbPath: filepath.Join(dir, "muelle.db"), fake: dockertest.New()}

	db, err := sqlite.Open(fx.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if stored != nil {
		if _, err := db.Conn(ctx).ExecContext(ctx,
			`INSERT INTO settings (key, value, encrypted) VALUES ('panel_allowed_cidrs', ?, 0)`, *stored); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	fx.fake.PutService(docker.Service{Name: service, Version: 3})
	fx.fake.SetImagePresent(muelleImage)
	fx.ctrID, err = fx.fake.CreateContainer(ctx, docker.ContainerSpec{
		Name: service + ".1.abc", Image: muelleImage,
		Labels: map[string]string{naming.LabelSwarmService: service},
	})
	if err != nil {
		t.Fatal(err)
	}

	fx.deps = Deps{
		DataDir:  func() (string, error) { return dir, nil },
		Hostname: func() (string, error) { return fx.ctrID, nil },
		Docker: func() (Engine, func() error, error) {
			fx.dockerCalls++
			e := restartEngine{Engine: fx.fake, beforeRestart: func() { fx.stdoutAtRestart = fx.stdout.String() }}
			return e, func() error { fx.closed++; return nil }, nil
		},
		Log: slog.New(slog.DiscardHandler),
	}
	return fx
}

func (fx *fixture) run(args ...string) int {
	return Run(context.Background(), args, fx.deps, &fx.stdout, &fx.stderr)
}

// stored lee panel_allowed_cidrs y cuántos eventos de auditoría hay.
func (fx *fixture) stored(t *testing.T) (value string, events int) {
	t.Helper()
	ctx := context.Background()
	db, err := sqlite.Open(fx.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Conn(ctx).QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'panel_allowed_cidrs'`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if err := db.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	return value, events
}

func (fx *fixture) restarted() []string {
	var out []string
	for _, c := range fx.fake.Calls() {
		if strings.HasPrefix(c, "ForceUpdateService ") {
			out = append(out, c)
		}
	}
	return out
}

func TestRunUsage(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
	}{
		{nil, 2},
		{[]string{"x"}, 2},
		{[]string{"reset-panel-ips", "extra"}, 2},
		{[]string{"reset-panel-ips", "--nope"}, 2},
		{[]string{"-h"}, 0},
		{[]string{"--help"}, 0},
		{[]string{"help"}, 0},
		{[]string{"reset-panel-ips", "-h"}, 0},
	} {
		// Los argumentos se revisan antes de tocar nada.
		d := Deps{
			DataDir:  func() (string, error) { t.Errorf("%q: leyó DataDir", tc.args); return "", errors.New("no") },
			Hostname: func() (string, error) { t.Errorf("%q: leyó Hostname", tc.args); return "", errors.New("no") },
			Docker: func() (Engine, func() error, error) {
				t.Errorf("%q: armó Docker", tc.args)
				return nil, nil, errors.New("no")
			},
		}
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), tc.args, d, &stdout, &stderr)
		if code != tc.code {
			t.Errorf("%q: código %d, esperaba %d", tc.args, code, tc.code)
		}
		// La ayuda pedida va a stdout; la de un error, a stderr.
		out, other := &stderr, &stdout
		if tc.code == 0 {
			out, other = &stdout, &stderr
		}
		if !strings.Contains(out.String(), Usage) {
			t.Errorf("%q: falta el uso: %q", tc.args, out.String())
		}
		if other.Len() != 0 {
			t.Errorf("%q: salida inesperada: %q", tc.args, other.String())
		}
	}
}

// Vacía la lista, lo dice y reinicia el servicio de su contenedor.
func TestResetClearsAndRestarts(t *testing.T) {
	fx := newFixture(t, new(`["190.5.6.0/24"]`), naming.MuelleService)
	if code := fx.run("reset-panel-ips"); code != 0 {
		t.Fatalf("código %d\nstdout: %s\nstderr: %s", code, fx.stdout.String(), fx.stderr.String())
	}
	out := fx.stdout.String()
	if !strings.Contains(out, "190.5.6.0/24") || !strings.Contains(out, "reiniciando "+naming.MuelleService) {
		t.Errorf("stdout=%q", out)
	}
	if fx.stderr.Len() != 0 {
		t.Errorf("stderr=%q", fx.stderr.String())
	}
	if v, events := fx.stored(t); v != "[]" || events != 1 {
		t.Errorf("guardado %q con %d eventos, esperaba [] y 1", v, events)
	}
	if got := fx.restarted(); !slices.Equal(got, []string{"ForceUpdateService " + naming.MuelleService}) {
		t.Errorf("reinicios: %q", got)
	}
	// Swarm puede cortar este docker exec al detener la tarea: el aviso sale antes.
	if !strings.Contains(fx.stdoutAtRestart, "reiniciando "+naming.MuelleService) {
		t.Errorf("al reiniciar, stdout era %q", fx.stdoutAtRestart)
	}
	if fx.dockerCalls != 1 || fx.closed != 1 {
		t.Errorf("Docker armado %d veces y cerrado %d", fx.dockerCalls, fx.closed)
	}
}

// Con la lista ya vacía no escribe nada, pero reinicia igual: el arranque le
// reenvía la configuración a Caddy (contrato §5.4).
func TestResetEmptyListStillRestarts(t *testing.T) {
	fx := newFixture(t, new("[]"), naming.MuelleService)
	if code := fx.run("reset-panel-ips"); code != 0 {
		t.Fatalf("código %d\nstderr: %s", code, fx.stderr.String())
	}
	out := fx.stdout.String()
	if !strings.Contains(out, "ya estaba vacía") || !strings.Contains(out, "reiniciando "+naming.MuelleService) {
		t.Errorf("stdout=%q", out)
	}
	if v, events := fx.stored(t); v != "[]" || events != 0 {
		t.Errorf("guardado %q con %d eventos, esperaba [] y ninguno", v, events)
	}
	if len(fx.restarted()) != 1 {
		t.Errorf("reinicios: %q", fx.restarted())
	}
}

// --no-restart solo cambia la base y dice cómo reiniciar a mano, sin Docker.
func TestResetNoRestart(t *testing.T) {
	fx := newFixture(t, new(`["190.5.6.0/24"]`), naming.MuelleService)
	if code := fx.run("reset-panel-ips", "--no-restart"); code != 0 {
		t.Fatalf("código %d\nstderr: %s", code, fx.stderr.String())
	}
	if fx.dockerCalls != 0 || len(fx.fake.Calls()) != 1 { // la de CreateContainer del fixture
		t.Errorf("usó Docker: %d %q", fx.dockerCalls, fx.fake.Calls())
	}
	if out := fx.stdout.String(); !strings.Contains(out, "docker service update --force "+naming.MuelleService) {
		t.Errorf("stdout=%q", out)
	}
	if v, _ := fx.stored(t); v != "[]" {
		t.Errorf("guardado %q", v)
	}
}

// El servicio es el de la etiqueta de su contenedor, no un nombre fijo.
func TestResetUsesServiceFromLabel(t *testing.T) {
	fx := newFixture(t, new(`["190.5.6.0/24"]`), "it5abc_muelle")
	if code := fx.run("reset-panel-ips"); code != 0 {
		t.Fatalf("código %d\nstderr: %s", code, fx.stderr.String())
	}
	if got := fx.restarted(); !slices.Equal(got, []string{"ForceUpdateService it5abc_muelle"}) {
		t.Errorf("reinicios: %q", got)
	}
	if out := fx.stdout.String(); !strings.Contains(out, "reiniciando it5abc_muelle") {
		t.Errorf("stdout=%q", out)
	}
}

// Si no puede reiniciar, la base igual quedó cambiada: lo dice, da el comando
// para reiniciar a mano y termina con 1.
func TestResetRestartFails(t *testing.T) {
	for _, tc := range []struct {
		name    string
		setup   func(t *testing.T, fx *fixture)
		service string // el del comando a mano
	}{
		{"contenedor sin la etiqueta", func(t *testing.T, fx *fixture) {
			id, err := fx.fake.CreateContainer(context.Background(), docker.ContainerSpec{Name: "suelto", Image: muelleImage})
			if err != nil {
				t.Fatal(err)
			}
			fx.deps.Hostname = func() (string, error) { return id, nil }
		}, naming.MuelleService},
		{"hostname que no es un contenedor", func(t *testing.T, fx *fixture) {
			fx.deps.Hostname = func() (string, error) { return "otra-maquina", nil }
		}, naming.MuelleService},
		{"sin hostname", func(t *testing.T, fx *fixture) {
			fx.deps.Hostname = func() (string, error) { return "", errors.New("sin hostname") }
		}, naming.MuelleService},
		{"Docker no responde", func(t *testing.T, fx *fixture) { fx.fake.SetUnavailable(true) }, naming.MuelleService},
		{"no se puede armar el cliente", func(t *testing.T, fx *fixture) {
			fx.deps.Docker = func() (Engine, func() error, error) { return nil, nil, errors.New("DOCKER_HOST no sirve") }
		}, naming.MuelleService},
		{"el servicio no existe", func(t *testing.T, fx *fixture) {
			fx.fake.FailNext("ForceUpdateService", docker.ErrNotFound)
		}, naming.MuelleService},
		{"falla el reinicio de otro servicio", func(t *testing.T, fx *fixture) {
			fx.fake.FailNext("ForceUpdateService", docker.ErrUnavailable)
			fx.fake.PutService(docker.Service{Name: "it5abc_muelle"})
			id, err := fx.fake.CreateContainer(context.Background(), docker.ContainerSpec{
				Name: "it5abc_muelle.1.x", Image: muelleImage, Labels: map[string]string{naming.LabelSwarmService: "it5abc_muelle"},
			})
			if err != nil {
				t.Fatal(err)
			}
			fx.deps.Hostname = func() (string, error) { return id, nil }
		}, "it5abc_muelle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newFixture(t, new(`["190.5.6.0/24"]`), naming.MuelleService)
			tc.setup(t, fx)
			if code := fx.run("reset-panel-ips"); code != 1 {
				t.Fatalf("código %d, esperaba 1\nstdout: %s\nstderr: %s", code, fx.stdout.String(), fx.stderr.String())
			}
			if errOut := fx.stderr.String(); !strings.Contains(errOut, "docker service update --force "+tc.service) {
				t.Errorf("stderr=%q", errOut)
			}
			if v, _ := fx.stored(t); v != "[]" {
				t.Errorf("guardado %q, esperaba []", v)
			}
		})
	}
}

// Fuera del contenedor de muelle (sin su entorno o sin su base), aborta sin
// tocar nada y dice que se corre con docker exec.
func TestResetOutsideContainer(t *testing.T) {
	noDocker := func() (Engine, func() error, error) {
		t.Error("armó Docker")
		return nil, nil, errors.New("no")
	}
	t.Run("sin entorno", func(t *testing.T) {
		d := Deps{DataDir: func() (string, error) { return "", errors.New("MUELLE_PANEL_ORIGIN: es obligatorio") }, Docker: noDocker}
		var stdout, stderr bytes.Buffer
		if code := Run(context.Background(), []string{"reset-panel-ips"}, d, &stdout, &stderr); code != 1 {
			t.Fatalf("código %d", code)
		}
		if !strings.Contains(stderr.String(), "MUELLE_PANEL_ORIGIN") || !strings.Contains(stderr.String(), "docker exec") {
			t.Errorf("stderr=%q", stderr.String())
		}
	})
	t.Run("sin la base", func(t *testing.T) {
		dir := t.TempDir()
		d := Deps{DataDir: func() (string, error) { return dir, nil }, Docker: noDocker}
		var stdout, stderr bytes.Buffer
		if code := Run(context.Background(), []string{"reset-panel-ips"}, d, &stdout, &stderr); code != 1 {
			t.Fatalf("código %d", code)
		}
		if !strings.Contains(stderr.String(), "docker exec") {
			t.Errorf("stderr=%q", stderr.String())
		}
		// sqlite.Open la habría creado vacía.
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 0 {
			t.Errorf("creó archivos en DataDir: %v %v", entries, err)
		}
	})
}
