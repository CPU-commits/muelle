// Package appspec define y valida el spec de una app de formulario (§6.1 del spec).
package appspec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/CPU-commits/muelle/internal/validate"
)

const (
	HealthHTTP    = "http"
	HealthBunHTTP = "bun-http"
	HealthCommand = "command"
	HealthNone    = "none"
)

type Spec struct {
	Image       string       `json:"image"`
	Port        int          `json:"port"`
	Replicas    *int         `json:"replicas"`
	Command     []string     `json:"command"`
	Healthcheck *Healthcheck `json:"healthcheck"`
	Volumes     []Volume     `json:"volumes"`
	Resources   *Resources   `json:"resources"`
}

type Healthcheck struct {
	Type         string   `json:"type"`
	Path         string   `json:"path,omitempty"`
	Command      []string `json:"command,omitempty"`
	IntervalS    int      `json:"interval_s,omitempty"`
	TimeoutS     int      `json:"timeout_s,omitempty"`
	Retries      int      `json:"retries,omitempty"`
	StartPeriodS int      `json:"start_period_s,omitempty"`
}

type Volume struct {
	Name  string `json:"name"`
	Mount string `json:"mount"`
}

type Resources struct {
	MemoryMB int     `json:"memory_mb,omitempty"`
	CPUs     float64 `json:"cpus,omitempty"`
}

// La ruta del healthcheck termina dentro de un comando de shell o de JS,
// así que solo se aceptan caracteres que no necesitan escape.
var healthPathRe = regexp.MustCompile(`^/[A-Za-z0-9._~/?&=%+-]*$`)

// Parse decodifica el JSON (rechazando campos desconocidos), aplica los valores
// por defecto y valida. Lo que devuelve está listo para guardarse con JSON().
func Parse(raw []byte) (Spec, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return Spec{}, validate.Errorf("spec", "es obligatorio")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var s Spec
	if err := dec.Decode(&s); err != nil {
		return Spec{}, validate.Errorf("spec", "JSON inválido: %v", err)
	}
	s.normalize()
	if err := s.Validate(); err != nil {
		return Spec{}, err
	}
	return s, nil
}

// JSON serializa el spec normalizado para guardarlo.
func (s Spec) JSON() json.RawMessage {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err) // Spec solo tiene tipos serializables
	}
	return b
}

func (s *Spec) normalize() {
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

func (s Spec) Validate() error {
	if s.Image == "" || len(s.Image) > 512 || strings.ContainsAny(s.Image, " \t\r\n") {
		return validate.Errorf("image", "es obligatoria, sin espacios y de hasta 512 caracteres")
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
		if r.CPUs != 0 && (r.CPUs < 0.01 || r.CPUs > 256) {
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
		return nil
	case HealthHTTP, HealthBunHTTP:
		if !healthPathRe.MatchString(h.Path) || len(h.Path) > 256 {
			return validate.Errorf("healthcheck.path", "debe empezar con / y usar solo letras, números y ._~/?&=%%+-")
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
	case h.StartPeriodS < 0 || h.StartPeriodS > 3600:
		return validate.Errorf("healthcheck.start_period_s", "debe estar entre 0 y 3600")
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
		if !strings.HasPrefix(v.Mount, "/") || path.Clean(v.Mount) != v.Mount || v.Mount == "/" {
			return validate.Errorf(field+".mount", "debe ser una ruta absoluta limpia distinta de /")
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
