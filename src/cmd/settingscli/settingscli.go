// Package settingscli es `muelle settings <subcomando>`. Por ahora solo
// reset-panel-ips, que recupera el acceso al panel (contrato §4.1 y §5.4).
// Corre dentro del contenedor de muelle, con su entorno.
package settingscli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CPU-commits/muelle/src/internal/admin"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// execHint es cómo se corre en el contenedor de muelle desde el host.
const execHint = `docker exec "$(docker ps -q -f label=com.docker.swarm.service.name=muelle_muelle | head -n1)" muelle settings reset-panel-ips`

const Usage = `uso: muelle settings <subcomando>

subcomandos:
  reset-panel-ips [--no-restart]   vacía la lista de IPs permitidas del panel y reinicia muelle

Corre dentro del contenedor de muelle:
  ` + execHint

// dbFile es la base de muelle dentro de DataDir (container.New).
const dbFile = "muelle.db"

// dockerTimeout acota las llamadas a Docker para reiniciar: el socket es
// local, y si no responde en este plazo, el comando a mano sirve igual.
const dockerTimeout = 30 * time.Second

// Engine es lo que usa de Docker: su propio contenedor y reiniciar su servicio.
type Engine interface {
	InspectContainer(ctx context.Context, nameOrID string) (docker.Container, error)
	docker.ServiceRestarter
}

type Deps struct {
	DataDir  func() (string, error)                             // MUELLE_DATA_DIR (main: settings.Load)
	Hostname func() (string, error)                             // os.Hostname: en una tarea de Swarm, el id corto del contenedor
	Docker   func() (e Engine, closeFn func() error, err error) // se arma recién al reiniciar
	Log      *slog.Logger                                       // los errores de la auditoría; nil: slog.Default
}

// Run ejecuta `muelle settings <args>` y devuelve el código de salida:
// 0 bien, 1 si algo falló, 2 si los argumentos no sirven.
func Run(ctx context.Context, args []string, d Deps, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, Usage)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, Usage)
		return 0
	case "reset-panel-ips":
		return resetPanelIPs(ctx, args[1:], d, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "muelle settings: subcomando desconocido %q\n\n%s\n", args[0], Usage)
		return 2
	}
}

func resetPanelIPs(ctx context.Context, args []string, d Deps, stdout, stderr io.Writer) int {
	const cmd = "muelle settings reset-panel-ips"
	fset := flag.NewFlagSet("reset-panel-ips", flag.ContinueOnError)
	fset.SetOutput(io.Discard)
	noRestart := fset.Bool("no-restart", false, "solo cambia la base, sin reiniciar muelle")
	if err := fset.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(stdout, Usage)
			return 0
		}
		fmt.Fprintf(stderr, "%s: %v\n\n%s\n", cmd, err, Usage)
		return 2
	}
	if fset.NArg() > 0 {
		fmt.Fprintf(stderr, "%s: no recibe argumentos (sobra %q)\n\n%s\n", cmd, fset.Arg(0), Usage)
		return 2
	}

	// La base: sin el entorno de muelle o sin su archivo, esto no corre en
	// su contenedor.
	outside := func(err error) int {
		fmt.Fprintf(stderr, "%s: %v\n\nCorre este comando dentro del contenedor de muelle, desde el host:\n  %s\n", cmd, err, execHint)
		return 1
	}
	dir, err := d.DataDir()
	if err != nil {
		return outside(err)
	}
	path := filepath.Join(dir, dbFile)
	// sqlite.Open crearía una base vacía: primero se mira que exista.
	if st, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return outside(fmt.Errorf("no está la base de muelle (%s)", path))
		}
		return outside(err)
	} else if !st.Mode().IsRegular() {
		return outside(fmt.Errorf("%s no es un archivo", path))
	}
	res, err := reset(ctx, path, d.Log)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", cmd, err)
		return 1
	}
	if res.Changed {
		if res.Previous != nil {
			fmt.Fprintf(stdout, "Vacié la lista de IPs permitidas del panel (quité %s).\n", strings.Join(res.Previous, ", "))
		} else {
			fmt.Fprintln(stdout, "Vacié la lista de IPs permitidas del panel (el valor guardado no se entendía).")
		}
	} else {
		fmt.Fprintln(stdout, "La lista de IPs permitidas del panel ya estaba vacía: si no llegas al panel, el problema es otro (¿los puertos del proxy? ver \"Recuperar el acceso\" en el README).")
	}

	if *noRestart {
		if res.Changed {
			fmt.Fprint(stdout, "No reinicio muelle (--no-restart): Caddy sigue filtrando con la lista anterior hasta que muelle le reenvíe la configuración (el job \"proxy\", cada 5 min) o lo reinicies desde el host:\n")
		} else {
			fmt.Fprint(stdout, "No reinicio muelle (--no-restart). Para que le reenvíe la configuración a Caddy, reinícialo desde el host:\n")
		}
		fmt.Fprintf(stdout, "  docker service update --force %s\n", naming.MuelleService)
		return 0
	}
	if !res.Changed {
		fmt.Fprintln(stdout, "Reinicio muelle igual: al arrancar le reenvía la configuración a Caddy.")
	}
	return restart(ctx, d, res.Changed, stdout, stderr)
}

// reset abre la base sin migrar (es la de muelle, que ya corre) y vacía la lista.
func reset(ctx context.Context, path string, log *slog.Logger) (admin.ResetResult, error) {
	db, err := sqlite.Open(path)
	if err != nil {
		return admin.ResetResult{}, err
	}
	defer db.Close()
	res, err := admin.ResetPanelAllowedCIDRs(ctx, db, sqlite.NewAuditRecorder(db, clock.System{}, log))
	if err != nil {
		return admin.ResetResult{}, fmt.Errorf("vaciando la lista de IPs permitidas: %w", err)
	}
	return res, nil
}

// restart reinicia el servicio de su propio contenedor, como docker service
// update --force. Si no puede, la base ya quedó cambiada: lo dice y da el
// comando para hacerlo a mano.
func restart(ctx context.Context, d Deps, changed bool, stdout, stderr io.Writer) int {
	service := naming.MuelleService // el del comando a mano, si no se lee la etiqueta
	fail := func(err error) int {
		fmt.Fprintf(stderr, "muelle settings reset-panel-ips: no pude reiniciar muelle: %v\n\n", err)
		if changed {
			fmt.Fprint(stderr, "La lista ya quedó vacía en la base, pero Caddy la sigue usando hasta que muelle le reenvíe la configuración. Reinícialo desde el host:\n")
		} else {
			fmt.Fprint(stderr, "Para que muelle le reenvíe la configuración a Caddy, reinícialo desde el host:\n")
		}
		fmt.Fprintf(stderr, "  docker service update --force %s\n", service)
		return 1
	}
	ctx, cancel := context.WithTimeout(ctx, dockerTimeout)
	defer cancel()

	e, closeFn, err := d.Docker()
	if err != nil {
		return fail(err)
	}
	if closeFn != nil {
		defer closeFn()
	}
	host, err := d.Hostname()
	if err != nil {
		return fail(fmt.Errorf("leyendo el hostname: %w", err))
	}
	ctr, err := e.InspectContainer(ctx, host)
	if err != nil {
		return fail(fmt.Errorf("buscando mi contenedor (hostname %q; ¿corre con docker exec en el contenedor de muelle?): %w", host, err))
	}
	name := ctr.Labels[naming.LabelSwarmService]
	if name == "" {
		return fail(fmt.Errorf("el contenedor %s no es una tarea de Swarm (no tiene la etiqueta %s)", host, naming.LabelSwarmService))
	}
	service = name
	// Antes de reiniciar: Swarm detiene esta tarea (stop-first) y eso puede
	// cortar el docker exec antes de que vuelva ForceUpdateService.
	fmt.Fprintf(stdout, "reiniciando %s: el panel se corta unos segundos…\n", service)
	if err := e.ForceUpdateService(ctx, service); err != nil {
		return fail(err)
	}
	fmt.Fprintln(stdout, "Listo: Swarm reemplaza la tarea de muelle.")
	return 0
}
