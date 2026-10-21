package naming

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

func TestNames(t *testing.T) {
	if got := Stack("auditfy", "staging", "api"); got != "auditfy_staging_api" {
		t.Errorf("Stack = %q", got)
	}
	if got := Service("auditfy", "staging", "api", "web"); got != "auditfy_staging_api_web" {
		t.Errorf("Service = %q", got)
	}
	if got := Network("auditfy", "production"); got != "auditfy_production" {
		t.Errorf("Network = %q", got)
	}
	if got := Volume("auditfy_staging_api", "data"); got != "auditfy_staging_api_data" {
		t.Errorf("Volume = %q", got)
	}
	// El servicio Swarm es el stack + el nombre corto: así lo nombra docker stack deploy.
	if Service("a", "b", "c", "d") != Stack("a", "b", "c")+"_d" {
		t.Error("Service debe ser <stack>_<servicio>")
	}
}

// Los slugs más largos que acepta validate (§5.2 del spec) arman nombres que
// Docker acepta: el servicio Swarm tiene a lo más 63 caracteres, igual que el
// volumen más largo, y la red del stage cabe con holgura.
func TestMaxLengths(t *testing.T) {
	project, stage, app, service := strings.Repeat("p", 16), strings.Repeat("s", 12), strings.Repeat("a", 16), strings.Repeat("v", 16)
	for _, err := range []error{
		validate.ProjectSlug(project), validate.StageSlug(stage), validate.AppSlug(app), validate.VolumeName("service", service),
	} {
		if err != nil {
			t.Fatalf("los slugs de prueba deben ser válidos: %v", err)
		}
	}
	// Uno más y validate los rechaza: son de verdad los máximos.
	if validate.ProjectSlug(project+"p") == nil || validate.StageSlug(stage+"s") == nil ||
		validate.AppSlug(app+"a") == nil || validate.VolumeName("service", service+"v") == nil {
		t.Fatal("validate acepta un slug más largo que el máximo de §5.2")
	}

	cases := map[string]string{
		"servicio": Service(project, stage, app, service),
		"volumen":  Volume(Stack(project, stage, app), service),
		"stack":    Stack(project, stage, app),
		"red":      Network(project, stage),
	}
	for what, name := range cases {
		if len(name) > MaxSwarmName {
			t.Errorf("%s %q tiene %d caracteres, más que %d", what, name, len(name), MaxSwarmName)
		}
	}
	if n := len(cases["servicio"]); n != MaxSwarmName {
		t.Errorf("el servicio más largo debería usar justo los %d caracteres, usa %d", MaxSwarmName, n)
	}
}

func TestLabels(t *testing.T) {
	got := Labels("auditfy", "staging", "api", 42)
	want := map[string]string{
		"muelle.project": "auditfy",
		"muelle.stage":   "staging",
		"muelle.app":     "api",
		"muelle.deploy":  "42",
	}
	if !maps.Equal(got, want) {
		t.Fatalf("Labels = %v, esperaba %v", got, want)
	}
	if LabelProject != "muelle.project" || LabelStage != "muelle.stage" || LabelApp != "muelle.app" || LabelDeploy != "muelle.deploy" {
		t.Fatal("las etiquetas son el contrato de §6.2: no cambian")
	}
	// AppLabels sirve para filtrar: no lleva el deploy.
	if l := AppLabels("auditfy", "staging", "api"); len(l) != 3 || l[LabelDeploy] != "" || l[LabelApp] != "api" {
		t.Fatalf("AppLabels = %v", l)
	}
	if l := StageLabels("auditfy", "staging"); len(l) != 2 || l[LabelStage] != "staging" || l[LabelProject] != "auditfy" {
		t.Fatalf("StageLabels = %v", l)
	}
}

// Una red de stage nunca se llama como las de muelle o Docker: los slugs de
// proyecto muelle y docker están reservados (§5.2 regla 1).
func TestStageNetworkNeverCollidesWithInfra(t *testing.T) {
	for _, infra := range []string{PanelNetwork, "docker_gwbridge"} {
		project, stage, _ := strings.Cut(infra, "_")
		if validate.ProjectSlug(project) == nil && validate.StageSlug(stage) == nil {
			t.Errorf("la red %q se podría crear como red de stage (%s/%s)", infra, project, stage)
		}
	}
}

func TestInfraNames(t *testing.T) {
	// Los nombres que install (Fase 5), ingress (Fase 3) y deploy (Fase 2) comparten.
	checks := map[string]string{
		PanelNetwork:      "muelle_panel",
		CaddyContainer:    "muelle-caddy",
		MuelleStack:       "muelle",
		MuelleService:     "muelle_muelle",
		DataVolume:        "muelle_data",
		CaddyDataVolume:   "muelle_caddy_data",
		CaddyConfigVolume: "muelle_caddy_config",
		CaddyAdminVolume:  "muelle_caddy_admin",
		MasterKeySecret:   "muelle_master_key",
		CaddyAdminSocket:  "/run/caddy-admin/admin.sock",
		MuelleImage:       "ghcr.io/cpu-commits/muelle",
		CaddyImage:        "ghcr.io/cpu-commits/muelle-caddy",
	}
	for got, want := range checks {
		if got != want {
			t.Errorf("%q, esperaba %q", got, want)
		}
	}
	if MuelleService != MuelleStack+"_muelle" {
		t.Errorf("el servicio de muelle es <stack>_<servicio>: %q", MuelleService)
	}
	if MuellePort != 8080 {
		t.Errorf("MuellePort = %d", MuellePort)
	}
	if got := Upstream(MuelleService, MuellePort); got != "muelle_muelle:8080" {
		t.Errorf("Upstream del panel = %q", got)
	}
	if got := Upstream(Service("auditfy", "staging", "api", "api"), 3000); got != "auditfy_staging_api_api:3000" {
		t.Errorf("Upstream = %q", got)
	}
}

// Las imágenes de una versión publicada (v1.2.3, con sufijo opcional) llevan
// ese tag; un binario de desarrollo no tiene imagen por defecto.
func TestImagesForVersion(t *testing.T) {
	for version, want := range map[string][2]string{
		"v0.1.0":       {"ghcr.io/cpu-commits/muelle:v0.1.0", "ghcr.io/cpu-commits/muelle-caddy:v0.1.0"},
		"v1.12.3-rc.1": {"ghcr.io/cpu-commits/muelle:v1.12.3-rc.1", "ghcr.io/cpu-commits/muelle-caddy:v1.12.3-rc.1"},
		"":             {"", ""},
		"dev":          {"", ""},
		"(devel)":      {"", ""},
		"v1.2":         {"", ""},
		"v1.2.3 x":     {"", ""},
	} {
		if got := MuelleImageFor(version); got != want[0] {
			t.Errorf("MuelleImageFor(%q) = %q, esperaba %q", version, got, want[0])
		}
		if got := CaddyImageFor(version); got != want[1] {
			t.Errorf("CaddyImageFor(%q) = %q, esperaba %q", version, got, want[1])
		}
	}
}

// muelle-caddy arranca igual con cualquier imagen (la de la Fase 5 o una
// caddy_image propia): el comando y el entorno los pone ingress al crear el
// contenedor, no el CMD de la imagen, que en una imagen de Caddy típica deja
// la admin API en TCP y sin el socket.
func TestCaddyRuntime(t *testing.T) {
	if CaddyAdminListen != "unix//run/caddy-admin/admin.sock" {
		t.Errorf("CaddyAdminListen = %q", CaddyAdminListen)
	}
	if CaddyDataDir != "/data" || CaddyConfigDir != "/config" {
		t.Errorf("montajes: %q %q", CaddyDataDir, CaddyConfigDir)
	}
	if got := CaddyCommand(); !slices.Equal(got, []string{"caddy", "run", "--resume"}) {
		t.Errorf("CaddyCommand() = %q", got)
	}
	want := []string{"CADDY_ADMIN=unix//run/caddy-admin/admin.sock", "XDG_CONFIG_HOME=/config", "XDG_DATA_HOME=/data"}
	if got := CaddyEnv(); !slices.Equal(got, want) {
		t.Errorf("CaddyEnv() = %q", got)
	}
	// Cada llamada da una copia: quien la modifica no cambia la de otro.
	CaddyCommand()[0], CaddyEnv()[0] = "x", "x"
	if CaddyCommand()[0] != "caddy" || CaddyEnv()[0] != want[0] {
		t.Error("CaddyCommand y CaddyEnv deben devolver un slice nuevo")
	}
}
