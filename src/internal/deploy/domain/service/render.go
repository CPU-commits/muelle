package service

import (
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/CPU-commits/muelle/src/shared/domain/compose"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// El entorno de un servicio tiene que caber en lo que Linux le da a un
// proceso al arrancar (execve, §6.3): si no, el contenedor no arranca (E2BIG).
const (
	// MaxEnvVarBytes es el máximo de una variable, como CLAVE=valor.
	MaxEnvVarBytes = 128 << 10
	// MaxEnvBytes es el máximo del entorno entero: por variable, CLAVE=valor,
	// el \0 y 8 bytes del puntero.
	MaxEnvBytes = 1536 << 10
	// envVarOverhead es lo que suma cada variable además de CLAVE=valor.
	envVarOverhead = 1 + 8
)

// Lo que §6.2 inyecta en update_config y rollback_config.
const (
	orderStartFirst = "start-first"
	orderStopFirst  = "stop-first"
	defaultMonitor  = "30s"
	failureRollback = "rollback"
)

// Las opciones de logging que §6.2 agrega con json-file o local.
var defaultLogOptions = map[string]string{"max-size": "10m", "max-file": "3"}

// nodeIDRe es un id de nodo de Swarm (en la práctica, 25 caracteres [a-z0-9]):
// va dentro de una restricción de placement.
var nodeIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// RenderInput es lo que el paso 3 le pasa al render, además de la Config.
type RenderInput struct {
	App           App
	DeployID      int64
	Images        map[string]string // servicio → ref resuelta (todas las de la Config)
	StorageNodeID string            // obligatorio si algún servicio monta un volumen nombrado
	Domains       []DomainTarget
}

// Rendered es el compose final y lo que el paso 5 necesita de él.
type Rendered struct {
	Compose  []byte   // compose.Marshal del resultado; nunca se guarda (§12)
	Services []string // nombres cortos, ordenados
	Hosts    []string // hosts de las imágenes (ImageHost), ordenados, para las credenciales del paso 5
}

// Render aplica §6.2 a la Config (sin modificarla) y verifica los destinos de
// dominio y el entorno (§6.3). Para cada servicio: la imagen resuelta, la red
// del stage (que reemplaza a cualquier otra), logging, update_config y
// rollback_config campo por campo, con volúmenes stop-first y la restricción
// al nodo de los volúmenes, y las etiquetas de muelle. Los errores son
// *RenderError.
func Render(cfg Config, in RenderInput) (Rendered, error) {
	if cfg.File == nil || len(cfg.File.Services) == 0 {
		return Rendered{}, renderErrorf("la configuración no tiene servicios")
	}
	f, err := cloneFile(cfg.File)
	if err != nil {
		return Rendered{}, renderErrorf("no se pudo copiar la configuración")
	}
	p, s, a := in.App.Project, in.App.Stage, in.App.Slug
	network := naming.Network(p, s)
	names := slices.Sorted(maps.Keys(f.Services))
	hosts := map[string]bool{}
	for _, name := range names {
		svc := f.Services[name]
		image := in.Images[name]
		if image == "" {
			return Rendered{}, renderErrorf("falta la imagen resuelta del servicio %s", name)
		}
		host, err := ImageHost(image)
		if err != nil || validate.ImageRef("image", image) != nil {
			return Rendered{}, renderErrorf("la imagen resuelta del servicio %s no es válida", name)
		}
		hosts[host] = true
		svc.Image = image
		svc.Networks = []string{network}

		if svc.Logging, err = injectLogging(svc.Logging, name); err != nil {
			return Rendered{}, err
		}

		volumes := hasNamedVolume(svc)
		if volumes {
			if in.StorageNodeID == "" {
				return Rendered{}, renderErrorf("el servicio %s monta volúmenes y falta el nodo donde viven (storage_node_id)", name)
			}
			if !nodeIDRe.MatchString(in.StorageNodeID) {
				return Rendered{}, renderErrorf("el nodo de los volúmenes (storage_node_id) no es un id de nodo válido")
			}
		}
		svc.Deploy = injectDeploy(svc.Deploy, volumes, in.StorageNodeID, naming.Labels(p, s, a, in.DeployID))
		svc.Labels = withLabels(svc.Labels, naming.AppLabels(p, s, a))

		if err := checkEnv(name, svc.Environment); err != nil {
			return Rendered{}, err
		}
		f.Services[name] = svc
	}
	f.Networks = map[string]compose.Network{network: {Name: network, External: true}}

	for _, d := range in.Domains {
		if _, ok := f.Services[d.Service]; !ok {
			return Rendered{}, renderErrorf("el dominio %s apunta al servicio %s, que no está en la configuración de la app", d.Hostname, d.Service)
		}
		if validate.Port("port", d.Port) != nil {
			return Rendered{}, renderErrorf("el dominio %s apunta al puerto %d del servicio %s, que no es válido (de 1 a 65535)", d.Hostname, d.Port, d.Service)
		}
	}

	out, err := compose.Marshal(f)
	if err != nil {
		return Rendered{}, renderErrorf("no se pudo serializar el compose final")
	}
	return Rendered{Compose: out, Services: names, Hosts: slices.Sorted(maps.Keys(hosts))}, nil
}

// cloneFile copia el modelo entero (todos sus campos van en JSON), para que
// Render no modifique la Config.
func cloneFile(f *compose.File) (*compose.File, error) {
	raw, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}
	var out compose.File
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// injectLogging completa el logging campo por campo (§6.2): sin logging,
// json-file con max-size 10m y max-file 3; sin driver (o vacío), json-file;
// con json-file o local, max-size y max-file si faltan (o quedaron vacíos), y
// las demás opciones del usuario quedan; con none, nada. El compose final
// siempre lleva el driver: si no, Docker usaría el del daemon (journald,
// syslog o gelf en algunos hosts) con las opciones libres del usuario. Otro
// driver es un error (compose.Load ya lo rechazó; esto es la última barrera).
func injectLogging(l *compose.Logging, svc string) (*compose.Logging, error) {
	out := &compose.Logging{Driver: "json-file"}
	if l != nil {
		out.Options = maps.Clone(l.Options)
		if l.Driver != "" {
			out.Driver = l.Driver
		}
	}
	switch out.Driver {
	case "json-file", "local":
		if out.Options == nil {
			out.Options = map[string]string{}
		}
		for k, v := range defaultLogOptions {
			if out.Options[k] == "" {
				out.Options[k] = v
			}
		}
	case "none":
	default:
		return nil, renderErrorf("el servicio %s usa el driver de logging %q: solo se aceptan json-file, local y none", svc, out.Driver)
	}
	return out, nil
}

// injectDeploy completa deploy (§6.2): update_config y rollback_config campo
// por campo (stop-first en vez de start-first con volúmenes, salvo un order
// propio), la restricción al nodo de los volúmenes salvo una propia sobre
// node.id o node.hostname, y las etiquetas del deploy.
func injectDeploy(d *compose.Deploy, volumes bool, node string, labels map[string]string) *compose.Deploy {
	if d == nil {
		d = &compose.Deploy{}
	}
	order := orderStartFirst
	if volumes {
		order = orderStopFirst
	}
	d.UpdateConfig = fillUpdate(d.UpdateConfig, order, true)
	d.RollbackConfig = fillUpdate(d.RollbackConfig, order, false)
	if volumes && !hasNodeConstraint(d.Placement) {
		if d.Placement == nil {
			d.Placement = &compose.Placement{}
		}
		d.Placement.Constraints = append(d.Placement.Constraints, "node.id == "+node)
	}
	d.Labels = withLabels(d.Labels, labels)
	return d
}

// fillUpdate completa update_config (update) o rollback_config: parallelism
// 1 y order; en update_config, además, failure_action rollback y monitor 30s.
func fillUpdate(u *compose.UpdateConfig, order string, update bool) *compose.UpdateConfig {
	if u == nil {
		u = &compose.UpdateConfig{}
	}
	if u.Parallelism == nil {
		one := uint64(1)
		u.Parallelism = &one
	}
	if u.Order == "" {
		u.Order = order
	}
	if update {
		if u.FailureAction == "" {
			u.FailureAction = failureRollback
		}
		if u.Monitor == "" {
			u.Monitor = defaultMonitor
		}
	}
	return u
}

// hasNodeConstraint dice si placement ya restringe el nodo (node.id o
// node.hostname, con cualquier operador y sin importar mayúsculas, como las
// lee swarmkit): §6.2 respeta la restricción propia.
func hasNodeConstraint(p *compose.Placement) bool {
	if p == nil {
		return false
	}
	for _, c := range p.Constraints {
		key := c
		if i := strings.IndexAny(c, "=!"); i >= 0 {
			key = c[:i]
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "node.id", "node.hostname":
			return true
		}
	}
	return false
}

// withLabels devuelve las etiquetas del usuario con las de muelle encima (las
// de muelle ganan; una del usuario con ese prefijo ya es un error de §6.3).
func withLabels(user, muelle map[string]string) map[string]string {
	out := maps.Clone(user)
	if out == nil {
		out = make(map[string]string, len(muelle))
	}
	maps.Copy(out, muelle)
	return out
}

// checkEnv verifica que el entorno del servicio quepa al arrancar (§6.3).
// Los mensajes llevan la clave y el tamaño, nunca el valor.
func checkEnv(svc string, env map[string]string) error {
	total := 0
	for _, k := range slices.Sorted(maps.Keys(env)) {
		n := len(k) + 1 + len(env[k])
		if n > MaxEnvVarBytes {
			return renderErrorf("la variable %s del servicio %s mide %d bytes (CLAVE=valor) y el máximo es 128 KiB: el contenedor no arrancaría", k, svc, n)
		}
		total += n + envVarOverhead
	}
	if total > MaxEnvBytes {
		return renderErrorf("el entorno del servicio %s mide %d bytes y el máximo es 1,5 MiB: el contenedor no arrancaría", svc, total)
	}
	return nil
}
