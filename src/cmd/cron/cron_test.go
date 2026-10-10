package cron

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// Los tests corren en una burbuja de synctest: el reloj es falso y avanza
// solo cuando todas las goroutines esperan, así que los tiempos son exactos.

func TestRunsEveryInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var n atomic.Int32
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		Run(ctx, []Job{{Name: "cuenta", Every: 10 * time.Millisecond, Run: func(context.Context) error {
			n.Add(1)
			return nil
		}}}, discard())
		if got := n.Load(); got < 2 {
			t.Fatalf("el job corrió %d veces en 50 ms, esperaba al menos 2", got)
		}
	})
}

func TestFirstRunAfterEvery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var n atomic.Int32
		ctx, cancel := context.WithTimeout(context.Background(), 59*time.Minute)
		defer cancel()
		Run(ctx, []Job{{Name: "horaria", Every: time.Hour, Run: func(context.Context) error {
			n.Add(1)
			return nil
		}}}, discard())
		if got := n.Load(); got != 0 {
			t.Fatalf("el job no debe correr antes de Every, corrió %d veces", got)
		}
	})
}

func TestErrorDoesNotStopJob(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var logs bytes.Buffer
		var n atomic.Int32
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		Run(ctx, []Job{{Name: "falla", Every: 10 * time.Millisecond, Run: func(context.Context) error {
			n.Add(1)
			return errors.New("se cayó")
		}}}, slog.New(slog.NewTextHandler(&logs, nil)))
		if got := n.Load(); got < 2 {
			t.Fatalf("un job que falla debe seguir corriendo, corrió %d veces", got)
		}
		if out := logs.String(); !strings.Contains(out, "job=falla") || !strings.Contains(out, "se cayó") {
			t.Fatalf("el error debe quedar en el log: %q", out)
		}
	})
}

func TestPanicDoesNotStopJob(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var logs bytes.Buffer
		var n atomic.Int32
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		Run(ctx, []Job{{Name: "explota", Every: 10 * time.Millisecond, Run: func(context.Context) error {
			n.Add(1)
			panic("boom")
		}}}, slog.New(slog.NewTextHandler(&logs, nil)))
		if got := n.Load(); got < 2 {
			t.Fatalf("un job que hace panic debe seguir corriendo, corrió %d veces", got)
		}
		if out := logs.String(); !strings.Contains(out, "job=explota") || !strings.Contains(out, "boom") {
			t.Fatalf("el panic debe quedar en el log: %q", out)
		}
	})
}

func TestJobsRunIndependently(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var fast, slow atomic.Int32
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		Run(ctx, []Job{
			{Name: "rápido", Every: 10 * time.Millisecond, Run: func(context.Context) error {
				fast.Add(1)
				return nil
			}},
			{Name: "lento", Every: 30 * time.Millisecond, Run: func(ctx context.Context) error {
				slow.Add(1)
				time.Sleep(25 * time.Millisecond) // no frena al otro job
				return nil
			}},
		}, discard())
		if f, s := fast.Load(), slow.Load(); f < 9 || s < 2 {
			t.Fatalf("rápido=%d lento=%d", f, s)
		}
	})
}

func TestRunStopsWhenContextIsCanceled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, jobs := range [][]Job{nil, {{Name: "x", Every: time.Millisecond, Run: func(context.Context) error { return nil }}}} {
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				Run(ctx, jobs, discard())
				close(done)
			}()
			time.Sleep(time.Second)
			synctest.Wait()
			select {
			case <-done:
				t.Fatalf("Run (%d jobs) terminó antes de cancelar el ctx", len(jobs))
			default:
			}
			cancel()
			<-done
		}
	})
}

// Al cancelar, Run espera a que termine la ejecución en curso (apagado ordenado).
func TestRunWaitsForRunningJob(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var finished atomic.Bool
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
		defer cancel()
		Run(ctx, []Job{{Name: "larga", Every: 10 * time.Millisecond, Run: func(ctx context.Context) error {
			<-ctx.Done()
			time.Sleep(time.Second)
			finished.Store(true)
			return ctx.Err()
		}}}, discard())
		if !finished.Load() {
			t.Fatal("Run volvió antes de que terminara el job en curso")
		}
	})
}

func TestRunRejectsNonPositiveEvery(t *testing.T) {
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "sin-intervalo") {
			t.Fatalf("recover() = %v, esperaba un panic que nombre el job", r)
		}
	}()
	Run(context.Background(), []Job{{Name: "sin-intervalo", Run: func(context.Context) error { return nil }}}, discard())
}
