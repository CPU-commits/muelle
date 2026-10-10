// Package cron ejecuta las tareas periódicas que registran los módulos.
package cron

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type Job struct {
	Name  string
	Every time.Duration
	Run   func(ctx context.Context) error
}

// Run ejecuta cada job una vez al arrancar y después cada Every hasta que ctx
// se cancele. Un error se registra en el log y no detiene al job. Bloquea hasta que ctx termina.
// La ejecución inicial evita que un muelle que se reinicia más seguido que
// Every nunca corra un job (ej. el backup).
//
// Cada job corre en su propia goroutine, así que uno lento no frena a los
// demás, y nunca se superpone consigo mismo: si una ejecución tarda más que
// Every, los ticks perdidos se saltan. Un panic dentro de un job se registra
// como un error. Al cancelar ctx, Run espera a que terminen las ejecuciones
// en curso, que reciben ese mismo ctx. Un Every <= 0 es un error de
// programación y hace panic antes de arrancar ningún job.
func Run(ctx context.Context, jobs []Job, log *slog.Logger) {
	if log == nil {
		log = slog.Default()
	}
	for _, j := range jobs {
		if j.Every <= 0 {
			panic(fmt.Sprintf("cron: el job %q tiene Every <= 0", j.Name))
		}
	}
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Go(func() { loop(ctx, j, log) })
	}
	<-ctx.Done()
	wg.Wait()
}

func loop(ctx context.Context, j Job, log *slog.Logger) {
	t := time.NewTicker(j.Every)
	defer t.Stop()
	for {
		// Si ctx se canceló (también justo al mismo tiempo que el tick), no se
		// empieza otra ejecución.
		if ctx.Err() != nil {
			return
		}
		if err := runOnce(ctx, j); err != nil {
			log.Error("tarea periódica", "job", j.Name, "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func runOnce(ctx context.Context, j Job) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return j.Run(ctx)
}
