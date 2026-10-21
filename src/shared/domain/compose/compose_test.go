package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// wantInvalid exige un *validate.Error en Field cuyo texto completo
// ("compose_yaml: …") contenga cada uno de parts. Lo devuelve.
func wantInvalid(t *testing.T, err error, parts ...string) *validate.Error {
	t.Helper()
	ve, ok := errors.AsType[*validate.Error](err)
	if !ok {
		t.Fatalf("esperaba *validate.Error, got %v", err)
	}
	if ve.Field != Field {
		t.Fatalf("field = %q, esperaba %q (%v)", ve.Field, Field, err)
	}
	for _, p := range parts {
		if !strings.Contains(ve.Error(), p) {
			t.Fatalf("el error %q no contiene %q", ve.Error(), p)
		}
	}
	return ve
}

func readKitchenSink(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/kitchen-sink.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// web arma un compose con un servicio web (image nginx) y extra como sus
// claves, ya indentadas a 4 espacios.
func web(extra string) string {
	return "services:\n  web:\n    image: nginx\n" + extra
}

func TestValidateKitchenSink(t *testing.T) {
	names, err := Validate(readKitchenSink(t))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"api", "cron", "worker"}; !slices.Equal(names, want) {
		t.Fatalf("names = %v, esperaba %v", names, want)
	}
}

func TestValidateInterpolationNotNeeded(t *testing.T) {
	src := "services:\n  web:\n    image: ghcr.io/x/api:${TAG}\n    deploy:\n      replicas: ${N:-1}\n" +
		"    logging:\n      driver: ${D}\n    environment:\n      PRICE: $$5\n      HOME_DIR: $HOME\n"
	names, err := Validate(src)
	if err != nil || !slices.Equal(names, []string{"web"}) {
		t.Fatalf("names=%v err=%v", names, err)
	}
	// Lo que es error de sintaxis sin importar los valores falla al guardar.
	for _, expr := range []string{"${A:-${B}}", "${A:-$B}", "${}", "${A B}"} {
		t.Run(expr, func(t *testing.T) {
			_, err := Validate(web("    environment:\n      X: \"" + expr + "\"\n"))
			wantInvalid(t, err, "compose_yaml: services.web.environment.X: ")
		})
	}
}

func TestValidateSizeAndCount(t *testing.T) {
	for _, src := range []string{"", "  \n"} {
		_, err := Validate(src)
		if ve := wantInvalid(t, err); ve.Error() != "compose_yaml: es obligatorio" {
			t.Errorf("%q: %v", src, ve)
		}
	}
	_, err := Validate(strings.Repeat("a", MaxBytes+1))
	if ve := wantInvalid(t, err); ve.Error() != "compose_yaml: no puede superar 256 KiB" {
		t.Errorf("tamaño: %v", ve)
	}
	// MaxBytes exactos sí (con un comentario de relleno).
	src := web("")
	src += "#" + strings.Repeat("x", MaxBytes-len(src)-2) + "\n"
	if len(src) != MaxBytes {
		t.Fatalf("len = %d", len(src))
	}
	if _, err := Validate(src); err != nil {
		t.Errorf("256 KiB exactos deben aceptarse: %v", err)
	}

	_, err = Validate("services: {}\n")
	wantInvalid(t, err, "al menos un servicio")
	_, err = Validate("volumes:\n  data: {}\n")
	wantInvalid(t, err, "al menos un servicio")

	var b strings.Builder
	b.WriteString("services:\n")
	for i := range 33 {
		fmt.Fprintf(&b, "  s%d:\n    image: nginx\n", i)
	}
	_, err = Validate(b.String())
	wantInvalid(t, err, "a lo más 32")

	_, err = Validate("services: [\n")
	wantInvalid(t, err, "compose_yaml: el compose no es válido: ")
	// Un tipo equivocado lo rechaza el esquema de compose-go, sin panic.
	_, err = Validate(web("    networks: [1]\n"))
	wantInvalid(t, err, "compose_yaml: el compose no es válido: ")
}

func TestValidateNames(t *testing.T) {
	for _, name := range []string{"Web", "a_b", "abcdefghijklmnopq"} {
		_, err := Validate("services:\n  " + name + ":\n    image: nginx\n")
		wantInvalid(t, err, `"`+name+`"`, "servicio")
	}
	_, err := Validate(web("") + "volumes:\n  Data: {}\n")
	wantInvalid(t, err, `"Data"`, "volumen")
	// Una imagen inválida (sin variables) también falla al guardar.
	_, err = Validate("services:\n  web:\n    image: Mala Imagen\n")
	wantInvalid(t, err, "services.web.image: ")
	_, err = Validate("services:\n  web:\n    command: x\n")
	wantInvalid(t, err, "services.web.image: ")
}

func TestValidateNeverReadsFiles(t *testing.T) {
	const marker = "MARCADOR-QUE-NO-DEBE-LEERSE"
	path := filepath.Join(t.TempDir(), "x.yaml")
	if err := os.WriteFile(path, []byte(marker+": 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct{ src, reason string }{
		"include":             {"include: [" + path + "]\n" + web(""), "archivos del servidor"},
		"extends":             {web("    extends: {file: " + path + ", service: x}\n"), "archivos del servidor"},
		"env_file":            {web("    env_file: " + path + "\n"), "archivos del servidor"},
		"label_file":          {web("    label_file: " + path + "\n"), "archivos del servidor"},
		"secrets":             {web("    secrets: [s]\n") + "secrets:\n  s:\n    file: " + path + "\n", "secrets y configs"},
		"configs":             {web("    configs: [c]\n") + "configs:\n  c:\n    file: " + path + "\n", "secrets y configs"},
		"include sin archivo": {"include: [/nonexistent/muelle/x.yaml]\n" + web(""), "archivos del servidor"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Validate(c.src)
			ve := wantInvalid(t, err, c.reason)
			if strings.Contains(ve.Error(), marker) || strings.Contains(ve.Error(), "no such file") {
				t.Fatalf("el error leyó un archivo: %v", ve)
			}
			// Load tampoco los lee.
			_, err = Load(context.Background(), c.src, "p_s_a", map[string]string{})
			ve = wantInvalid(t, err, c.reason)
			if strings.Contains(ve.Error(), marker) || strings.Contains(ve.Error(), "no such file") {
				t.Fatalf("Load leyó un archivo: %v", ve)
			}
		})
	}
}

func TestLoadUsesOnlyGivenEnv(t *testing.T) {
	ctx := context.Background()
	t.Setenv("MUELLE_TEST_SECRETO", "x")
	_, err := Load(ctx, web("    environment:\n      S: ${MUELLE_TEST_SECRETO}\n"), "p_s_a", map[string]string{})
	wantInvalid(t, err, "la variable MUELLE_TEST_SECRETO no está definida")

	src := web("    environment:\n      P: ${COMPOSE_PROJECT_NAME}\n")
	for name, env := range map[string]map[string]string{
		"vacío":            {},
		"nil":              nil,
		"con el suyo":      {"COMPOSE_PROJECT_NAME": "otro"},
		"con otras claves": {"A": "1"},
	} {
		t.Run(name, func(t *testing.T) {
			before := maps.Clone(env)
			f, err := Load(ctx, src, "p_s_a", env)
			if err != nil {
				t.Fatal(err)
			}
			if got := f.Services["web"].Environment["P"]; got != "p_s_a" {
				t.Fatalf("COMPOSE_PROJECT_NAME = %q", got)
			}
			if !maps.Equal(env, before) || (env == nil) != (before == nil) {
				t.Fatalf("env cambió: %v → %v", before, env)
			}
		})
	}
}

// Una variable de environment sin valor (- X o X:) toma la de la app, como
// en compose con el entorno de la shell; si la app no la tiene, no va: nunca
// sale del entorno de muelle (docker stack deploy la tomaría de ahí).
func TestLoadEnvironmentWithoutValue(t *testing.T) {
	t.Setenv("MUELLE_TEST_PROC", "del-proceso")
	src := web("    environment:\n      X:\n      MUELLE_TEST_PROC:\n") +
		"  api:\n    image: nginx\n    environment: [Y, MUELLE_TEST_PROC]\n"
	f, err := Load(context.Background(), src, "p_s_a", map[string]string{"X": "1", "Y": "2"})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Services["web"].Environment; !maps.Equal(got, map[string]string{"X": "1"}) {
		t.Errorf("web: %v", got)
	}
	if got := f.Services["api"].Environment; !maps.Equal(got, map[string]string{"Y": "2"}) {
		t.Errorf("api: %v", got)
	}
	f, err = Load(context.Background(), src, "p_s_a", nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.Services["web"].Environment != nil || f.Services["api"].Environment != nil {
		t.Errorf("sin variables no va ninguna: %+v", f.Services)
	}
}

func TestLoadModel(t *testing.T) {
	env := map[string]string{"TAG": "2.0", "SECRET": "s3cr$t", "B": "bval"}
	f, err := Load(context.Background(), readKitchenSink(t), "p_s_a", env)
	if err != nil {
		t.Fatal(err)
	}
	u := func(n uint64) *uint64 { return &n }
	fl := func(n float64) *float64 { return &n }
	yes := true
	want := &File{
		Services: map[string]Service{
			"api": {
				Image:      "ghcr.io/acme/api:2.0",
				Command:    []string{"./api", "--port", "3000", "--price=$5"},
				Entrypoint: []string{"/bin/entrypoint"},
				// NOVALUE no tiene valor ni está en env: no va (docker stack
				// deploy lo tomaría del entorno de muelle).
				Environment: map[string]string{"DATABASE_URL": "postgres://db/app", "EMPTY": "", "SECRET": "s3cr$t"},
				Healthcheck: &Healthcheck{
					Test:     []string{"CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:3000/health"},
					Interval: "10s", Timeout: "5s", Retries: u(3), StartPeriod: "20s", StartInterval: "2s",
				},
				Deploy: &Deploy{
					Mode: "replicated", Replicas: u(2), EndpointMode: "vip",
					Labels: map[string]string{"com.example.tier": "web"},
					Placement: &Placement{
						Constraints:        []string{"node.role == worker"},
						Preferences:        []Preference{{Spread: "node.labels.zone"}},
						MaxReplicasPerNode: 1,
					},
					Resources: &Resources{
						Limits:       &Resource{CPUs: "0.5", Memory: "536870912"},
						Reservations: &Resource{CPUs: "0.1", Memory: "134217728"},
					},
					RestartPolicy: &RestartPolicy{Condition: "on-failure", Delay: "5s", MaxAttempts: u(3), Window: "2m0s"},
					UpdateConfig: &UpdateConfig{
						Parallelism: u(2), Delay: "10s", FailureAction: "rollback", Monitor: "30s",
						MaxFailureRatio: fl(0.25), Order: "start-first",
					},
					RollbackConfig: &UpdateConfig{
						Parallelism: u(1), FailureAction: "pause", Monitor: "10s", Order: "stop-first",
					},
				},
				Labels:  map[string]string{"com.example.team": "backend", "com.example.price": "$10"},
				Logging: &Logging{Driver: "json-file", Options: map[string]string{"max-size": "5m", "max-file": "2"}},
				Volumes: []Mount{
					{Type: "volume", Source: "data", Target: "/data"},
					{Type: "volume", Source: "cache", Target: "/cache", ReadOnly: true, Volume: &MountVolume{NoCopy: true}},
					{Type: "tmpfs", Target: "/scratch", Tmpfs: &MountTmpfs{Size: 1048576, Mode: 1777}},
				},
				Tmpfs:           []string{"/run", "/tmp"},
				User:            "1000:1000",
				WorkingDir:      "/app",
				StopGracePeriod: "30s",
				StopSignal:      "SIGTERM",
				ReadOnly:        true,
				Init:            &yes,
				Ulimits:         map[string]Ulimit{"nproc": {Soft: 65535, Hard: 65535}, "nofile": {Soft: 20000, Hard: 40000}},
				CapDrop:         []string{"ALL"},
				Hostname:        "api",
				Domainname:      "example.internal",
				ExtraHosts:      []string{"otherhost:50.31.209.229", "somehost:162.242.195.82"},
				DNS:             []string{"1.1.1.1", "8.8.8.8"},
				DNSSearch:       []string{"example.internal"},
				Expose:          []string{"3000", "9090"},
				Tty:             true,
				StdinOpen:       true,
			},
			"worker": {
				Image:      "ghcr.io/acme/worker@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				Command:    []string{"./worker", "--queue", "default"},
				Entrypoint: []string{"/bin/sh", "-c"},
				// B sin valor toma el de la variable B de la app, como en
				// compose con el entorno de la shell.
				Environment: map[string]string{"A": "1", "B": "bval", "C": "con=igual"},
				Labels:      map[string]string{"com.example.role": "worker", "com.example.flag": ""},
				Logging:     &Logging{Driver: "local"},
				Deploy:      &Deploy{Mode: "global"},
				ExtraHosts:  []string{"host.one:10.0.0.1", "host.two:10.0.0.2"},
				DNS:         []string{"9.9.9.9"},
				DNSSearch:   []string{"a.internal", "b.internal"},
				Tmpfs:       []string{"/var/cache"},
			},
			"cron": {
				Image:   "busybox:1.37",
				Command: []string{"sh", "-c", "echo $HOME"},
				Logging: &Logging{Driver: "none"},
			},
		},
		Volumes: map[string]Volume{
			"data":  {Labels: map[string]string{"com.example.backup": "daily"}},
			"cache": {Labels: map[string]string{"com.example.backup": "never"}},
		},
	}
	if !reflect.DeepEqual(f, want) {
		got, _ := json.MarshalIndent(f, "", "  ")
		exp, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("File distinto.\ngot:\n%s\nwant:\n%s", got, exp)
	}
	// Lo que se descarta no aparece en el compose final.
	out, err := Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"depends_on", `"restart"`, "platform", "x-notes", "x-common", "version"} {
		if strings.Contains(string(out), k) {
			t.Errorf("el compose final no debe tener %s:\n%s", k, out)
		}
	}
}

func TestLoadChecksInterpolatedValues(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		src, path string
		env       map[string]string
	}{
		{web("    logging:\n      driver: ${D}\n"), "services.web.logging.driver", map[string]string{"D": "syslog"}},
		{"services:\n  web:\n    image: ${IMG}\n", "services.web.image", map[string]string{"IMG": "Mala Imagen"}},
		{web("    labels:\n      - ${K}=x\n"), "services.web.labels", map[string]string{"K": "muelle.app"}},
		{web("    volumes:\n      - ${V}:/x\n"), "services.web.volumes", map[string]string{"V": "/var/run/docker.sock"}},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			if _, err := Validate(c.src); err != nil {
				t.Fatalf("al guardar debe pasar: %v", err)
			}
			_, err := Load(ctx, c.src, "p_s_a", c.env)
			wantInvalid(t, err, "compose_yaml: "+c.path)
		})
	}
}
