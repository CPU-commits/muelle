// Package secrets son las variables cifradas de stages y apps: leerlas
// enmascaradas, reemplazarlas (PUT), modificarlas (PATCH) y mezclarlas para
// el deploy. module.go es el único lugar que conoce todas sus piezas.
package secrets

import (
	"log/slog"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
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

// Module arma secrets. usecases.ResolveEnv (mezclar stage + app) todavía no
// se expone: lo conecta el deploy en la Fase 2.
type Module struct {
	env *envhttp.EnvHandler
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
	}
}

func (m *Module) RegisterRoutes(k *utils.Kit) {
	m.env.Register(k)
}
