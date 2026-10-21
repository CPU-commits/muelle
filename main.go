// Comando muelle: `muelle serve` levanta el panel y la API; `muelle install`
// instala muelle en un servidor (desde la imagen, contrato §5.4); `muelle
// genkey` imprime una clave maestra nueva (para desarrollo local; en
// producción la genera install); `muelle version` imprime la versión del
// binario; `muelle settings reset-panel-ips` recupera el acceso al panel desde
// el contenedor de muelle.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/cron"
	"github.com/CPU-commits/muelle/src/cmd/install"
	"github.com/CPU-commits/muelle/src/cmd/settingscli"
	"github.com/CPU-commits/muelle/src/settings"
	"github.com/CPU-commits/muelle/src/shared/infra/container"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
)

const usage = `uso: muelle <comando>

comandos:
  serve     levanta el panel y la API (configuración por variables MUELLE_*)
  install   instala muelle en este servidor (muelle install -h para los flags)
  genkey    imprime una clave maestra nueva en base64
  settings  ajustes desde el servidor (reset-panel-ips recupera el acceso al panel)
  version   imprime la versión de muelle
  help      muestra esta ayuda (también -h y --help)`

// version es la versión de muelle. La imagen la fija al compilar (Fase 5):
// go build -ldflags "-X main.version=v0.1.0". Con una versión publicada,
// muelle-caddy usa la imagen del mismo tag (naming.CaddyImageFor); "dev" no
// tiene imagen por defecto.
var version = "dev"

const (
	// memoryLimit es el límite blando de memoria de serve si GOMEMLIMIT no
	// está definido. Argon2id usa 19 MiB por hash, hasta 2 a la vez: cerca
	// del límite el GC trabaja más seguido y acota el pico de una ráfaga de
	// logins. Después, el hasher de identity devuelve esa memoria al sistema
	// 5 s después del último hash, así que el RSS vuelve al de reposo
	// (< 40 MB, §1 del spec) sin esperar al GC que el runtime fuerza cada
	// 2 min. Es blando: si de verdad hace falta más memoria, el runtime la usa
	// en vez de fallar.
	memoryLimit = 64 << 20

	// shutdownTimeout es lo que se espera a las peticiones en curso al
	// apagar. Es menor que los 10 s que Docker espera entre SIGTERM y SIGKILL
	// (stop_grace_period por defecto), así muelle alcanza a cerrar la base.
	shutdownTimeout = 8 * time.Second
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run ejecuta el comando de args (sin el nombre del programa) y devuelve el
// código de salida: 0 si terminó bien, 1 si serve, install o settings fallaron y 2 si
// los argumentos no sirven (con el uso en stderr).
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	// Cada comando valida sus propios argumentos.
	cmd, extra := args[0], args[1:]
	switch cmd {
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, usage)
		return 0
	case "version":
		if !noArgs(cmd, extra, stderr) {
			return 2
		}
		fmt.Fprintln(stdout, version)
		return 0
	case "genkey":
		if !noArgs(cmd, extra, stderr) {
			return 2
		}
		fmt.Fprintln(stdout, crypto.GenerateKey())
		return 0
	case "settings":
		return runSettings(extra, stdout, stderr)
	case "install":
		return runInstall(extra, stdout, stderr)
	case "serve":
		if !noArgs(cmd, extra, stderr) {
			return 2
		}
		// También el error final y lo que se registre con slog.Default o el
		// paquete log sale en JSON.
		log := slog.New(slog.NewJSONHandler(stdout, nil))
		slog.SetDefault(log)
		if err := serve(log); err != nil {
			log.Error("muelle terminó con error", "err", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "muelle: comando desconocido %q\n\n%s\n", cmd, usage)
		return 2
	}
}

// runSettings es muelle settings <subcomando> (settingscli), con el entorno
// de muelle y el Docker del socket. Corre dentro del contenedor de muelle.
func runSettings(args []string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return settingscli.Run(ctx, args, settingscli.Deps{
		// settings.Load ya dice qué variable falta; settingscli le agrega la
		// pista de docker exec.
		DataDir: func() (string, error) {
			s, err := settings.Load()
			if err != nil {
				return "", err
			}
			return s.DataDir, nil
		},
		Hostname: os.Hostname,
		Docker: func() (settingscli.Engine, func() error, error) {
			c, err := docker.New(docker.Options{})
			if err != nil {
				return nil, nil, err
			}
			return c, c.Close, nil
		},
		Log: slog.New(slog.NewTextHandler(stderr, nil)),
	}, stdout, stderr)
}

// runInstall es muelle install (contrato §5.4): la clave y el link a stdout,
// el avance y los avisos a stderr. 2 si un flag no sirve, 1 si algo falló.
func runInstall(args []string, stdout, stderr io.Writer) int {
	o, err := install.ParseArgs(args, os.Stdin)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(stdout, install.Usage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "muelle install: %v\n\n%s\n", err, install.Usage)
		return 2
	}
	// Un Ctrl-C cancela; lo creado antes de que el deploy funcione se borra igual.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	cli, err := docker.New(docker.Options{})
	if err != nil {
		fmt.Fprintf(stderr, "muelle install: %v\n", err)
		return 1
	}
	defer cli.Close()
	if err := install.Run(ctx, o, install.Config{Version: version, Docker: cli, Stdout: stdout, Stderr: stderr}); err != nil {
		fmt.Fprintf(stderr, "muelle install: %v\n", err)
		return 1
	}
	return 0
}

// noArgs dice si un comando sin argumentos no recibió ninguno; si recibió
// alguno, escribe el error y el uso en stderr.
func noArgs(cmd string, extra []string, stderr io.Writer) bool {
	if len(extra) == 0 {
		return true
	}
	fmt.Fprintf(stderr, "muelle %s: no recibe argumentos (sobra %q)\n\n%s\n", cmd, extra[0], usage)
	return false
}

func serve(log *slog.Logger) error {
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(memoryLimit)
	}
	s, err := settings.Load()
	if err != nil {
		return err
	}
	s.Version = version

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Se escucha antes de armar el contenedor. Lo único que esto garantiza:
	// si la dirección ya está tomada (por ejemplo, por otra instancia de
	// muelle), serve termina sin leer la clave, sin crear DataDir, sin abrir
	// ni migrar la base y sin anular el link de setup anterior. No protege la
	// base de otra instancia que escuche en otra dirección con el mismo
	// DataDir. Las conexiones que lleguen mientras se arma el contenedor
	// esperan en la cola del kernel hasta que empieza Serve.
	ln, err := net.Listen("tcp", s.Listen)
	if err != nil {
		return fmt.Errorf("escuchando en MUELLE_LISTEN=%s: %w", s.Listen, err)
	}
	defer ln.Close() // Serve también lo cierra; esto cubre los errores de antes

	c, err := container.New(ctx, s, log)
	if err != nil {
		return err
	}
	defer c.Close()

	srv := newHTTPServer(c.Server, log)

	if token, created, err := c.Identity.EnsureSetupInvite(ctx); err != nil {
		return err
	} else if created {
		if err := logSetupURL(log, c.PanelOrigin(ctx), token); err != nil {
			return err
		}
	}

	// Las tareas periódicas y los workers de los módulos (deploy, ingress)
	// usan la base: antes de cerrarla (el defer de arriba) se detienen y se
	// espera a que terminen.
	bgCtx, stopBackground := context.WithCancel(ctx)
	cronDone, runDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(cronDone)
		cron.Run(bgCtx, c.Jobs, log)
	}()
	go func() {
		defer close(runDone)
		c.Run(bgCtx)
	}()
	defer func() {
		stopBackground()
		<-cronDone
		<-runDone
	}()

	errc := make(chan error, 1)
	panel := c.PanelOrigin(ctx)
	go func() {
		logListening(log, ln.Addr().String(), panel)
		errc <- srv.Serve(ln)
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		// Desde acá, una segunda señal ya no se captura: mata el proceso.
		stop()
		log.Info("apagando")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			// Se acabó el plazo con peticiones en curso: se cortan.
			srv.Close()
			return fmt.Errorf("apagando: %w; se cortaron las conexiones que seguían abiertas", err)
		}
		return nil
	}
}

// logSetupURL deja en el log el link de setup de un solo uso. La línea lleva
// event=setup_url: muelle install (Fase 5) sigue los logs del servicio hasta
// encontrarla y le imprime la url al usuario (§3.3 paso 8). El mensaje se
// puede cambiar; event y url no.
//
// Sin origin (sin MUELLE_PANEL_ORIGIN, y los ajustes no se pudieron leer)
// devuelve un error, sin el token, en vez de un link relativo que no abre
// nada: serve termina con él, y el próximo arranque (Swarm lo reinicia) arma
// un link nuevo. install muestra la línea ERROR y sigue esperando.
func logSetupURL(log *slog.Logger, origin, token string) error {
	if origin == "" {
		return errors.New("no se pudo armar el link de setup: falta el origen del panel (sin MUELLE_PANEL_ORIGIN, y el de los ajustes no se pudo leer); el próximo arranque arma uno nuevo")
	}
	log.Info("todavía no hay usuarios: abre este link para crear el primero (vence en 24 h)",
		"event", "setup_url", "url", origin+"/setup?token="+token)
	return nil
}

// logListening deja en el log que serve empezó a atender, con
// event=listening: install la espera después del setup_url (contrato
// §5.4). Lleva el origen del panel (panel, si hay uno): con usuarios, install
// dice que se entre ahí, que sale de los ajustes y no de sus flags. El
// mensaje se puede cambiar; event y panel no.
func logListening(log *slog.Logger, addr, panel string) {
	attrs := []any{"event", "listening", "addr", addr}
	if panel != "" {
		attrs = append(attrs, "panel", panel)
	}
	log.Info("muelle escuchando", attrs...)
}

// newHTTPServer arma el servidor HTTP con los plazos de muelle. En producción
// está detrás de Caddy, que no limita la lectura y reenvía el cuerpo a medida
// que llega: los plazos de lectura tienen que estar acá.
//
//   - ReadHeaderTimeout: los headers tienen que llegar en 10 s.
//   - ReadTimeout: la petición entera, cuerpo incluido, en 30 s (los cuerpos
//     son JSON de 1 MiB como mucho). Un cliente que manda un byte a la vez ya
//     no ocupa la conexión para siempre.
//   - IdleTimeout: una conexión keep-alive sin peticiones se cierra a los 3 min.
//     Tiene que ser mayor que el keepalive de Caddy hacia muelle (2 min por
//     defecto): si muelle cerrara primero, Caddy podría reusar una conexión
//     recién cerrada y responder 502 a una petición que no se puede reintentar.
//   - MaxHeaderBytes: 64 KiB de headers, en vez del 1 MiB por defecto.
//
// No hay WriteTimeout global: los streams SSE de la Fase 2 duran lo que el
// cliente quiera, así que cada handler pondrá su propio plazo con
// http.ResponseController (SetWriteDeadline).
func newHTTPServer(h http.Handler, log *slog.Logger) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       3 * time.Minute,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
}
