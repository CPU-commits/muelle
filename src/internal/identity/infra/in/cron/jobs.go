// Package cron tiene las tareas periódicas de identity.
package cron

import (
	"context"
	"log/slog"
	"time"

	cmdcron "github.com/CPU-commits/muelle/src/cmd/cron"
	"github.com/CPU-commits/muelle/src/internal/identity/app/usecases"
)

// CleanupSessions borra cada hora las sesiones vencidas o sin uso.
func CleanupSessions(uc *usecases.CleanupSessions, log *slog.Logger) cmdcron.Job {
	return cmdcron.Job{
		Name:  "sesiones vencidas",
		Every: time.Hour,
		Run: func(ctx context.Context) error {
			n, err := uc.Execute(ctx)
			if err != nil {
				return err
			}
			if n > 0 {
				log.Info("sesiones vencidas borradas", "n", n)
			}
			return nil
		},
	}
}
