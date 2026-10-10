// Comando muelle: `muelle serve` levanta el panel y la API; `muelle genkey`
// imprime una clave maestra nueva (para desarrollo local; en producción la
// genera `muelle install`, Fase 5).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
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
  genkey   imprime una clave maestra nueva en base64`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = serve()
	case "genkey":
		fmt.Println(crypto.GenerateKey())
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		slog.Error("muelle terminó con error", "err", err)
		os.Exit(1)
	}
}

func serve() error {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	s, err := settings.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c, err := container.New(ctx, s, log)
	if err != nil {
		return err
	}
	defer c.Close()

	// Se escucha antes que nada, así un puerto ocupado falla acá: sin
	// arrancar las tareas periódicas ni anular el link de setup anterior.
	ln, err := net.Listen("tcp", s.Listen)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           c.Server,
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	if token, created, err := c.Identity.EnsureSetupInvite(ctx); err != nil {
		ln.Close()
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
		log.Info("apagando")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
