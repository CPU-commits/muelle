// Package container arma muelle completo: la base, la clave maestra, los
// cuatro módulos y el servidor HTTP con sus rutas. Lo usan main (`muelle
// serve`) y los tests de integración; cada llamada a New arma su propio
// muelle, sin estado global.
package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/CPU-commits/muelle/src/cmd/cron"
	httpserver "github.com/CPU-commits/muelle/src/cmd/http"
	"github.com/CPU-commits/muelle/src/internal/admin"
	"github.com/CPU-commits/muelle/src/internal/catalog"
	"github.com/CPU-commits/muelle/src/internal/identity"
	"github.com/CPU-commits/muelle/src/internal/secrets"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/settings"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

type Container struct {
	DB       *sqlite.DB
	Identity *identity.Module
	Catalog  *catalog.Module
	Secrets  *secrets.Module
	Admin    *admin.Module
	Server   *httpserver.Server
	Jobs     []cron.Job // las tareas periódicas de identity y admin, para cron.Run
}

// New carga la clave maestra (s.MasterKeyFile), abre la base en
// <DataDir>/muelle.db (crea DataDir con 0700 si no existe) y la migra. Arma
// los cuatro módulos con el reloj del sistema y un AuditRecorder compartido,
// verifica la clave maestra (admin.VerifyMasterKey: con otra clave devuelve un
// error que envuelve admin.ErrWrongMasterKey), arma el servidor con el
// Authenticator de identity y registra las rutas de todos los módulos.
//
// La clave se lee antes de tocar DataDir, así una clave que falta no deja
// una base vacía. Si algo falla, cierra lo que alcanzó a abrir.
func New(ctx context.Context, s settings.Settings, log *slog.Logger) (*Container, error) {
	if log == nil {
		log = slog.Default()
	}
	key, err := crypto.LoadKeyFile(s.MasterKeyFile)
	if err != nil {
		return nil, err
	}
	ciph, err := crypto.New(key)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(s.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("container: creando %s: %w", s.DataDir, err)
	}
	db, err := sqlite.Open(filepath.Join(s.DataDir, "muelle.db"))
	if err != nil {
		return nil, err
	}
	c, err := build(ctx, db, ciph, s, log)
	if err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return c, nil
}

func build(ctx context.Context, db *sqlite.DB, ciph *crypto.Cipher, s settings.Settings, log *slog.Logger) (*Container, error) {
	if err := db.Migrate(ctx); err != nil {
		return nil, err
	}

	clk := clock.System{}
	rec := sqlite.NewAuditRecorder(db, clk, log)
	c := &Container{
		DB: db,
		Identity: identity.New(identity.Deps{
			DB: db, Cipher: ciph, Clock: clk, Audit: rec, Log: log, PanelOrigin: s.PanelOrigin,
		}),
		Catalog: catalog.New(catalog.Deps{DB: db, Clock: clk, Audit: rec, Log: log}),
		Secrets: secrets.New(secrets.Deps{DB: db, Cipher: ciph, Clock: clk, Audit: rec, Log: log}),
		Admin: admin.New(admin.Deps{
			DB: db, Cipher: ciph, Clock: clk, Audit: rec, Log: log, DataDir: s.DataDir,
		}),
	}
	if err := c.Admin.VerifyMasterKey(ctx); err != nil {
		return nil, err
	}

	c.Server = httpserver.NewServer(httpserver.Config{
		PanelOrigin:   s.PanelOrigin,
		SecureCookies: s.SecureCookies,
		Auth:          c.Identity.Authenticator(),
		Logger:        log,
	})
	k := c.Server.Kit()
	c.Identity.RegisterRoutes(k)
	c.Catalog.RegisterRoutes(k)
	c.Secrets.RegisterRoutes(k)
	c.Admin.RegisterRoutes(k)

	c.Jobs = slices.Concat(c.Identity.Jobs(), c.Admin.Jobs())
	return c, nil
}

// Close cierra la base. Hay que llamarlo después de apagar el servidor y
// cron.Run, que la usan.
func (c *Container) Close() error { return c.DB.Close() }
