package compose

import (
	"maps"
	"slices"
	"strings"

	"github.com/compose-spec/compose-go/v2/tree"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// fail arma un error de validación con la ruta: "compose_yaml: <ruta>: <motivo>".
func fail(path, format string, args ...any) error {
	return validate.Errorf(Field, "%s: "+format, append([]any{path}, args...)...)
}

// logDrivers son los drivers de logging permitidos (§6.3): los demás los
// corre el daemon en la red del host.
var logDrivers = map[string]bool{"json-file": true, "local": true, "none": true}

// reservedLabel dice si una etiqueta la pone muelle (§6.2) o docker stack
// deploy: deploy, ingress y el estado de las apps filtran por ellas.
func reservedLabel(key string) bool {
	return strings.HasPrefix(key, "muelle.") || strings.HasPrefix(key, "com.docker.stack.")
}

// check aplica las reglas por valor de §6.3 sobre el modelo canónico (lo que
// la lista de permitidos no ve: tipos de montaje, drivers, etiquetas,
// nombres) y devuelve los nombres de los servicios, ordenados.
//
// Con interpolated=false (Validate, sin variables todavía), una regla sobre
// un valor con $ se salta (el render la repite con el valor), pero cada string
// con $ pasa por checkTemplate: una variable mal escrita falla al guardar.
func check(dict map[string]any, interpolated bool) ([]string, error) {
	if !interpolated {
		if err := checkTemplates(dict, tree.NewPath()); err != nil {
			return nil, err
		}
	}
	services, _ := dict["services"].(map[string]any)
	if len(services) == 0 {
		return nil, validate.Errorf(Field, "el compose debe tener al menos un servicio")
	}
	if len(services) > MaxServices {
		return nil, validate.Errorf(Field, "un compose puede tener a lo más %d servicios (tiene %d)", MaxServices, len(services))
	}
	volumes, _ := dict["volumes"].(map[string]any)
	names := slices.Sorted(maps.Keys(services))
	for _, name := range names {
		if err := checkName("el servicio", name); err != nil {
			return nil, err
		}
	}
	for _, name := range slices.Sorted(maps.Keys(volumes)) {
		if err := checkName("el volumen", name); err != nil {
			return nil, err
		}
	}
	for _, name := range names {
		svc, _ := services[name].(map[string]any)
		if err := checkService(svc, tree.NewPath("services", name), volumes, services, interpolated); err != nil {
			return nil, err
		}
	}
	for _, name := range slices.Sorted(maps.Keys(volumes)) {
		v, _ := volumes[name].(map[string]any)
		if err := checkLabels(v["labels"], tree.NewPath("volumes", name, "labels"), interpolated); err != nil {
			return nil, err
		}
	}
	return names, nil
}

// checkName valida el nombre de un servicio o un volumen (§5.2 regla 1).
func checkName(what, name string) error {
	if err := validate.VolumeName(Field, name); err != nil {
		ve, _ := err.(*validate.Error)
		return validate.Errorf(Field, "%s %s", what, ve.Message)
	}
	return nil
}

// skip dice si una regla sobre s se salta: al guardar, un valor con $ todavía
// no tiene su valor.
func skip(s string, interpolated bool) bool { return !interpolated && strings.Contains(s, "$") }

func checkService(svc map[string]any, p tree.Path, volumes, services map[string]any, interpolated bool) error {
	image, ok := svc["image"].(string)
	if !ok || image == "" {
		return fail(displayPath(p.Next("image")), "es obligatoria")
	}
	if !skip(image, interpolated) {
		if err := validate.ImageRef(displayPath(p.Next("image")), image); err != nil {
			return validate.Errorf(Field, "%s", err.Error())
		}
	}

	if err := checkMounts(svc["volumes"], p.Next("volumes"), volumes, interpolated); err != nil {
		return err
	}

	if lg, ok := svc["logging"].(map[string]any); ok {
		if d, ok := lg["driver"].(string); ok && d != "" && !skip(d, interpolated) && !logDrivers[d] {
			return fail(displayPath(p.Next("logging").Next("driver")), "%s", reasonLogging)
		}
	}

	if err := checkLabels(svc["labels"], p.Next("labels"), interpolated); err != nil {
		return err
	}

	if dep, ok := svc["deploy"].(map[string]any); ok {
		dp := p.Next("deploy")
		if err := checkLabels(dep["labels"], dp.Next("labels"), interpolated); err != nil {
			return err
		}
		if mode, ok := dep["mode"].(string); ok && !skip(mode, interpolated) {
			switch mode {
			case "", "replicated", "global":
			case "replicated-job", "global-job":
				return fail(displayPath(dp.Next("mode")), "%s", reasonJobs)
			default:
				return fail(displayPath(dp.Next("mode")), "debe ser replicated o global")
			}
		}
		if n, ok := dep["replicas"].(int); ok && n < 0 {
			return fail(displayPath(dp.Next("replicas")), "no puede ser negativo")
		}
	}

	// depends_on se descarta, pero compose-go exige que nombre servicios del
	// compose al renderizar: se revisa también al guardar.
	if deps, ok := svc["depends_on"].(map[string]any); ok {
		for _, dep := range slices.Sorted(maps.Keys(deps)) {
			if _, ok := services[dep]; !ok {
				return fail(displayPath(p.Next("depends_on")), "el servicio %q no existe en el compose", dep)
			}
		}
	}

	// Lo mismo con la forma de healthcheck.test (Swarm también lo exige).
	if hc, ok := svc["healthcheck"].(map[string]any); ok {
		if test, ok := hc["test"].([]any); ok && len(test) > 0 {
			if first, ok := test[0].(string); ok && !skip(first, interpolated) {
				switch first {
				case "CMD", "CMD-SHELL", "NONE":
				default:
					return fail(displayPath(p.Next("healthcheck").Next("test")), "debe empezar con CMD, CMD-SHELL o NONE")
				}
			}
		}
	}
	return nil
}

// checkMounts revisa cada montaje (§6.3): un volumen con source declarado
// en los volumes de primer nivel, o un tmpfs.
func checkMounts(v any, p tree.Path, volumes map[string]any, interpolated bool) error {
	list, _ := v.([]any)
	path := displayPath(p)
	for _, item := range list {
		switch m := item.(type) {
		case string:
			// Una forma corta que compose-go no pudo leer: al guardar, porque
			// tiene variables; al renderizar no llega acá (es un error de
			// compose-go).
			if skip(m, interpolated) {
				continue
			}
			return fail(path, "el montaje %q no es válido", m)
		case map[string]any:
			typ, _ := m["type"].(string)
			source, _ := m["source"].(string)
			if skip(typ, interpolated) || skip(source, interpolated) {
				continue
			}
			switch typ {
			case "volume":
				if source == "" {
					return fail(path, "un volumen sin source (anónimo) no está soportado: declara el volumen en volumes de primer nivel y móntalo por su nombre")
				}
				if _, ok := volumes[source]; !ok {
					return fail(path, "el volumen %q no está en los volumes de primer nivel", source)
				}
			case "tmpfs":
			default:
				return fail(path, "%s", reasonHost)
			}
		}
	}
	return nil
}

// checkLabels rechaza las etiquetas reservadas. Las etiquetas no pasan por
// transform.Canonical: vienen como mapa o como lista. En una lista, la clave
// es lo que va antes del primer "=", o el string entero si no tiene "=" (así
// la lee Labels.DecodeMapstructure: la clave con valor "").
func checkLabels(v any, p tree.Path, interpolated bool) error {
	var keys []string
	switch l := v.(type) {
	case map[string]any:
		keys = slices.Sorted(maps.Keys(l))
	case []any:
		for _, item := range l {
			s, ok := item.(string)
			if !ok {
				continue
			}
			key, _, _ := strings.Cut(s, "=")
			if skip(key, interpolated) {
				continue // en la lista, la clave también se interpola
			}
			keys = append(keys, key)
		}
	}
	for _, k := range keys {
		if reservedLabel(k) {
			return fail(displayPath(p), "la etiqueta %q %s", k, reasonReserved)
		}
	}
	return nil
}

// checkTemplates pasa checkTemplate por cada string con $ del modelo, en
// orden (las claves de los mapas ordenadas), y nombra la ruta del primero que
// falla. Las claves no se interpolan.
func checkTemplates(v any, p tree.Path) error {
	switch x := v.(type) {
	case string:
		if err := checkTemplate(x); err != nil {
			return fail(displayPath(p), "%s", err.Error())
		}
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(x)) {
			if err := checkTemplates(x[k], p.Next(k)); err != nil {
				return err
			}
		}
	case []any:
		for _, e := range x {
			if err := checkTemplates(e, p.Next(tree.PathMatchList)); err != nil {
				return err
			}
		}
	}
	return nil
}
