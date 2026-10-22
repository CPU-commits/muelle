// Package secrets son las variables cifradas de stages y apps: leerlas
// enmascaradas, reemplazarlas (PUT), modificarlas (PATCH) y mezclarlas para
// el deploy. module.go es el único lugar que conoce todas sus piezas.
package secrets

import (
	"context"
	"log/slog"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/cmd/mcp/tools"
	"github.com/CPU-commits/muelle/src/internal/secrets/app/dto"
	"github.com/CPU-commits/muelle/src/internal/secrets/app/usecases"
	"github.com/CPU-commits/muelle/src/internal/secrets/domain/service"
	envhttp "github.com/CPU-commits/muelle/src/internal/secrets/infra/in/http"
	envsqlite "github.com/CPU-commits/muelle/src/internal/secrets/infra/out/repository/sqlite"
	outservice "github.com/CPU-commits/muelle/src/internal/secrets/infra/out/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/cipher"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

type Deps struct {
	DB     *sqlite.DB
	Cipher cipher.Cipher
	Clock  clock.Clock
	Audit  audit.Recorder
	Log    *slog.Logger
}

// Module arma secrets.
type Module struct {
	env     *envhttp.EnvHandler
	resolve *usecases.ResolveEnv
}

func New(d Deps) *Module {
	repo := envsqlite.NewEnvRepository(d.DB)
	owners := outservice.NewOwnerResolver(d.DB)
	marker := outservice.NewChangeMarker(d.DB)
	ec := service.NewEnvCrypto(d.Cipher)
	return &Module{
		env: envhttp.NewEnvHandler(
			usecases.NewGetEnv(owners, repo, ec),
			usecases.NewReplaceEnv(d.DB, owners, repo, marker, ec, d.Clock, d.Audit),
			usecases.NewPatchEnv(d.DB, owners, repo, marker, ec, d.Clock, d.Audit),
		),
		resolve: usecases.NewResolveEnv(d.DB, owners, repo, ec),
	}
}

// ResolveEnv es lo que recibe una app al desplegarse (§5.2 regla 3; lo llama
// deploy en la Fase 2, por el puerto de su domain/service): las variables de
// su stage con las de la app encima (la app gana), ya descifradas, y los
// valores de las secretas de los dos (también las del stage que la app pisa),
// sin repetidos ni vacíos y de los más largos a los más cortos, para que el
// deploy los reemplace por *** en su log antes de guardarlo o transmitirlo
// (§7). El stage sale de la propia app: nunca mezcla las variables de otro.
// Si la app no existe devuelve repository.ErrNotFound. Nada de esto va a la
// API ni a la auditoría.
func (m *Module) ResolveEnv(ctx context.Context, appID int64) (vars map[string]string, secretValues []string, err error) {
	out, err := m.resolve.Execute(ctx, dto.ResolveEnv{AppID: appID})
	return out.Vars, out.SecretValues, err
}

func (m *Module) RegisterRoutes(k *utils.Kit) {
	m.env.Register(k)
}

// RegisterTools registra las herramientas MCP de secrets (contrato §8.3).
func (m *Module) RegisterTools(k *tools.Kit) {}
