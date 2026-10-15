// Package cron arma las tareas periódicas del módulo ingress.
package cron

import (
	"context"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/cron"
)

const (
	// ProxyEvery es la frecuencia del job "proxy" (contrato §1.2); cron.Run
	// lo corre también al arrancar.
	ProxyEvery = 5 * time.Minute
	// CertsEvery es la frecuencia del job "certificados" (§8 del spec).
	CertsEvery = time.Hour
)

// Jobs son las tareas de ingress: "proxy" asegura muelle-caddy y las redes de
// los stages, borra las redes huérfanas y reenvía la configuración;
// "certificados" verifica los certificados de los dominios pending, active y
// error + certificate (contrato §3.1).
func Jobs(proxy, certs func(ctx context.Context) error) []cron.Job {
	return []cron.Job{
		{Name: "proxy", Every: ProxyEvery, Run: proxy},
		{Name: "certificados", Every: CertsEvery, Run: certs},
	}
}
