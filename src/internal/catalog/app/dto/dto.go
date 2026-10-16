// Package dto tiene los datos que entran y salen de los casos de uso del
// catálogo. Las etiquetas json son el contrato de la API REST. Los campos con
// json:"-" vienen de la ruta, no del cuerpo.
package dto

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"time"

	"github.com/CPU-commits/muelle/src/internal/catalog/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/compose"
)

type CreateProjectInput struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type UpdateProjectInput struct {
	Slug string `json:"-"`
	Name string `json:"name"`
}

type ProjectOutput struct {
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func ProjectFrom(p entity.Project) ProjectOutput {
	return ProjectOutput{Slug: p.Slug, Name: p.Name, CreatedAt: p.CreatedAt}
}

type CreateStageInput struct {
	Project string `json:"-"`
	Slug    string `json:"slug"`
}

type StageOutput struct {
	Project   string    `json:"project"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}

func StageFrom(s entity.Stage) StageOutput {
	return StageOutput{Project: s.ProjectSlug, Slug: s.Slug, CreatedAt: s.CreatedAt}
}

// CreateAppInput: según Kind se usa Spec (service) o ComposeYAML (compose); el otro debe faltar.
type CreateAppInput struct {
	Project        string         `json:"-"`
	Stage          string         `json:"-"`
	Slug           string         `json:"slug"`
	Kind           string         `json:"kind"`
	Spec           jsontext.Value `json:"spec"`
	ComposeYAML    *string        `json:"compose_yaml"`
	DeployTimeoutS *int           `json:"deploy_timeout_s"`
}

// UpdateAppInput: los campos que faltan (o spec null) no se tocan.
type UpdateAppInput struct {
	Project        string         `json:"-"`
	Stage          string         `json:"-"`
	Slug           string         `json:"-"`
	Spec           jsontext.Value `json:"spec"`
	ComposeYAML    *string        `json:"compose_yaml"`
	DeployTimeoutS *int           `json:"deploy_timeout_s"`
}

type AppOutput struct {
	Project         string         `json:"project"`
	Stage           string         `json:"stage"`
	Slug            string         `json:"slug"`
	Kind            string         `json:"kind"`
	Spec            jsontext.Value `json:"spec,omitzero"`
	ComposeYAML     string         `json:"compose_yaml,omitempty"`
	DeployTimeoutS  int            `json:"deploy_timeout_s"`
	ConfigChangedAt time.Time      `json:"config_changed_at"`
	LastDeployedAt  *time.Time     `json:"last_deployed_at"`
	PendingChanges  bool           `json:"pending_changes"`
	CreatedAt       time.Time      `json:"created_at"`
	// Services son los servicios de la app (contrato §2.12), ordenados por
	// nombre; nunca null.
	Services []ServiceOutput `json:"services"`
}

// ServiceOutput es un servicio de una app y los puertos que declara: el port
// del spec en una de formulario, los de expose en una compose. Solo una
// sugerencia para el destino de un dominio: no se publican.
type ServiceOutput struct {
	Name  string `json:"name"`
	Ports []int  `json:"ports"`
}

func AppFrom(a entity.App) AppOutput {
	out := AppOutput{
		Project: a.ProjectSlug, Stage: a.StageSlug, Slug: a.Slug, Kind: string(a.Kind),
		ComposeYAML: a.ComposeYAML, DeployTimeoutS: a.DeployTimeoutS,
		ConfigChangedAt: a.ConfigChangedAt, LastDeployedAt: a.LastDeployedAt,
		PendingChanges: a.PendingChanges(), CreatedAt: a.CreatedAt,
	}
	if a.Spec != nil {
		out.Spec = jsontext.Value(a.Spec)
	}
	out.Services = servicesOf(a)
	return out
}

// servicesOf lee los servicios de la configuración guardada, no de Swarm. En
// una app de formulario es uno, con el slug; en una compose, los de su compose
// ([] si no se puede leer, como uno guardado antes de validarlo).
func servicesOf(a entity.App) []ServiceOutput {
	out := []ServiceOutput{}
	switch a.Kind {
	case entity.KindService:
		var spec struct {
			Port int `json:"port"`
		}
		ports := []int{}
		if jsonv2.Unmarshal(a.Spec, &spec) == nil && spec.Port > 0 {
			ports = append(ports, spec.Port)
		}
		out = append(out, ServiceOutput{Name: a.Slug, Ports: ports})
	case entity.KindCompose:
		for _, s := range compose.Services(a.ComposeYAML) {
			out = append(out, ServiceOutput{Name: s.Name, Ports: s.Ports})
		}
	}
	return out
}
