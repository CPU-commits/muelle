// Package vo tiene los objetos de valor del catálogo. AppSpec define y valida el spec de una app de formulario (§6.1 del spec).
package vo

import (
	"bytes"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/distribution/reference"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

const (
	HealthHTTP    = "http"
	HealthBunHTTP = "bun-http"
	HealthCommand = "command"
	HealthNone    = "none"
)

type AppSpec struct {
	Image    string `json:"image"`
	Port     int    `json:"port"`
	Replicas *int   `json:"replicas"`
	// Command y healthcheck.command aceptan "$" y saltos de línea a propósito
	// (son argumentos, no una ruta). El renderizador de compose de la Fase 2
	// debe escapar "$" como "$$", o compose lo interpolaría.
	Command     []string     `json:"command"`
	Healthcheck *Healthcheck `json:"healthcheck"`
	Volumes     []Volume     `json:"volumes"`
	Resources   *Resources   `json:"resources"`
}

type Healthcheck struct {
	Type         string   `json:"type"`
	Path         string   `json:"path,omitempty"`
	Command      []string `json:"command,omitempty"`
	IntervalS    int      `json:"interval_s,omitzero"`
	TimeoutS     int      `json:"timeout_s,omitzero"`
	Retries      int      `json:"retries,omitzero"`
	StartPeriodS int      `json:"start_period_s,omitzero"`
}

type Volume struct {
	Name  string `json:"name"`
	Mount string `json:"mount"`
}

type Resources struct {
	MemoryMB int     `json:"memory_mb,omitzero"`
	CPUs     float64 `json:"cpus,omitzero"`
}

// La ruta del healthcheck termina dentro de un comando de shell o de JS, así
// que solo se aceptan caracteres que no necesitan escape. Quedan fuera & ; | $
// comillas, comilla invertida y espacios. En la Fase 2 el healthcheck http se
// genera en forma exec, sin shell (§6.1 del spec).
var healthPathRe = regexp.MustCompile(`^/[A-Za-z0-9._~/?=%+-]*$`)

// mountRe limita las rutas de montaje a caracteres que no necesitan escape
// (sin ":" que agregaría opciones como ":ro", ni saltos de línea ni "$").
var mountRe = regexp.MustCompile(`^/[A-Za-z0-9._/-]{1,255}$`)

// ParseAppSpec decodifica el JSON de forma estricta (json/v2: rechaza campos
// desconocidos, claves duplicadas, UTF-8 inválido, datos después del valor y
// mayúsculas distintas en las claves), aplica los valores por defecto y valida.
// Lo que devuelve está listo para guardarse con JSON().
func ParseAppSpec(raw []byte) (AppSpec, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return AppSpec{}, validate.Errorf("spec", "es obligatorio")
	}
	var s AppSpec
	if err := jsonv2.Unmarshal(raw, &s, jsonv2.RejectUnknownMembers(true)); err != nil {
		return AppSpec{}, validate.Errorf("spec", "%s", jsonErrorMessage(err))
	}
	s.normalize()
	if err := s.Validate(); err != nil {
		return AppSpec{}, err
	}
	return s, nil
}

// jsonErrorMessage describe el fallo sin exponer tipos internos de Go.
func jsonErrorMessage(err error) string {
	if se, ok := errors.AsType[*jsonv2.SemanticError](err); ok {
		field := strings.Join(slices.Collect(se.JSONPointer.Tokens()), ".")
		switch {
		case errors.Is(se.Err, jsonv2.ErrUnknownName):
			return fmt.Sprintf("JSON inválido: el campo %q no existe", field)
		case field != "":
			return fmt.Sprintf("JSON inválido: el campo %q tiene un valor incorrecto", field)
		}
		return "JSON inválido: no tiene la forma esperada"
	}
	if se, ok := errors.AsType[*jsontext.SyntacticError](err); ok {
		if errors.Is(se.Err, jsontext.ErrDuplicateName) {
			return fmt.Sprintf("JSON inválido: el campo %q está repetido", strings.Join(slices.Collect(se.JSONPointer.Tokens()), "."))
		}
		return fmt.Sprintf("JSON inválido cerca del byte %d", se.ByteOffset)
	}
	return "JSON inválido"
}

// JSON serializa el spec normalizado para guardarlo.
func (s AppSpec) JSON() []byte {
	b, err := jsonv2.Marshal(s)
	if err != nil {
		panic(err) // AppSpec solo tiene tipos serializables
	}
	return b
}

func (s *AppSpec) normalize() {
	if s.Replicas == nil {
		one := 1
		s.Replicas = &one
	}
	if s.Command == nil {
		s.Command = []string{}
	}
	if s.Volumes == nil {
		s.Volumes = []Volume{}
	}
	if s.Healthcheck == nil {
		s.Healthcheck = &Healthcheck{Type: HealthNone}
	}
	h := s.Healthcheck
	if h.Type != HealthNone {
		if h.IntervalS == 0 {
			h.IntervalS = 10
		}
		if h.TimeoutS == 0 {
			h.TimeoutS = 5
		}
		if h.Retries == 0 {
			h.Retries = 3
		}
		if h.StartPeriodS == 0 {
			h.StartPeriodS = 20
		}
	}
}

func (s AppSpec) Validate() error {
	if err := validateImage(s.Image); err != nil {
		return err
	}
	if s.Port < 1 || s.Port > 65535 {
		return validate.Errorf("port", "debe estar entre 1 y 65535")
	}
	if s.Replicas == nil || *s.Replicas < 0 || *s.Replicas > 50 {
		return validate.Errorf("replicas", "debe estar entre 0 y 50")
	}
	if len(s.Command) > 64 {
		return validate.Errorf("command", "máximo 64 argumentos")
	}
	if err := s.Healthcheck.validate(); err != nil {
		return err
	}
	if err := validateVolumes(s.Volumes); err != nil {
		return err
	}
	if r := s.Resources; r != nil {
		if r.MemoryMB != 0 && (r.MemoryMB < 16 || r.MemoryMB > 1<<20) {
			return validate.Errorf("resources.memory_mb", "debe estar entre 16 y 1048576")
		}
		if r.CPUs != 0 && !(r.CPUs >= 0.01 && r.CPUs <= 256) {
			return validate.Errorf("resources.cpus", "debe estar entre 0.01 y 256")
		}
	}
	return nil
}

func (h *Healthcheck) validate() error {
	if h == nil {
		return validate.Errorf("healthcheck", "es obligatorio")
	}
	switch h.Type {
	case HealthNone:
		if h.Path != "" || len(h.Command) > 0 {
			return validate.Errorf("healthcheck", "con type none no lleva path ni command")
		}
		if h.IntervalS != 0 || h.TimeoutS != 0 || h.Retries != 0 || h.StartPeriodS != 0 {
			return validate.Errorf("healthcheck", "con type none no lleva intervalos, timeouts ni reintentos")
		}
		return nil
	case HealthHTTP, HealthBunHTTP:
		if !healthPathRe.MatchString(h.Path) || len(h.Path) > 256 {
			return validate.Errorf("healthcheck.path", "debe empezar con / y usar solo letras, números y ._~/?=%%+-")
		}
		if len(h.Command) > 0 {
			return validate.Errorf("healthcheck.command", "solo aplica a type command")
		}
	case HealthCommand:
		if len(h.Command) == 0 || len(h.Command) > 64 {
			return validate.Errorf("healthcheck.command", "es obligatorio con type command (máximo 64 argumentos)")
		}
		if h.Path != "" {
			return validate.Errorf("healthcheck.path", "solo aplica a type http o bun-http")
		}
	default:
		return validate.Errorf("healthcheck.type", "debe ser http, bun-http, command o none")
	}
	switch {
	case h.IntervalS < 1 || h.IntervalS > 3600:
		return validate.Errorf("healthcheck.interval_s", "debe estar entre 1 y 3600")
	case h.TimeoutS < 1 || h.TimeoutS > 600:
		return validate.Errorf("healthcheck.timeout_s", "debe estar entre 1 y 600")
	case h.Retries < 1 || h.Retries > 20:
		return validate.Errorf("healthcheck.retries", "debe estar entre 1 y 20")
	case h.StartPeriodS < 1 || h.StartPeriodS > 3600:
		return validate.Errorf("healthcheck.start_period_s", "debe estar entre 1 y 3600")
	}
	return nil
}

func validateVolumes(vols []Volume) error {
	if len(vols) > 10 {
		return validate.Errorf("volumes", "máximo 10")
	}
	names := map[string]bool{}
	mounts := map[string]bool{}
	for i, v := range vols {
		field := fmt.Sprintf("volumes[%d]", i)
		if err := validate.VolumeName(field+".name", v.Name); err != nil {
			return err
		}
		if !mountRe.MatchString(v.Mount) || path.Clean(v.Mount) != v.Mount {
			return validate.Errorf(field+".mount", "debe ser una ruta absoluta limpia distinta de /, de hasta 256 caracteres, con solo letras, números y ._/-")
		}
		if names[v.Name] {
			return validate.Errorf(field+".name", "%q está repetido", v.Name)
		}
		if mounts[v.Mount] {
			return validate.Errorf(field+".mount", "%q está repetido", v.Mount)
		}
		names[v.Name] = true
		mounts[v.Mount] = true
	}
	return nil
}

// validateImage acepta solo referencias de imagen Docker bien formadas
// (nombre en minúsculas, tag y digest opcionales).
func validateImage(img string) error {
	if img == "" || len(img) > 512 {
		return validate.Errorf("image", "es obligatoria y de hasta 512 caracteres")
	}
	// Defensa extra: una imagen que empieza con "-" o lleva "$" nunca debe
	// llegar a un comando aunque la librería la aceptara.
	if strings.HasPrefix(img, "-") || strings.Contains(img, "$") {
		return validate.Errorf("image", "no es una referencia de imagen válida")
	}
	if _, err := reference.ParseNormalizedNamed(img); err != nil { // el texto de err está en inglés: no se muestra
		return validate.Errorf("image", "la imagen no es una referencia válida de Docker (registro/repositorio[:tag][@sha256:…], en minúsculas)")
	}
	return nil
}
