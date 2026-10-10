// Comando muelle: `muelle serve` levanta el panel y la API; `muelle genkey`
// imprime una clave maestra nueva (para desarrollo local; en producción la
// genera `muelle install`, Fase 5).
package main

import (
	"context"
	"errors"
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
	"github.com/CPU-commits/muelle/src/settings"
	"github.com/CPU-commits/muelle/src/shared/infra/container"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
)

const usage = `uso: muelle <comando>

comandos:
  serve    levanta el panel y la API (configuración por variables MUELLE_*)
  genkey   imprime una clave maestra nueva en base64
  help     muestra esta ayuda (también -h y --help)`

const (
	// memoryLimit es el límite blando de memoria de serve si GOMEMLIMIT no
	// está definido. Argon2id usa 19 MiB por hash, hasta 2 a la vez: cerca
	// del límite el GC trabaja más seguido, y una ráfaga de logins deja el
	// RSS en ~60 MB en vez de ~100 MB, lo que permite que muelle + Caddy
	// quepan en los 100 MB del §1 del spec. El RSS de reposo (< 40 MB)
	// vuelve después del GC que el runtime fuerza cada 2 min. Es blando: si
	// de verdad hace falta más memoria, el runtime la usa en vez de fallar.
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
// código de salida: 0 si terminó bien, 1 si serve falló y 2 si los
// argumentos no sirven (con el uso en stderr).
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	cmd, extra := args[0], args[1:]
	switch cmd {
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, usage)
		return 0
	case "serve", "genkey":
		if len(extra) > 0 {
			fmt.Fprintf(stderr, "muelle %s: no recibe argumentos (sobra %q)\n\n%s\n", cmd, extra[0], usage)
			return 2
		}
	default:
		fmt.Fprintf(stderr, "muelle: comando desconocido %q\n\n%s\n", cmd, usage)
		return 2
	}

	if cmd == "genkey" {
		fmt.Fprintln(stdout, crypto.GenerateKey())
		return 0
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
}

func serve(log *slog.Logger) error {
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(memoryLimit)
	}
	s, err := settings.Load()
	if err != nil {
		return err
	}

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
		log.Info("todavía no hay usuarios: abre este link para crear el primero (vence en 24 h)",
			"url", s.PanelOrigin+"/setup?token="+token)
	}

	// Las tareas periódicas usan la base: antes de cerrarla (el defer de
	// arriba) se detienen y se espera a que terminen.
	cronCtx, stopCron := context.WithCancel(ctx)
	cronDone := make(chan struct{})
	go func() {
		defer close(cronDone)
		cron.Run(cronCtx, c.Jobs, log)
	}()
	defer func() {
		stopCron()
		<-cronDone
	}()

	errc := make(chan error, 1)
	go func() {
		log.Info("muelle escuchando", "addr", ln.Addr().String())
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

// newHTTPServer arma el servidor HTTP con los plazos de muelle. En producción
// está detrás de Caddy, que no limita la lectura y reenvía el cuerpo a medida
// que llega: los plazos de lectura tienen que estar acá.
//
//   - ReadHeaderTimeout: los headers tienen que llegar en 10 s.
//   - ReadTimeout: la petición entera, cuerpo incluido, en 30 s (los cuerpos
//     son JSON de 1 MiB como mucho). Un cliente que manda un byte a la vez ya
//     no ocupa la conexión para siempre.
//   - IdleTimeout: una conexión keep-alive sin peticiones se cierra a los 120 s.
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
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
}
