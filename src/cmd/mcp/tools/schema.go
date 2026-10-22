package tools

import (
	"fmt"
	"slices"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// slugRules son las reglas de §5.2 del spec para los campos que nombran un
// recurso de muelle (contrato §8.3, "Entrada").
var slugRules = map[string]struct {
	pattern string
	max     int
}{
	"project": {validate.Slug16Pattern, 16},
	"app":     {validate.Slug16Pattern, 16},
	"service": {validate.Slug16Pattern, 16},
	"stage":   {validate.Slug12Pattern, 12},
}

// inputSchema infiere el esquema de In con jsonschema-go (draft 2020-12: un
// struct da "additionalProperties": false, y los campos sin omitempty u
// omitzero van en required) y le suma:
//   - pattern y maxLength en project, stage, app y service;
//   - minimum y maximum en los enteros, según t.Bounds: todo entero tiene que
//     estar ahí, así ninguno queda sin rango;
//   - el default de t.Defaults, que el SDK aplica antes del handler y valida
//     al registrar. Un campo con default no puede ser obligatorio.
//
// Lo que el esquema no expresa (since, stream, el largo de contains, after,
// image, images, vars, keys) lo valida el handler con validate.Errorf.
func inputSchema[In any](t Tool) (*jsonschema.Schema, error) {
	s, err := jsonschema.For[In](nil)
	if err != nil {
		return nil, err
	}
	if s.Type != "object" {
		return nil, fmt.Errorf("la entrada tiene que ser un struct, y es %q", s.Type)
	}
	for name, prop := range s.Properties {
		if rule, ok := slugRules[name]; ok {
			if prop.Type != "string" {
				return nil, fmt.Errorf("%s tiene que ser un string", name)
			}
			prop.Pattern = rule.pattern
			prop.MaxLength = jsonschema.Ptr(rule.max)
		}
		if prop.Type == "integer" {
			if _, ok := t.Bounds[name]; !ok {
				return nil, fmt.Errorf("el entero %s no tiene rango en Tool.Bounds", name)
			}
		}
	}
	for name, r := range t.Bounds {
		prop, ok := s.Properties[name]
		if !ok || prop.Type != "integer" {
			return nil, fmt.Errorf("Tool.Bounds nombra %s, que no es un entero de la entrada", name)
		}
		prop.Minimum = jsonschema.Ptr(float64(r.Min))
		if r.Max > 0 {
			if r.Max < r.Min {
				return nil, fmt.Errorf("el rango de %s es %d a %d", name, r.Min, r.Max)
			}
			prop.Maximum = jsonschema.Ptr(float64(r.Max))
		}
	}
	for name, v := range t.Defaults {
		prop, ok := s.Properties[name]
		if !ok {
			return nil, fmt.Errorf("Tool.Defaults nombra %s, que no está en la entrada", name)
		}
		if slices.Contains(s.Required, name) {
			return nil, fmt.Errorf("%s tiene default pero es obligatorio (le falta omitzero)", name)
		}
		raw, err := rawDefault(v)
		if err != nil {
			return nil, fmt.Errorf("el default de %s: %w", name, err)
		}
		prop.Default = raw
	}
	return s, nil
}
