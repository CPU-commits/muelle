// Package container arma muelle completo: la base, la clave maestra, el
// cliente de Docker, los seis módulos y el servidor HTTP con sus rutas. Lo
// usan main (`muelle serve`) y los tests de integración; cada llamada a New
// arma su propio muelle, sin estado global.
//
// Es el único lugar que conecta los módulos entre sí (§4.1 regla 1), por los
// puertos que cada uno declara en su domain/service:
//
//	catalog → ingress  la red de cada stage (StageNetworks)
//	catalog → deploy   docker stack rm al borrar una app (AppRuntime)
//	deploy  → secrets  las variables de la app y sus secretos (EnvResolver)
//	deploy  → ingress  la red antes del deploy y las rutas después (Ingress)
//	admin   → ingress  verificar el token de Cloudflare y aplicar los ajustes (ProxyHook)
//
// Lo que un módulo lee de las tablas de otro va por sus adaptadores SQL:
// deploy lee apps (y escribe solo storage_node_id y last_deployed_at) y
// domains; ingress lee settings; catalog cuenta domains; admin mira users.
package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/CPU-commits/muelle/src/cmd/cron"
	httpserver "github.com/CPU-commits/muelle/src/cmd/http"
	"github.com/CPU-commits/muelle/src/internal/admin"
	"github.com/CPU-commits/muelle/src/internal/catalog"
	"github.com/CPU-commits/muelle/src/internal/deploy"
	"github.com/CPU-commits/muelle/src/internal/identity"
	"github.com/CPU-commits/muelle/src/internal/ingress"
	"github.com/CPU-commits/muelle/src/internal/secrets"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/settings"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

type Container struct {
	DB       *sqlite.DB
	Docker   docker.Engine
	Identity *identity.Module
	Catalog  *catalog.Module
	Secrets  *secrets.Module
	Admin    *admin.Module
	Deploy   *deploy.Module
	Ingress  *ingress.Module
	Server   *httpserver.Server
	Jobs     []cron.Job // las tareas periódicas de los módulos, para cron.Run

	closeDocker func() error // nil si el Docker vino de WithDocker
}

// Option cambia cómo se arma el contenedor.
type Option func(*options)

type options struct {
	docker docker.Engine
}

// WithDocker usa e en vez del cliente de Docker del entorno (DOCKER_HOST o el
// socket por defecto). Los tests pasan un dockertest.Fake: así nunca tocan el
// Docker real de la máquina. El contenedor no lo cierra.
func WithDocker(e docker.Engine) Option { return func(o *options) { o.docker = e } }

// New carga la clave maestra (s.MasterKeyFile), arma el cliente de Docker (sin
// conectarse: muelle arranca aunque Docker no responda), abre la base en
// <DataDir>/muelle.db (crea DataDir con 0700 si no existe) y la migra. Arma
// los seis módulos con el reloj del sistema y un AuditRecorder compartido,
// verifica la clave maestra (admin.VerifyMasterKey: con otra clave devuelve un
// error que envuelve admin.ErrWrongMasterKey), guarda los ajustes de arranque
// (admin.ApplyStartupSettings: panel_domain y MUELLE_INIT_*), arma el servidor
// con el Authenticator de identity y registra las rutas de todos los módulos.
// Nada de esto llama a Docker: lo que corre en segundo plano arranca con Run.
//
// Sin PanelOrigin o DataDir devuelve un error sin tocar nada. La clave se lee
// antes de tocar DataDir, así una clave que falta no deja una base vacía; si
// falta o no sirve, el error nombra MUELLE_MASTER_KEY_FILE y `muelle genkey`.
// Si algo falla, cierra lo que alcanzó a abrir.
func New(ctx context.Context, s settings.Settings, log *slog.Logger, opts ...Option) (*Container, error) {
	// Sin estos valores un módulo haría panic (identity y admin los exigen).
	switch {
	case s.PanelOrigin == "":
		return nil, errors.New("container: falta PanelOrigin (MUELLE_PANEL_ORIGIN)")
	case s.DataDir == "":
		return nil, errors.New("container: falta DataDir (MUELLE_DATA_DIR)")
	}
	if log == nil {
		log = slog.Default()
	}
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	ciph, err := loadCipher(s.MasterKeyFile)
	if err != nil {
		return nil, err
	}
	engine, closeDocker := o.docker, func() error { return nil }
	if engine == nil {
		client, err := docker.New(docker.Options{})
		if err != nil {
			return nil, fmt.Errorf("container: %w", err)
		}
		engine, closeDocker = client, client.Close
	}

	if err := os.MkdirAll(s.DataDir, 0o700); err != nil {
		return nil, errors.Join(fmt.Errorf("container: creando %s: %w", s.DataDir, err), closeDocker())
	}
	db, err := sqlite.Open(filepath.Join(s.DataDir, "muelle.db"))
	if err != nil {
		return nil, errors.Join(err, closeDocker())
	}
	c, err := build(ctx, db, ciph, engine, s, log)
	if err != nil {
		return nil, errors.Join(err, db.Close(), closeDocker())
	}
	c.closeDocker = closeDocker
	return c, nil
}

// loadCipher lee la clave maestra de path y arma el cifrador. Si falla, el
// error dice de dónde se leyó y cómo crear una clave nueva.
func loadCipher(path string) (*crypto.Cipher, error) {
	key, err := crypto.LoadKeyFile(path)
	if err != nil {
		return nil, keyError(path, err)
	}
	ciph, err := crypto.New(key)
	if err != nil {
		return nil, keyError(path, err)
	}
	return ciph, nil
}

func keyError(path string, err error) error {
	return fmt.Errorf("container: MUELLE_MASTER_KEY_FILE=%s: %w (en una instalación nueva, crea la clave con `muelle genkey`; si ya hay datos, hace falta la clave con la que se cifraron)", path, err)
}

func build(ctx context.Context, db *sqlite.DB, ciph *crypto.Cipher, engine docker.Engine, s settings.Settings, log *slog.Logger) (*Container, error) {
	if err := db.Migrate(ctx); err != nil {
		return nil, err
	}

	clk := clock.System{}
	rec := sqlite.NewAuditRecorder(db, clk, log)
	c := &Container{DB: db, Docker: engine}
	// En orden de dependencias: cada módulo recibe los que ya están armados.
	c.Ingress = ingress.New(ingress.Deps{
		DB: db, Cipher: ciph, Clock: clk, Audit: rec, Log: log, Docker: engine, Version: s.Version,
	})
	c.Secrets = secrets.New(secrets.Deps{DB: db, Cipher: ciph, Clock: clk, Audit: rec, Log: log})
	c.Deploy = deploy.New(deploy.Deps{
		DB: db, Cipher: ciph, Clock: clk, Audit: rec, Log: log, Docker: engine,
		Env: c.Secrets, Ingress: c.Ingress,
	})
	c.Catalog = catalog.New(catalog.Deps{
		DB: db, Clock: clk, Audit: rec, Log: log, Networks: c.Ingress, Runtime: c.Deploy,
	})
	c.Admin = admin.New(admin.Deps{
		DB: db, Cipher: ciph, Clock: clk, Audit: rec, Log: log, DataDir: s.DataDir, ProxyHook: c.Ingress,
	})
	c.Identity = identity.New(identity.Deps{
		DB: db, Cipher: ciph, Clock: clk, Audit: rec, Log: log, PanelOrigin: s.PanelOrigin,
	})
	if err := c.Admin.VerifyMasterKey(ctx); err != nil {
		return nil, err
	}
	i := s.Init
	if err := c.Admin.ApplyStartupSettings(ctx, s.PanelDomain(), admin.InitialSettings{
		PublicIP: i.PublicIP, PublicIPv6: i.PublicIPv6, ACMEEmail: i.ACMEEmail,
		ProxyHTTPPort: i.ProxyHTTPPort, ProxyHTTPSPort: i.ProxyHTTPSPort,
		CloudflareToken: i.CloudflareToken, CaddyImage: i.CaddyImage,
	}); err != nil {
		return nil, fmt.Errorf("container: guardando los ajustes de arranque: %w", err)
	}

	c.Server = httpserver.NewServer(httpserver.Config{
		PanelOrigin:   s.PanelOrigin,
		SecureCookies: s.SecureCookies,
		Auth:          c.Identity.Authenticator(),
		Logger:        log,
		ProxyOrigin:   c.Admin.PanelOrigin,
	})
	k := c.Server.Kit()
	c.Identity.RegisterRoutes(k)
	c.Catalog.RegisterRoutes(k)
	c.Secrets.RegisterRoutes(k)
	c.Admin.RegisterRoutes(k)
	c.Deploy.RegisterRoutes(k)
	c.Ingress.RegisterRoutes(k)

	c.Jobs = slices.Concat(c.Identity.Jobs(), c.Admin.Jobs(), c.Ingress.Jobs(), c.Deploy.Jobs())
	return c, nil
}

// Run corre lo que los módulos tienen en segundo plano hasta que ctx se
// cancela, y vuelve cuando todo terminó: los workers de deploy, que primero
// reconcilian los deploys que quedaron a medias (§7 "Reconciliación al
// arrancar"), y los reintentos de ingress (§8). La reconciliación de Caddy y
// las redes (§3.1) es el job "proxy" de ingress, que cron.Run corre también al
// arrancar. main lo llama junto con cron.Run, después de New y antes de
// atender peticiones, y lo espera antes de Close. Un error de Docker al
// arrancar no detiene a muelle: el panel sigue y los endpoints que usan Docker
// responden 503.
func (c *Container) Run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Go(func() { c.Deploy.Run(ctx) })
	wg.Go(func() { c.Ingress.Run(ctx) })
	wg.Wait()
}

// Close cierra la base y el cliente de Docker. Hay que llamarlo después de
// apagar el servidor, cron.Run y Run, que los usan.
func (c *Container) Close() error {
	err := c.DB.Close()
	if c.closeDocker != nil {
		err = errors.Join(err, c.closeDocker())
	}
	return err
}
