package entity

import (
	"errors"
	"strings"
	"time"

	"github.com/CPU-commits/muelle/src/internal/catalog/domain/vo"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// Kind es el tipo de app: de formulario (un servicio con spec) o un compose.
type Kind string

const (
	KindService Kind = "service"
	KindCompose Kind = "compose"
)

// ErrWrongKind: el cambio no corresponde al tipo de la app (el spec de una
// compose o el compose de una de formulario). El caso de uso lo traduce a
// service.ErrKindMismatch.
var ErrWrongKind = errors.New("el cambio no corresponde al tipo de app")

const (
	DefaultDeployTimeoutS = 300
	MinDeployTimeoutS     = 30
	MaxDeployTimeoutS     = 3600

	// En la Fase 1 el compose solo se guarda; la Fase 2 lo valida con compose-go.
	MaxComposeBytes = 256 << 10
)

// App es un servicio de formulario (Spec) o un compose (ComposeYAML), nunca ambos.
type App struct {
	ID              int64
	StageID         int64
	ProjectSlug     string
	StageSlug       string
	Slug            string
	Kind            Kind
	Spec            []byte // JSON normalizado (vo.AppSpec.JSON()); solo KindService
	ComposeYAML     string // solo KindCompose
	DeployTimeoutS  int
	ConfigChangedAt time.Time
	LastDeployedAt  *time.Time
	CreatedAt       time.Time
}

// NewServiceApp crea una app de formulario. Valida el spec y guarda su JSON
// normalizado, así que un spec sin validar no llega a la base.
// timeoutS 0 usa DefaultDeployTimeoutS.
func NewServiceApp(stage Stage, slug string, spec vo.AppSpec, timeoutS int, now time.Time) (App, error) {
	if err := spec.Validate(); err != nil {
		return App{}, err
	}
	a, err := newApp(stage, slug, KindService, timeoutS, now)
	if err != nil {
		return App{}, err
	}
	a.Spec = spec.JSON()
	return a, nil
}

// NewComposeApp crea una app compose. timeoutS 0 usa DefaultDeployTimeoutS.
func NewComposeApp(stage Stage, slug, composeYAML string, timeoutS int, now time.Time) (App, error) {
	if err := ValidateCompose(composeYAML); err != nil {
		return App{}, err
	}
	a, err := newApp(stage, slug, KindCompose, timeoutS, now)
	if err != nil {
		return App{}, err
	}
	a.ComposeYAML = composeYAML
	return a, nil
}

func newApp(stage Stage, slug string, kind Kind, timeoutS int, now time.Time) (App, error) {
	if err := validate.AppSlug(slug); err != nil {
		return App{}, err
	}
	if timeoutS == 0 {
		timeoutS = DefaultDeployTimeoutS
	} else if err := ValidateDeployTimeout(timeoutS); err != nil {
		return App{}, err
	}
	return App{
		StageID: stage.ID, ProjectSlug: stage.ProjectSlug, StageSlug: stage.Slug,
		Slug: slug, Kind: kind, DeployTimeoutS: timeoutS,
		ConfigChangedAt: now, CreatedAt: now,
	}, nil
}

// ValidateCompose exige un compose no vacío y de hasta 256 KiB.
func ValidateCompose(yaml string) error {
	if strings.TrimSpace(yaml) == "" {
		return validate.Errorf("compose_yaml", "es obligatorio")
	}
	if len(yaml) > MaxComposeBytes {
		return validate.Errorf("compose_yaml", "no puede superar 256 KiB")
	}
	return nil
}

func ValidateDeployTimeout(s int) error {
	if s < MinDeployTimeoutS || s > MaxDeployTimeoutS {
		return validate.Errorf("deploy_timeout_s", "debe estar entre %d y %d", MinDeployTimeoutS, MaxDeployTimeoutS)
	}
	return nil
}

// ChangeSpec valida y reemplaza el spec, y marca cambios pendientes.
// ErrWrongKind si la app no es KindService.
func (a *App) ChangeSpec(spec vo.AppSpec, now time.Time) error {
	if a.Kind != KindService {
		return ErrWrongKind
	}
	if err := spec.Validate(); err != nil {
		return err
	}
	a.Spec = spec.JSON()
	a.ConfigChangedAt = now
	return nil
}

// ChangeCompose reemplaza el compose y marca cambios pendientes.
// ErrWrongKind si la app no es KindCompose.
func (a *App) ChangeCompose(yaml string, now time.Time) error {
	if a.Kind != KindCompose {
		return ErrWrongKind
	}
	if err := ValidateCompose(yaml); err != nil {
		return err
	}
	a.ComposeYAML = yaml
	a.ConfigChangedAt = now
	return nil
}

// SetDeployTimeout cambia el timeout. No cuenta como cambio de configuración.
func (a *App) SetDeployTimeout(s int) error {
	if err := ValidateDeployTimeout(s); err != nil {
		return err
	}
	a.DeployTimeoutS = s
	return nil
}

// PendingChanges indica si la configuración cambió después del último deploy.
// Una app que nunca se desplegó no tiene "cambios pendientes": no tiene deploy
// contra el cual comparar.
func (a App) PendingChanges() bool {
	return a.LastDeployedAt != nil && a.ConfigChangedAt.After(*a.LastDeployedAt)
}
