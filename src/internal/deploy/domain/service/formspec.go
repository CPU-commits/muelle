package service

import (
	"fmt"
	"maps"
	"strconv"
	"time"

	jsonv2 "encoding/json/v2"

	"github.com/CPU-commits/muelle/src/shared/domain/compose"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// Los tipos de healthcheck de un formulario (§6.1).
const (
	HealthHTTP    = "http"
	HealthBunHTTP = "bun-http"
	HealthCommand = "command"
	HealthNone    = "none"
)

// FormSpec es apps.spec (§6.1) como lo lee deploy: catalog ya lo validó y
// normalizó. Se decodifica con json/v2 sin rechazar campos (deploy no es
// dueño del formato: un campo nuevo de catalog no rompe los deploys).
type FormSpec struct {
	Image       string           `json:"image"`
	Port        int              `json:"port"`
	Replicas    *int             `json:"replicas"`
	Command     []string         `json:"command"`
	Healthcheck *FormHealthcheck `json:"healthcheck"`
	Volumes     []FormVolume     `json:"volumes"`
	Resources   *FormResources   `json:"resources"`
}

// FormHealthcheck es el healthcheck del formulario.
type FormHealthcheck struct {
	Type         string   `json:"type"`
	Path         string   `json:"path"`
	Command      []string `json:"command"`
	IntervalS    int      `json:"interval_s"`
	TimeoutS     int      `json:"timeout_s"`
	Retries      int      `json:"retries"`
	StartPeriodS int      `json:"start_period_s"`
}

// FormVolume es un volumen nombrado del stack montado en Mount.
type FormVolume struct {
	Name  string `json:"name"`
	Mount string `json:"mount"`
}

// FormResources son los límites del servicio (0: sin límite).
type FormResources struct {
	MemoryMB int     `json:"memory_mb"`
	CPUs     float64 `json:"cpus"`
}

// ParseFormSpec decodifica apps.spec. Solo revisa lo que el render necesita
// para no armar un compose roto (la imagen, el puerto, los nombres de los
// volúmenes y el tipo de healthcheck): el resto ya lo validó catalog. Los
// errores son *RenderError en "spec: …".
func ParseFormSpec(raw []byte) (FormSpec, error) {
	var s FormSpec
	if err := jsonv2.Unmarshal(raw, &s, jsonv2.RejectUnknownMembers(false)); err != nil {
		return FormSpec{}, renderErrorf("spec: el spec de la app no se puede leer")
	}
	if s.Image == "" {
		return FormSpec{}, renderErrorf("spec: falta la imagen")
	}
	if err := validate.Port("port", s.Port); err != nil {
		return FormSpec{}, renderErrorf("spec: %s", err.Error())
	}
	if s.Replicas != nil && *s.Replicas < 0 {
		return FormSpec{}, renderErrorf("spec: replicas no puede ser negativo")
	}
	for i, v := range s.Volumes {
		if err := validate.VolumeName(fmt.Sprintf("volumes[%d].name", i), v.Name); err != nil {
			return FormSpec{}, renderErrorf("spec: %s", err.Error())
		}
		if v.Mount == "" {
			return FormSpec{}, renderErrorf("spec: volumes[%d].mount es obligatorio", i)
		}
	}
	if h := s.Healthcheck; h != nil {
		switch h.Type {
		case HealthHTTP, HealthBunHTTP, HealthNone:
		case HealthCommand:
			if len(h.Command) == 0 {
				return FormSpec{}, renderErrorf("spec: healthcheck.command es obligatorio con type command")
			}
		default:
			return FormSpec{}, renderErrorf("spec: healthcheck.type %q no es válido", h.Type)
		}
	}
	return s, nil
}

// service arma el servicio del formulario (§6.1), sin las inyecciones de
// §6.2: la imagen de la configuración, todas las variables como environment,
// el command en forma exec, el healthcheck, los volúmenes, replicas y
// resources.limits. env no se comparte con el resultado.
func (s FormSpec) service(env map[string]string) compose.Service {
	svc := compose.Service{Image: s.Image, Healthcheck: s.healthcheck()}
	if len(s.Command) > 0 {
		svc.Command = append([]string(nil), s.Command...)
	}
	if len(env) > 0 {
		svc.Environment = maps.Clone(env)
	}
	for _, v := range s.Volumes {
		svc.Volumes = append(svc.Volumes, compose.Mount{Type: "volume", Source: v.Name, Target: v.Mount})
	}
	dep := &compose.Deploy{}
	if s.Replicas != nil {
		r := uint64(*s.Replicas)
		dep.Replicas = &r
	}
	if r := s.Resources; r != nil && (r.MemoryMB > 0 || r.CPUs > 0) {
		lim := &compose.Resource{}
		if r.MemoryMB > 0 {
			lim.Memory = strconv.FormatInt(int64(r.MemoryMB)<<20, 10)
		}
		if r.CPUs > 0 {
			lim.CPUs = strconv.FormatFloat(r.CPUs, 'f', -1, 64)
		}
		dep.Resources = &compose.Resources{Limits: lim}
	}
	if dep.Replicas != nil || dep.Resources != nil {
		svc.Deploy = dep
	}
	return svc
}

// healthcheck es el de §6.1, en forma exec (sin shell); nil con none o sin
// healthcheck (si la imagen trae un HEALTHCHECK, sigue valiendo).
func (s FormSpec) healthcheck() *compose.Healthcheck {
	h := s.Healthcheck
	if h == nil {
		return nil
	}
	url := "http://127.0.0.1:" + strconv.Itoa(s.Port) + h.Path
	var test []string
	switch h.Type {
	case HealthHTTP:
		test = []string{"CMD", "wget", "-q", "-O", "/dev/null", url}
	case HealthBunHTTP:
		test = []string{"CMD", "bun", "-e", "fetch('" + url + "').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"}
	case HealthCommand:
		test = append([]string{"CMD"}, h.Command...)
	default:
		return nil
	}
	out := &compose.Healthcheck{
		Test:        test,
		Interval:    seconds(h.IntervalS),
		Timeout:     seconds(h.TimeoutS),
		StartPeriod: seconds(h.StartPeriodS),
	}
	if h.Retries > 0 {
		r := uint64(h.Retries)
		out.Retries = &r
	}
	return out
}

// seconds es n segundos como duración de compose ("10s", "1m30s"); "" con 0.
func seconds(n int) string {
	if n <= 0 {
		return ""
	}
	return (time.Duration(n) * time.Second).String()
}
