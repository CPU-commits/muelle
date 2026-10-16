package service

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/shared/domain/compose"
)

func formApp(spec string) App {
	return App{ID: 7, Project: "auditfy", Stage: "staging", Slug: "api", Kind: KindService, Spec: []byte(spec), DeployTimeoutS: 300}
}

func composeApp(src string) App {
	return App{ID: 8, Project: "auditfy", Stage: "staging", Slug: "front", Kind: KindCompose, ComposeYAML: src, DeployTimeoutS: 300}
}

// deploy no es dueño del formato del spec: lo lee sin rechazar campos (uno
// nuevo de catalog no rompe los deploys).
func TestParseFormSpec(t *testing.T) {
	s, err := ParseFormSpec([]byte(`{"image":"nginx","port":80,"replicas":2,"command":["a"],"nuevo":true,
		"healthcheck":{"type":"http","path":"/h","interval_s":10,"timeout_s":5,"retries":3,"start_period_s":20},
		"volumes":[{"name":"data","mount":"/data"}],"resources":{"memory_mb":512,"cpus":0.5}}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Image != "nginx" || s.Port != 80 || *s.Replicas != 2 || s.Healthcheck.Path != "/h" || s.Resources.CPUs != 0.5 || s.Volumes[0].Mount != "/data" {
		t.Fatalf("%+v", s)
	}
	for _, raw := range []string{"", "null", "[]", `{"image":`, `{"port":"80"}`} {
		if _, err := ParseFormSpec([]byte(raw)); err == nil {
			t.Errorf("%q: debió fallar", raw)
		}
	}
}

func TestReadConfigForm(t *testing.T) {
	env := map[string]string{"DATABASE_URL": "postgres://u:p@db/x", "PRICE": "$5"}
	app := formApp(`{"image":"ghcr.io/x/api:staging","port":3000,"replicas":2,"command":["bun","run","start"],
		"healthcheck":{"type":"http","path":"/health","interval_s":10,"timeout_s":5,"retries":3,"start_period_s":90},
		"volumes":[{"name":"data","mount":"/data"}],"resources":{"memory_mb":512,"cpus":0.1}}`)
	cfg, err := ReadConfig(context.Background(), app, env)
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(cfg.Images, map[string]string{"api": "ghcr.io/x/api:staging"}) || !cfg.HasVolumes {
		t.Fatalf("images=%v volumes=%v", cfg.Images, cfg.HasVolumes)
	}
	svc, ok := cfg.File.Services["api"]
	if !ok || len(cfg.File.Services) != 1 {
		t.Fatalf("servicios: %v", slices.Collect(maps.Keys(cfg.File.Services)))
	}
	if !maps.Equal(svc.Environment, env) {
		t.Errorf("environment = %v", svc.Environment)
	}
	env["NUEVA"] = "x" // la Config no comparte el mapa
	if _, ok := svc.Environment["NUEVA"]; ok {
		t.Error("comparte el mapa de variables")
	}
	if !slices.Equal(svc.Command, []string{"bun", "run", "start"}) {
		t.Errorf("command = %v", svc.Command)
	}
	retries := uint64(3)
	wantHC := &compose.Healthcheck{
		Test:     []string{"CMD", "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:3000/health"},
		Interval: "10s", Timeout: "5s", Retries: &retries, StartPeriod: "1m30s",
	}
	if !reflect.DeepEqual(svc.Healthcheck, wantHC) {
		t.Errorf("healthcheck = %+v", svc.Healthcheck)
	}
	if svc.Deploy == nil || *svc.Deploy.Replicas != 2 ||
		*svc.Deploy.Resources.Limits != (compose.Resource{CPUs: "0.1", Memory: "536870912"}) {
		t.Errorf("deploy = %+v", svc.Deploy)
	}
	if !reflect.DeepEqual(svc.Volumes, []compose.Mount{{Type: "volume", Source: "data", Target: "/data"}}) {
		t.Errorf("volumes = %+v", svc.Volumes)
	}
	if _, ok := cfg.File.Volumes["data"]; !ok || len(cfg.File.Volumes) != 1 {
		t.Errorf("volumes de primer nivel = %v", cfg.File.Volumes)
	}
}

func TestReadConfigFormHealthchecks(t *testing.T) {
	base := `"image":"oven/bun:1","port":8080,"replicas":1,"command":[],"volumes":[]`
	cases := map[string][]string{
		`{"type":"bun-http","path":"/up?x=1","interval_s":10,"timeout_s":5,"retries":3,"start_period_s":20}`: {
			"CMD", "bun", "-e", "fetch('http://127.0.0.1:8080/up?x=1').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"},
		`{"type":"command","command":["sh","-c","test -f /tmp/ok"],"interval_s":10,"timeout_s":5,"retries":3,"start_period_s":20}`: {
			"CMD", "sh", "-c", "test -f /tmp/ok"},
		`{"type":"none"}`: nil,
	}
	for hc, want := range cases {
		cfg, err := ReadConfig(context.Background(), formApp(`{`+base+`,"healthcheck":`+hc+`}`), nil)
		if err != nil {
			t.Fatal(err)
		}
		svc := cfg.File.Services["api"]
		if want == nil {
			if svc.Healthcheck != nil {
				t.Errorf("none: %+v", svc.Healthcheck)
			}
			continue
		}
		if svc.Healthcheck == nil || !slices.Equal(svc.Healthcheck.Test, want) {
			t.Errorf("%s: %+v", hc, svc.Healthcheck)
		}
		// Sin comando ni variables, no van; sin volúmenes, tampoco.
		if svc.Command != nil || svc.Environment != nil || cfg.HasVolumes || cfg.File.Volumes != nil {
			t.Errorf("%+v", svc)
		}
	}
}

func TestReadConfigCompose(t *testing.T) {
	src := "services:\n  web:\n    image: ghcr.io/x/web:${TAG}\n    volumes:\n      - data:/data\n  api:\n    image: ghcr.io/x/api:1\n    environment:\n      DB: ${DB_URL}\nvolumes:\n  data: {}\n"
	cfg, err := ReadConfig(context.Background(), composeApp(src), map[string]string{"TAG": "sha-1", "DB_URL": "postgres://x"})
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(cfg.Images, map[string]string{"web": "ghcr.io/x/web:sha-1", "api": "ghcr.io/x/api:1"}) || !cfg.HasVolumes {
		t.Fatalf("images=%v volumes=%v", cfg.Images, cfg.HasVolumes)
	}
	if cfg.File.Services["api"].Environment["DB"] != "postgres://x" {
		t.Fatalf("%+v", cfg.File.Services["api"])
	}
	// Un tmpfs no es un volumen nombrado.
	cfg, err = ReadConfig(context.Background(), composeApp("services:\n  web:\n    image: nginx\n    tmpfs:\n      - /tmp\n"), nil)
	if err != nil || cfg.HasVolumes {
		t.Fatalf("HasVolumes=%v err=%v", cfg.HasVolumes, err)
	}
}

// Los errores de ReadConfig son *RenderError: el deploy falla con
// "render: …" y Swarm no se toca (contrato §2.11).
func TestReadConfigErrors(t *testing.T) {
	cases := []struct {
		name string
		app  App
		env  map[string]string
		want string
	}{
		{"variable sin definir", composeApp("services:\n  web:\n    image: nginx:${TAG}\n"), nil,
			"render: compose_yaml: services.web.image: "},
		{"driver de logging que llega por una variable", composeApp("services:\n  web:\n    image: nginx\n    logging:\n      driver: ${D}\n"),
			map[string]string{"D": "syslog"}, "render: compose_yaml: services.web.logging.driver"},
		{"spec que no se puede leer", formApp(`{"image":`), nil, "render: spec: "},
		{"spec sin imagen", formApp(`{"port":80}`), nil, "render: spec: "},
		{"tipo desconocido", App{Slug: "x", Kind: "otro"}, nil, "render: "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ReadConfig(context.Background(), c.app, c.env)
			re, ok := errors.AsType[*RenderError](err)
			if !ok {
				t.Fatalf("esperaba *RenderError, llegó %v", err)
			}
			if !strings.HasPrefix(re.Error(), c.want) {
				t.Fatalf("%q no empieza con %q", re.Error(), c.want)
			}
		})
	}
	// Un ctx cancelado no es un error de render.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ReadConfig(ctx, composeApp("services:\n  web:\n    image: nginx\n"), nil)
	if _, ok := errors.AsType[*RenderError](err); ok || !errors.Is(err, context.Canceled) {
		t.Fatalf("ctx cancelado: %v", err)
	}
}
