// Package cron arma las tareas periódicas del módulo admin.
package cron

import (
	"context"
	"log/slog"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/cron"
	"github.com/CPU-commits/muelle/src/internal/admin/app/usecases"
)

// Every es la frecuencia de las dos tareas. El backup se crea solo si el del día no existe.
const Every = time.Hour

func Jobs(purge *usecases.PurgeAudit, backup *usecases.Backup, log *slog.Logger) []cron.Job {
	return []cron.Job{
		{
			Name:  "retención de auditoría",
			Every: Every,
			Run: func(ctx context.Context) error {
				n, err := purge.Execute(ctx)
				if err == nil && n > 0 {
					log.Info("auditoría purgada", "eventos", n)
				}
				return err
			},
		},
		{
			Name:  "backup diario",
			Every: Every,
			Run: func(ctx context.Context) error {
				out, err := backup.Execute(ctx)
				if err == nil && out.Created {
					log.Info("backup diario", "archivo", out.Name, "borradas", out.Removed)
				}
				return err
			},
		},
	}
}
