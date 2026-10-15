package compose

import (
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/tree"
)

// allowed son las rutas que acepta §6.3 del diseño, sobre el modelo
// canónico de compose-go (después de transform.Canonical: los montajes cortos
// ya son mapas, dns es una lista…). "*" vale para cualquier clave de un mapa y
// para los ítems de una lista. Una ruta declarada acepta solo ese nodo, no lo
// que tiene adentro: cada mapa de claves libres necesita su ".*" (si no, cada
// clave sale como hallazgo). Las listas de strings no lo necesitan (compose-go
// no revisa sus ítems), pero se declaran igual, por si un ítem es un mapa.
// Las extensiones x-* nunca se reportan.
//
// testdata/kitchen-sink.yaml usa todas: es lo que prueba que la lista está
// completa.
var allowed = paths(
	// Primer nivel. version lo borra el esquema de compose-go; se acepta igual.
	"version",
	"services", "services.*",
	"volumes", "volumes.*", "volumes.*.labels", "volumes.*.labels.*",

	// Servicio.
	"services.*.image",
	"services.*.command", "services.*.command.*",
	"services.*.entrypoint", "services.*.entrypoint.*",
	"services.*.environment", "services.*.environment.*",
	"services.*.labels", "services.*.labels.*",
	"services.*.tmpfs", "services.*.tmpfs.*",
	"services.*.user", "services.*.working_dir",
	"services.*.stop_grace_period", "services.*.stop_signal",
	"services.*.read_only", "services.*.init",
	"services.*.ulimits", "services.*.ulimits.*", "services.*.ulimits.*.soft", "services.*.ulimits.*.hard",
	"services.*.cap_drop", "services.*.cap_drop.*",
	"services.*.hostname", "services.*.domainname",
	"services.*.extra_hosts", "services.*.extra_hosts.*",
	"services.*.dns", "services.*.dns.*",
	"services.*.dns_search", "services.*.dns_search.*",
	"services.*.expose", "services.*.expose.*",
	"services.*.tty", "services.*.stdin_open",
	// Se aceptan y se descartan: Swarm los ignora.
	"services.*.restart", "services.*.platform",
	"services.*.depends_on", "services.*.depends_on.*", "services.*.depends_on.*.*",

	// healthcheck.
	"services.*.healthcheck",
	"services.*.healthcheck.test", "services.*.healthcheck.test.*",
	"services.*.healthcheck.interval", "services.*.healthcheck.timeout",
	"services.*.healthcheck.retries", "services.*.healthcheck.start_period",
	"services.*.healthcheck.start_interval", "services.*.healthcheck.disable",

	// logging: las opciones son libres porque el driver ya está acotado (check.go).
	"services.*.logging", "services.*.logging.driver",
	"services.*.logging.options", "services.*.logging.options.*",

	// Montajes: un volumen del compose o un tmpfs (el tipo lo revisa check.go).
	"services.*.volumes", "services.*.volumes.*",
	"services.*.volumes.*.type", "services.*.volumes.*.source", "services.*.volumes.*.target",
	"services.*.volumes.*.read_only",
	"services.*.volumes.*.volume", "services.*.volumes.*.volume.nocopy",
	"services.*.volumes.*.tmpfs", "services.*.volumes.*.tmpfs.size", "services.*.volumes.*.tmpfs.mode",

	// deploy.
	"services.*.deploy",
	"services.*.deploy.mode", "services.*.deploy.replicas", "services.*.deploy.endpoint_mode",
	"services.*.deploy.labels", "services.*.deploy.labels.*",
	"services.*.deploy.placement",
	"services.*.deploy.placement.constraints", "services.*.deploy.placement.constraints.*",
	"services.*.deploy.placement.preferences", "services.*.deploy.placement.preferences.*",
	"services.*.deploy.placement.preferences.*.spread",
	"services.*.deploy.placement.max_replicas_per_node",
	"services.*.deploy.resources",
	"services.*.deploy.resources.limits",
	"services.*.deploy.resources.limits.cpus", "services.*.deploy.resources.limits.memory",
	"services.*.deploy.resources.reservations",
	"services.*.deploy.resources.reservations.cpus", "services.*.deploy.resources.reservations.memory",
	"services.*.deploy.restart_policy",
	"services.*.deploy.restart_policy.condition", "services.*.deploy.restart_policy.delay",
	"services.*.deploy.restart_policy.max_attempts", "services.*.deploy.restart_policy.window",
	"services.*.deploy.update_config",
	"services.*.deploy.update_config.parallelism", "services.*.deploy.update_config.delay",
	"services.*.deploy.update_config.failure_action", "services.*.deploy.update_config.monitor",
	"services.*.deploy.update_config.max_failure_ratio", "services.*.deploy.update_config.order",
	"services.*.deploy.rollback_config",
	"services.*.deploy.rollback_config.parallelism", "services.*.deploy.rollback_config.delay",
	"services.*.deploy.rollback_config.failure_action", "services.*.deploy.rollback_config.monitor",
	"services.*.deploy.rollback_config.max_failure_ratio", "services.*.deploy.rollback_config.order",
)

func paths(p ...string) []tree.Path {
	out := make([]tree.Path, len(p))
	for i, s := range p {
		out[i] = tree.Path(s)
	}
	return out
}

// Los motivos de cada rechazo de §6.3, para que el mensaje lo explique.
const (
	reasonGeneric  = "no está soportado: muelle solo acepta las claves de compose que conoce (§6.3)"
	reasonPorts    = "publicar puertos no está soportado en la v1: los dominios van por Caddy"
	reasonBuild    = "construir imágenes no está soportado: las imágenes vienen de un registry"
	reasonHost     = "solo se monta un volumen del compose (type: volume, con source) o un tmpfs: montar el host no está soportado"
	reasonSubpath  = "volume.subpath no está soportado: en un montaje solo valen read_only y volume.nocopy"
	reasonTopVol   = "un volumen de primer nivel solo acepta labels: muelle lo crea como <stack>_<nombre> con el driver local, y driver, driver_opts, external o name montarían el host o volúmenes ajenos"
	reasonDevices  = "dar acceso a dispositivos del host no está soportado"
	reasonVolFrom  = "montar los volúmenes de otro contenedor no está soportado"
	reasonPriv     = "escala privilegios o sale del aislamiento del contenedor: no está soportado"
	reasonFiles    = "haría leer archivos del servidor de muelle, no de la app: no está soportado"
	reasonSecrets  = "secrets y configs no están soportados en la v1: los secretos de una app van como variables de la app o del stage"
	reasonNetwork  = "muelle conecta cada servicio a la red del stage: las redes y los nombres propios no están soportados"
	reasonName     = "el nombre del proyecto lo fija muelle"
	reasonDNSOpt   = "docker stack deploy no acepta dns_opt"
	reasonSocket   = "monta el socket de Docker: no está soportado"
	reasonLogging  = "solo se aceptan los drivers json-file, local y none: los demás los corre el daemon en la red del host"
	reasonJobs     = "los jobs no están soportados en la v1: el modo es replicated o global"
	reasonReserved = "está reservada: las etiquetas muelle.* las pone muelle y las com.docker.stack.*, docker stack deploy"
)

// reasons asocia la ruta de una clave rechazada (como patrón) a su motivo.
// Se busca primero por la ruta y, si ninguna coincide, por el último segmento
// en byKey. Lo que no está en ninguno lleva reasonGeneric.
var reasons = []struct {
	path   tree.Path
	reason string
}{
	{"name", reasonName},
	{"networks", reasonNetwork},
	{"secrets", reasonSecrets},
	{"configs", reasonSecrets},
	{"include", reasonFiles},
	{"volumes.*.driver", reasonTopVol},
	{"volumes.*.driver_opts", reasonTopVol},
	{"volumes.*.external", reasonTopVol},
	{"volumes.*.name", reasonTopVol},
	{"services.*.volumes.*.bind", reasonHost},
	{"services.*.volumes.*.npipe", reasonHost},
	{"services.*.volumes.*.cluster", reasonHost},
	{"services.*.volumes.*.image", reasonHost},
	{"services.*.volumes.*.volume.subpath", reasonSubpath},
	{"services.*.deploy.resources.reservations.devices", reasonDevices},
}

// byKey es el motivo por la clave (el último segmento de la ruta) en un servicio.
var byKey = map[string]string{
	"ports":               reasonPorts,
	"build":               reasonBuild,
	"devices":             reasonDevices,
	"device_cgroup_rules": reasonDevices,
	"gpus":                reasonDevices,
	"volumes_from":        reasonVolFrom,
	"privileged":          reasonPriv,
	"cap_add":             reasonPriv,
	"security_opt":        reasonPriv,
	"sysctls":             reasonPriv,
	"pid":                 reasonPriv,
	"ipc":                 reasonPriv,
	"uts":                 reasonPriv,
	"userns_mode":         reasonPriv,
	"cgroup":              reasonPriv,
	"cgroup_parent":       reasonPriv,
	"runtime":             reasonPriv,
	"isolation":           reasonPriv,
	"credential_spec":     reasonPriv,
	"secrets":             reasonSecrets,
	"configs":             reasonSecrets,
	"env_file":            reasonFiles,
	"label_file":          reasonFiles,
	"extends":             reasonFiles,
	"include":             reasonFiles,
	"container_name":      reasonNetwork,
	"network_mode":        reasonNetwork,
	"links":               reasonNetwork,
	"external_links":      reasonNetwork,
	"networks":            reasonNetwork,
	"dns_opt":             reasonDNSOpt,
	"use_api_socket":      reasonSocket,
}

// reasonFor es el motivo de rechazar la clave en p (una ruta del modelo, con
// "[]" en los ítems de una lista).
func reasonFor(p tree.Path) string {
	for _, r := range reasons {
		if p.Matches(r.path) {
			return r.reason
		}
	}
	parts := p.Parts()
	if len(parts) >= 3 && parts[0] == "services" {
		if r, ok := byKey[parts[len(parts)-1]]; ok {
			return r
		}
	}
	return reasonGeneric
}

// displayPath es la ruta de un mensaje: sin los segmentos "[]" de las listas
// y con los "." de las claves como "." (compose-go los guarda como 👻).
func displayPath(p tree.Path) string {
	var kept []string
	for _, part := range p.Parts() {
		if part != tree.PathMatchList {
			kept = append(kept, part)
		}
	}
	return tree.NewPath(kept...).String()
}

// findingError es el error de un hallazgo de la lista de permitidos.
func findingError(f loader.UnsupportedAttribute) error {
	return fail(displayPath(f.Path), "%s", reasonFor(f.Path))
}
