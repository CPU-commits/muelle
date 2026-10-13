// Package naming arma los nombres de Docker que comparten los módulos (deploy,
// ingress) e install: stacks, servicios Swarm, redes, volúmenes, etiquetas y
// las piezas fijas de muelle (§3 y §5.2 del spec). Es el único lugar donde
// están: un nombre armado a mano en otro lado es un bug.
//
// Las funciones no validan: reciben slugs que ya pasaron por
// shared/domain/validate. Como el "_" no es válido en un slug, sirve de
// separador y un nombre nunca se confunde con otro; con los largos máximos de
// validate, un servicio Swarm tiene a lo más 63 caracteres (MaxSwarmName).
package naming

import (
	"regexp"
	"strconv"
	"strings"
)

// MaxSwarmName es el largo máximo de un nombre de servicio Swarm (una etiqueta
// DNS) y el que garantizan los slugs: 16 + 12 + 16 + 16 + 3 separadores.
const MaxSwarmName = 63

const sep = "_"

// Stack es el stack de una app: proyecto_stage_app.
func Stack(project, stage, app string) string { return join(project, stage, app) }

// Service es el servicio Swarm de un servicio de la app: proyecto_stage_app_servicio,
// el nombre que le da docker stack deploy (<stack>_<servicio>). En una app de
// formulario el único servicio se llama como la app. Caddy enruta siempre por
// este nombre, que es único (§3.2).
func Service(project, stage, app, service string) string {
	return join(project, stage, app, service)
}

// Network es la red overlay de un stage: proyecto_stage (§3.2).
func Network(project, stage string) string { return join(project, stage) }

// Volume es el nombre real de un volumen nombrado del compose: <stack>_<nombre>,
// como lo crea docker stack deploy (§6.2).
func Volume(stack, name string) string { return join(stack, name) }

// Upstream es el destino de una ruta de Caddy: <servicio Swarm>:<puerto>, por
// el DNS de Swarm (la IP virtual del servicio, §8).
func Upstream(swarmService string, port int) string {
	return swarmService + ":" + strconv.Itoa(port)
}

func join(parts ...string) string { return strings.Join(parts, sep) }

// Etiquetas que muelle agrega a cada servicio (deploy.labels) y a sus
// contenedores (labels) al renderizar el compose (§6.2). Los valores son los
// slugs y el id del deploy en decimal.
const (
	LabelProject = "muelle.project"
	LabelStage   = "muelle.stage"
	LabelApp     = "muelle.app"
	LabelDeploy  = "muelle.deploy"

	// LabelStackNamespace la pone docker stack deploy en todo lo que crea
	// (servicios, redes, volúmenes, secrets y configs del stack).
	LabelStackNamespace = "com.docker.stack.namespace"
)

// Labels son las etiquetas de los servicios de un deploy.
func Labels(project, stage, app string, deployID int64) map[string]string {
	l := AppLabels(project, stage, app)
	l[LabelDeploy] = strconv.FormatInt(deployID, 10)
	return l
}

// AppLabels sirven para filtrar los servicios y tareas de una app.
func AppLabels(project, stage, app string) map[string]string {
	l := StageLabels(project, stage)
	l[LabelApp] = app
	return l
}

// StageLabels van en la red de un stage, que crea ingress, y sirven para
// filtrar lo de un stage.
func StageLabels(project, stage string) map[string]string {
	return map[string]string{LabelProject: project, LabelStage: stage}
}

// Las piezas fijas de muelle (§3.1 y §3.3). Las crea install (Fase 5), salvo
// muelle-caddy, que crea y mantiene ingress (Fase 3).
const (
	// PanelNetwork es la red overlay attachable de muelle y Caddy, y de nadie más.
	PanelNetwork = "muelle_panel"
	// CaddyContainer es el contenedor standalone del proxy.
	CaddyContainer = "muelle-caddy"
	// MuelleStack es el stack que despliega install, con un solo servicio.
	MuelleStack = "muelle"
	// MuelleService es el servicio Swarm de muelle (stack muelle, servicio muelle).
	MuelleService = "muelle_muelle"
	// MuellePort es donde escucha muelle (MUELLE_LISTEN por defecto); nunca se publica.
	MuellePort = 8080

	// DataVolume guarda /data de muelle (la base y los backups).
	DataVolume = "muelle_data"
	// CaddyDataVolume guarda los certificados de Caddy (/data).
	CaddyDataVolume = "muelle_caddy_data"
	// CaddyConfigVolume guarda el autosave de Caddy (/config).
	CaddyConfigVolume = "muelle_caddy_config"
	// CaddyAdminVolume comparte el socket de la admin API entre Caddy y muelle.
	CaddyAdminVolume = "muelle_caddy_admin"
	// CaddyAdminDir es donde se monta CaddyAdminVolume en los dos contenedores.
	CaddyAdminDir = "/run/caddy-admin"
	// CaddyAdminSocket es el socket unix de la admin API de Caddy.
	CaddyAdminSocket = CaddyAdminDir + "/admin.sock"

	// MasterKeySecret es el secreto Swarm con la clave maestra, montado en
	// /run/secrets/muelle_master_key (MUELLE_MASTER_KEY_FILE por defecto).
	MasterKeySecret = "muelle_master_key"
	// InitCloudflareTokenSecret es el secreto Swarm opcional con el token de
	// Cloudflare de install (MUELLE_INIT_CLOUDFLARE_TOKEN_FILE).
	InitCloudflareTokenSecret = "muelle_init_cloudflare_token"

	// MuelleImage y CaddyImage son los repositorios de las imágenes (Fase 5);
	// el tag es la versión de muelle (ej. ghcr.io/cpu-commits/muelle:v0.1.0).
	MuelleImage = "ghcr.io/cpu-commits/muelle"
	CaddyImage  = "ghcr.io/cpu-commits/muelle-caddy"
)

// releaseRe es una versión publicada: v<mayor>.<menor>.<parche>, con un sufijo opcional (-rc.1).
var releaseRe = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

// MuelleImageFor es la imagen de muelle de esa versión, o "" si no es una
// versión publicada (un binario de desarrollo, "dev").
func MuelleImageFor(version string) string { return imageFor(MuelleImage, version) }

// CaddyImageFor es la imagen de muelle-caddy que va con esa versión de muelle
// (se publican juntas, con el mismo tag), o "" si no es una versión publicada.
// Es la que usa ingress cuando el setting caddy_image está vacío: así,
// actualizar muelle actualiza también Caddy.
func CaddyImageFor(version string) string { return imageFor(CaddyImage, version) }

func imageFor(repo, version string) string {
	if !releaseRe.MatchString(version) {
		return ""
	}
	return repo + ":" + version
}
