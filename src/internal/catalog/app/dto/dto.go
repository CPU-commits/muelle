// Package dto tiene los datos que entran y salen de los casos de uso del
// catálogo. Las etiquetas json son el contrato de la API REST. Los campos con
// json:"-" vienen de la ruta, no del cuerpo.
package dto

import (
	"encoding/json/jsontext"
	"time"

	"github.com/CPU-commits/muelle/src/internal/catalog/domain/entity"
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
	return out
}
