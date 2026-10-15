package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/shared/domain/compose"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
)

var update = flag.Bool("update", false, "reescribe los golden")

// Los golden usan el prefijo de las pruebas de esta línea (it2): el test de
// integración se los pasa a docker stack config, que no crea nada.
const (
	goldenProject  = "it2g"
	goldenStage    = "staging"
	goldenDeployID = 145
)

// goldenInput es testdata/render/<caso>.input.json.
type goldenInput struct {
	Kind          string            `json:"kind"`
	Slug          string            `json:"slug"`
	Spec          json.RawMessage   `json:"spec"`
	Compose       string            `json:"compose"`
	Env           map[string]string `json:"env"`
	Images        map[string]string `json:"images"`
	StorageNodeID string            `json:"storage_node_id"`
	Domains       []struct {
		Hostname string `json:"hostname"`
		Service  string `json:"service"`
		Port     int    `json:"port"`
	} `json:"domains"`
}

// renderGolden lee un input y lo renderiza como lo haría el paso 3.
func renderGolden(t *testing.T, path string) (Rendered, error) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var in goldenInput
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	app := App{ID: 1, Project: goldenProject, Stage: goldenStage, Slug: in.Slug, Kind: in.Kind, Spec: in.Spec, ComposeYAML: in.Compose, DeployTimeoutS: 300}
	if app.Slug == "" {
		app.Slug = "api"
	}
	cfg, err := ReadConfig(context.Background(), app, in.Env)
	if err != nil {
		return Rendered{}, err
	}
	var domains []DomainTarget
	for _, d := range in.Domains {
		domains = append(domains, DomainTarget{Hostname: d.Hostname, Service: d.Service, Port: d.Port})
	}
	return Render(cfg, RenderInput{App: app, DeployID: goldenDeployID, Images: in.Images, StorageNodeID: in.StorageNodeID, Domains: domains})
}

func goldenCases(t *testing.T) []string {
	t.Helper()
	inputs, err := filepath.Glob("testdata/render/*.input.json")
	if err != nil || len(inputs) == 0 {
		t.Fatalf("sin casos: %v", err)
	}
	return inputs
}

// TestRenderGolden compara el compose final de cada caso byte a byte con su
// golden. go test ./src/internal/deploy/domain/service -run TestRenderGolden -update
// los reescribe.
func TestRenderGolden(t *testing.T) {
	for _, input := range goldenCases(t) {
		name := strings.TrimSuffix(filepath.Base(input), ".input.json")
		t.Run(name, func(t *testing.T) {
			r, err := renderGolden(t, input)
			if err != nil {
				t.Fatal(err)
			}
			golden := strings.TrimSuffix(input, ".input.json") + ".golden.json"
			if *update {
				if err := os.WriteFile(golden, r.Compose, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(r.Compose, want) {
				t.Fatalf("el compose no es el golden (-update lo reescribe):\n%s", r.Compose)
			}
			checkInjections(t, r.Compose)
		})
	}
}

// checkInjections verifica en el compose final lo que §6.2 garantiza para
// todo servicio, sin mirar el caso: el driver de logging (y sus topes), la
// red del stage, las etiquetas y update_config/rollback_config completos.
func checkInjections(t *testing.T, raw []byte) {
	t.Helper()
	var f compose.File
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	network := naming.Network(goldenProject, goldenStage)
	if len(f.Networks) != 1 || f.Networks[network] != (compose.Network{Name: network, External: true}) {
		t.Errorf("networks = %+v", f.Networks)
	}
	for name, s := range f.Services {
		l := s.Logging
		if l == nil {
			t.Errorf("%s: sin logging", name)
			continue
		}
		switch l.Driver {
		case "json-file", "local":
			if l.Options["max-size"] == "" || l.Options["max-file"] == "" {
				t.Errorf("%s: logging sin max-size o max-file: %+v", name, l)
			}
		case "none":
		default:
			t.Errorf("%s: driver %q", name, l.Driver)
		}
		if !slices.Equal(s.Networks, []string{network}) {
			t.Errorf("%s: networks = %v", name, s.Networks)
		}
		if strings.Contains(s.Image, "$") || !strings.Contains(s.Image, "@sha256:") {
			t.Errorf("%s: image = %q", name, s.Image)
		}
		d := s.Deploy
		if d == nil || d.UpdateConfig == nil || d.RollbackConfig == nil {
			t.Errorf("%s: deploy = %+v", name, d)
			continue
		}
		u, rb := d.UpdateConfig, d.RollbackConfig
		if u.Parallelism == nil || u.Order == "" || u.FailureAction == "" || u.Monitor == "" || rb.Parallelism == nil || rb.Order == "" {
			t.Errorf("%s: update_config %+v rollback_config %+v", name, u, rb)
		}
		app := d.Labels[naming.LabelApp]
		for k, v := range naming.Labels(goldenProject, goldenStage, app, goldenDeployID) {
			if d.Labels[k] != v {
				t.Errorf("%s: deploy.labels[%s] = %q", name, k, d.Labels[k])
			}
		}
		for k, v := range naming.AppLabels(goldenProject, goldenStage, app) {
			if s.Labels[k] != v {
				t.Errorf("%s: labels[%s] = %q", name, k, s.Labels[k])
			}
		}
		// muelle.deploy solo en el servicio: en los contenedores, cada deploy
		// reiniciaría todos los servicios de la app (§6.2).
		if _, ok := s.Labels[naming.LabelDeploy]; ok {
			t.Errorf("%s: muelle.deploy en las etiquetas de los contenedores", name)
		}
	}
}

func TestRenderServicesAndHosts(t *testing.T) {
	r, err := renderGolden(t, "testdata/render/compose-basic.input.json")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.Services, []string{"api", "web"}) {
		t.Errorf("services = %v", r.Services)
	}
	if !slices.Equal(r.Hosts, []string{"docker.io", "ghcr.io"}) {
		t.Errorf("hosts = %v", r.Hosts)
	}
}

// Render no modifica la Config: el caso de uso puede volver a renderizar.
func TestRenderDoesNotModifyConfig(t *testing.T) {
	app := formApp(`{"image":"ghcr.io/x/api:staging","port":3000,"replicas":1,"volumes":[{"name":"data","mount":"/data"}],"healthcheck":{"type":"none"}}`)
	cfg, err := ReadConfig(context.Background(), app, map[string]string{"A": "1"})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(cfg.File)
	in := RenderInput{App: app, DeployID: 1, Images: map[string]string{"api": "ghcr.io/x/api:staging@" + digA}, StorageNodeID: "node-1"}
	first, err := Render(cfg, in)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(cfg.File)
	if !bytes.Equal(before, after) {
		t.Fatalf("modificó la Config:\n%s\n%s", before, after)
	}
	second, err := Render(cfg, in)
	if err != nil || !bytes.Equal(first.Compose, second.Compose) {
		t.Fatalf("dos renders distintos: %v", err)
	}
}

func TestRenderErrors(t *testing.T) {
	big := strings.Repeat("x", 130<<10)
	form := `{"image":"ghcr.io/x/api:staging","port":3000,"replicas":1,"healthcheck":{"type":"none"}}`
	withVolume := `{"image":"ghcr.io/x/api:staging","port":3000,"replicas":1,"healthcheck":{"type":"none"},"volumes":[{"name":"data","mount":"/data"}]}`
	img := map[string]string{"api": "ghcr.io/x/api:staging@" + digA}
	twoSvc := "services:\n  web:\n    image: nginx\n  api:\n    image: ghcr.io/x/api:1\n"
	twoImg := map[string]string{"web": "nginx:latest@" + digA, "api": "ghcr.io/x/api:1@" + digB}
	// Muchas variables medianas: cada una cabe, el total no (1,6 MiB).
	many := map[string]string{}
	for i := range 16 {
		many["VAR_"+strings.Repeat("A", i+1)] = strings.Repeat("v", 100<<10)
	}
	cases := []struct {
		name    string
		app     App
		env     map[string]string
		images  map[string]string
		node    string
		domains []DomainTarget
		want    []string
	}{
		{"destino de dominio que no existe", composeApp(twoSvc), nil, twoImg, "",
			[]DomainTarget{{Hostname: "api.x.cl", Service: "worker", Port: 80}},
			[]string{"el dominio api.x.cl apunta al servicio worker, que no está en la configuración de la app"}},
		{"puerto de dominio inválido", composeApp(twoSvc), nil, twoImg, "",
			[]DomainTarget{{Hostname: "api.x.cl", Service: "web", Port: 0}},
			[]string{"api.x.cl", "web", "puerto 0"}},
		{"destino distinto del slug en un formulario", formApp(form), nil, img, "",
			[]DomainTarget{{Hostname: "a.x.cl", Service: "api", Port: 3000}, {Hostname: "b.x.cl", Service: "web", Port: 3000}},
			[]string{"el dominio b.x.cl apunta al servicio web"}},
		{"una variable de 130 KiB", formApp(form), map[string]string{"GRANDE": big}, img, "", nil,
			[]string{"GRANDE", "servicio api", "128 KiB"}},
		{"un total de 1,6 MiB", formApp(form), many, img, "", nil,
			[]string{"entorno del servicio api", "1,5 MiB"}},
		{"falta la imagen de un servicio", composeApp(twoSvc), nil, map[string]string{"web": "nginx:latest@" + digA}, "", nil,
			[]string{"falta la imagen resuelta del servicio api"}},
		{"imagen resuelta inválida", formApp(form), nil, map[string]string{"api": "$(echo x)"}, "", nil,
			[]string{"la imagen resuelta del servicio api no es válida"}},
		{"StorageNodeID vacío con volúmenes", formApp(withVolume), nil, img, "", nil,
			[]string{"volúmenes", "nodo"}},
		{"StorageNodeID que no es un id", formApp(withVolume), nil, img, "node 1\nx", nil,
			[]string{"nodo"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg, err := ReadConfig(context.Background(), c.app, c.env)
			if err != nil {
				t.Fatal(err)
			}
			_, err = Render(cfg, RenderInput{App: c.app, DeployID: 9, Images: c.images, StorageNodeID: c.node, Domains: c.domains})
			re, ok := errors.AsType[*RenderError](err)
			if !ok {
				t.Fatalf("esperaba *RenderError, llegó %v", err)
			}
			if !strings.HasPrefix(re.Error(), "render: ") {
				t.Errorf("%q", re.Error())
			}
			for _, w := range c.want {
				if !strings.Contains(re.Msg, w) {
					t.Errorf("%q no contiene %q", re.Msg, w)
				}
			}
			// Nunca el valor de una variable.
			if strings.Contains(re.Msg, "xxxxxxxx") || strings.Contains(re.Msg, "vvvvvvvv") {
				t.Errorf("el mensaje trae el valor: %.100q", re.Msg)
			}
		})
	}
}

// Render es la última barrera antes de Swarm: aunque la Config no haya pasado
// por compose.Load, un driver de logging fuera de la lista falla.
func TestRenderRejectsLoggingDriver(t *testing.T) {
	cfg := Config{File: &compose.File{Services: map[string]compose.Service{
		"web": {Image: "nginx", Logging: &compose.Logging{Driver: "syslog", Options: map[string]string{"syslog-address": "udp://127.0.0.1:514"}}},
	}}, Images: map[string]string{"web": "nginx"}}
	_, err := Render(cfg, RenderInput{App: composeApp(""), Images: map[string]string{"web": "nginx:latest@" + digA}})
	if re, ok := errors.AsType[*RenderError](err); !ok || !strings.Contains(re.Msg, "syslog") {
		t.Fatalf("%v", err)
	}
}

// Una opción de logging que quedó vacía (max-size: ${X} con X="") cuenta como
// faltante: sin tope, el log de la app llenaría el disco.
func TestRenderFillsEmptyLoggingOptions(t *testing.T) {
	cfg := Config{File: &compose.File{Services: map[string]compose.Service{
		"web": {Image: "nginx", Logging: &compose.Logging{Driver: "local", Options: map[string]string{"max-size": "", "compress": "false"}}},
	}}, Images: map[string]string{"web": "nginx"}}
	r, err := Render(cfg, RenderInput{App: composeApp(""), Images: map[string]string{"web": "nginx:latest@" + digA}})
	if err != nil {
		t.Fatal(err)
	}
	var f compose.File
	if err := json.Unmarshal(r.Compose, &f); err != nil {
		t.Fatal(err)
	}
	want := &compose.Logging{Driver: "local", Options: map[string]string{"max-size": "10m", "max-file": "3", "compress": "false"}}
	if got := f.Services["web"].Logging; !reflect.DeepEqual(got, want) {
		t.Fatalf("logging = %+v", got)
	}
}

// Una restricción propia sobre el nodo (node.id o node.hostname, con
// cualquier operador y sin importar mayúsculas, como las lee swarmkit) evita
// la de muelle (§6.2).
func TestHasNodeConstraint(t *testing.T) {
	for _, c := range []struct {
		constraints []string
		want        bool
	}{
		{nil, false},
		{[]string{"node.role == manager"}, false},
		{[]string{"node.labels.disk == ssd"}, false},
		{[]string{"node.labels.node.id == x"}, false},
		{[]string{"node.id == abc"}, true},
		{[]string{"node.id==abc"}, true},
		{[]string{"node.role == manager", "node.hostname != x"}, true},
		{[]string{"  node.hostname  ==  x  "}, true},
		{[]string{"node.ID == abc"}, true},
		{[]string{"Node.Hostname==x"}, true},
	} {
		var p *compose.Placement
		if c.constraints != nil {
			p = &compose.Placement{Constraints: c.constraints}
		}
		if got := hasNodeConstraint(p); got != c.want {
			t.Errorf("%q: %v", c.constraints, got)
		}
	}
}
