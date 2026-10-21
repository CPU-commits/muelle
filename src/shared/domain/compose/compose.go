// Package compose es la parte pura del compose de una app (§6 del diseño):
// parsear con compose-go sin leer archivos, la lista de permitidos y los
// rechazos de §6.3, la interpolación estricta con las variables de la app, y
// el modelo y la serialización del compose final. Lo usan catalog (al
// guardar) y deploy (al renderizar), sin importarse.
package compose

import (
	"context"
	"errors"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/tree"
	"github.com/compose-spec/compose-go/v2/types"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

const (
	// MaxBytes es el tamaño máximo de un compose (el mismo de catalog).
	MaxBytes = 256 << 10
	// MaxServices es el máximo de servicios de una app compose (§6.3).
	MaxServices = 32
	// Field es el campo de los errores de validación (contrato §0.2).
	Field = "compose_yaml"
)

const (
	// configFile es el nombre del compose para compose-go: solo aparece en
	// sus mensajes, nunca se abre.
	configFile = "compose.yaml"
	// workingDir no existe: nada debería resolverse contra él.
	workingDir = "/nonexistent/muelle-compose"
	// validateProject es el nombre de proyecto al guardar (compose-go exige uno).
	validateProject = "compose"
)

// Validate valida un compose como lo guarda catalog: sin interpolar (todavía
// no hay variables), con la lista de permitidos, los rechazos de §6.3 y los
// nombres. Devuelve los nombres de sus servicios, ordenados. Los errores son
// *validate.Error en Field.
func Validate(src string) ([]string, error) {
	if err := checkSize(src); err != nil {
		return nil, err
	}
	d := details(src, validateProject, nil)
	var found []loader.UnsupportedAttribute
	dict, err := loadModel(context.Background(), d, options(&d, validateProject, false, &found))
	if err != nil {
		return nil, schemaMessage(err)
	}
	if len(found) > 0 {
		return nil, findingError(found[0])
	}
	return check(dict, false)
}

// Load parsea src, lo interpola solo con env (nunca con el entorno del
// proceso) y lo valida entero (§6.3), ya con los valores. project es el
// nombre del stack (naming.Stack), que compose-go exige. Devuelve el modelo
// de salida sin las inyecciones de §6.2, que hace deploy. env no se modifica.
//
// Ojo: los errores de conversión de compose-go (replicas: ${N} con N=abc)
// traen el valor interpolado; quien los escriba en un log los enmascara.
func Load(ctx context.Context, src, project string, env map[string]string) (*File, error) {
	if err := checkSize(src); err != nil {
		return nil, err
	}
	d := details(src, project, env)
	var found []loader.UnsupportedAttribute
	opts := options(&d, project, true, &found)
	dict, err := loadModel(ctx, d, opts)
	if err != nil {
		return nil, loadError(err)
	}
	if len(found) > 0 {
		return nil, findingError(found[0])
	}
	if _, err := check(dict, true); err != nil {
		return nil, err
	}
	// Con SkipResolveEnvironment y SkipResolveLabels no lee archivos; la
	// verificación de consistencia queda prendida (cada volumen usado está
	// declarado, depends_on nombra servicios del compose…).
	var proj *types.Project
	err = safely(func() error {
		var err error
		proj, err = loader.ModelToProject(dict, loader.ToOptions(&d, opts), d)
		return err
	})
	if err != nil {
		return nil, invalid(err)
	}
	return convert(proj, d.Environment)
}

// ServiceInfo es un servicio como lo muestra catalog (contrato §2.12).
type ServiceInfo struct {
	Name  string
	Ports []int // de expose, ordenados; nunca nil
}

// Services lee los servicios de un compose ya guardado, sin validarlo ni
// interpolarlo. nil si no se puede leer.
//
// Lo lee con el esquema de compose-go (sin él, transform.Canonical hace
// panic con formas inválidas, y las filas de la Fase 1 solo pasaron por el
// chequeo de vacío y de tamaño), pero sin la lista de permitidos.
func Services(src string) []ServiceInfo {
	if strings.TrimSpace(src) == "" || len(src) > MaxBytes {
		return nil
	}
	d := details(src, validateProject, nil)
	dict, err := loadModel(context.Background(), d, options(&d, validateProject, false, nil))
	if err != nil {
		return nil
	}
	services, _ := dict["services"].(map[string]any)
	if len(services) == 0 {
		return nil
	}
	out := make([]ServiceInfo, 0, len(services))
	for _, name := range slices.Sorted(maps.Keys(services)) {
		svc, _ := services[name].(map[string]any)
		out = append(out, ServiceInfo{Name: name, Ports: exposed(svc["expose"])})
	}
	return out
}

// exposed son los puertos enteros de expose (80, "8080", "9000/tcp"),
// ordenados y sin repetir; los rangos, los de udp y los ${…} se omiten.
func exposed(v any) []int {
	ports := []int{}
	list, _ := v.([]any)
	for _, item := range list {
		var n int
		switch x := item.(type) {
		case int:
			n = x
		case string:
			x = strings.TrimSuffix(x, "/tcp")
			var err error
			if n, err = strconv.Atoi(x); err != nil {
				continue
			}
		default:
			continue
		}
		if n >= 1 && n <= 65535 && !slices.Contains(ports, n) {
			ports = append(ports, n)
		}
	}
	slices.Sort(ports)
	return ports
}

func checkSize(src string) error {
	if strings.TrimSpace(src) == "" {
		return validate.Errorf(Field, "es obligatorio")
	}
	if len(src) > MaxBytes {
		return validate.Errorf(Field, "no puede superar 256 KiB")
	}
	return nil
}

// details arma la entrada de compose-go: el compose en memoria, nunca un
// archivo; un WorkingDir que no existe (nada debería resolverse contra él), y
// una copia de env (nunca nil) con COMPOSE_PROJECT_NAME = project. compose-go
// también lo agrega (el defer de projectName), pero a la copia: env no se toca.
func details(src, project string, env map[string]string) types.ConfigDetails {
	e := maps.Clone(env)
	if e == nil {
		e = map[string]string{}
	}
	e["COMPOSE_PROJECT_NAME"] = project
	return types.ConfigDetails{
		WorkingDir:  workingDir,
		ConfigFiles: []types.ConfigFile{{Filename: configFile, Content: []byte(src)}},
		Environment: e,
	}
}

// options son las del loader: ningún paso que lea archivos (§6.3, "compose-go
// no lee archivos"), el nombre del proyecto explícito y, con found, la lista
// de permitidos, que deja sus hallazgos en *found. Con interpolate, las
// variables se buscan solo en d.Environment (la copia, que ya trae
// COMPOSE_PROJECT_NAME), nunca en el entorno del proceso ni en el env del que
// llama, y con la sustitución estricta.
//
// La validación del esquema queda prendida: es la que revisa los tipos al
// guardar, y sin ella transform.Canonical hace panic con formas inválidas.
func options(d *types.ConfigDetails, project string, interpolate bool, found *[]loader.UnsupportedAttribute) []func(*loader.Options) {
	opts := []func(*loader.Options){
		func(o *loader.Options) {
			o.SetProjectName(project, true)
			o.SkipInclude, o.SkipExtends = true, true
			o.SkipResolveEnvironment, o.SkipResolveLabels = true, true
			o.SkipNormalization, o.SkipDefaultValues = true, true
			o.ResolvePaths = false
			o.SkipInterpolation = !interpolate
			if interpolate {
				// o.Interpolate ya lo armó loader.ToOptions (con su
				// TypeCastMapping): solo se cambia de dónde salen los valores
				// y cómo se sustituyen.
				o.Interpolate.LookupValue = d.LookupEnv
				o.Interpolate.Substitute = strictSubstitute
			}
		},
	}
	if found != nil {
		opts = append(opts, loader.WithSupportedAttributes(allowed, func(f []loader.UnsupportedAttribute) { *found = f }))
	}
	return opts
}

// loadModel corre compose-go sobre una entrada que no es de confianza: un
// panic es un compose inválido, no una caída de muelle.
func loadModel(ctx context.Context, d types.ConfigDetails, opts []func(*loader.Options)) (map[string]any, error) {
	var dict map[string]any
	err := safely(func() error {
		var err error
		dict, err = loader.LoadModelWithContext(ctx, d, opts...)
		return err
	})
	return dict, err
}

// errPanic no lleva el valor del panic: podría traer valores de variables.
var errPanic = errors.New("compose-go no pudo leerlo")

func safely(f func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errPanic
		}
	}()
	return f()
}

// loadError traduce un error de compose-go al cargar con las variables: uno
// de la sustitución estricta lleva la ruta donde ocurrió; los demás siguen la
// regla de schemaMessage.
func loadError(err error) error {
	if path, inner, ok := interpolationError(err); ok {
		return validate.Errorf(Field, "%s: %s", strings.ReplaceAll(path, "."+tree.PathMatchList, ""), inner.Error())
	}
	return schemaMessage(err)
}

// quotedRe es cada clave entre comillas simples del mensaje de additional
// properties del esquema ('a', 'b').
var quotedRe = regexp.MustCompile(`'((?:[^'\\]|\\.)*)'`)

// schemaMessage traduce un error de compose-go. Una clave desconocida para
// el esquema sale con el mismo formato que un hallazgo de la lista de
// permitidos: "compose_yaml: <ruta>: <motivo>". compose-go no exporta el error
// del esquema, así que se reconocen sus dos textos (verificados en v2.16.1;
// TestSchemaMessage los fija):
//   - "validating compose.yaml: <ruta> false schema" (una clave de un
//     servicio: la ruta ya la incluye);
//   - "validating compose.yaml: <ruta del padre> additional properties 'k1',
//     'k2' not allowed" (la ruta del padre, vacía en el primer nivel). Las
//     claves vienen en el orden de un mapa: se usa la primera en orden
//     alfabético, para que el mensaje sea estable.
//
// Cualquier otro error (un tipo, validation.Validate, YAML roto) va tal cual,
// en una línea: "compose_yaml: el compose no es válido: <texto>".
func schemaMessage(err error) error {
	if rest, ok := strings.CutPrefix(err.Error(), "validating "+configFile+": "); ok {
		if p, ok := strings.CutSuffix(rest, " false schema"); ok && p != "" {
			return fail(p, "%s", reasonFor(tree.Path(p)))
		}
		const add, notAllowed = "additional properties ", " not allowed"
		if i := strings.Index(rest, add); i >= 0 && strings.HasSuffix(rest, notAllowed) {
			var keys []string
			for _, m := range quotedRe.FindAllStringSubmatch(rest[i+len(add):len(rest)-len(notAllowed)], -1) {
				keys = append(keys, m[1])
			}
			if len(keys) > 0 {
				slices.Sort(keys)
				p := keys[0]
				if parent := strings.TrimSpace(rest[:i]); parent != "" {
					p = parent + "." + p
				}
				return fail(p, "%s", reasonFor(tree.Path(p)))
			}
		}
	}
	return invalid(err)
}

var oneLine = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ")

// invalid es el error genérico: el texto de compose-go en una línea.
func invalid(err error) error {
	return validate.Errorf(Field, "el compose no es válido: %s", oneLine.Replace(err.Error()))
}
