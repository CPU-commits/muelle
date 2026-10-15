// Package cron arma las tareas periódicas del módulo ingress.
package cron

import (
	"context"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/cron"
)

// ProxyEvery es la frecuencia del job "proxy" (contrato §1.2); cron.Run lo
// corre también al arrancar.
const ProxyEvery = 5 * time.Minute

// Jobs son las tareas de ingress: "proxy" asegura muelle-caddy y las redes de
// los stages, borra las redes huérfanas y reenvía la configuración.
// "certificados" (cada hora, §8) llega con la verificación de los certificados.
func Jobs(proxy func(ctx context.Context) error) []cron.Job {
	return []cron.Job{{Name: "proxy", Every: ProxyEvery, Run: proxy}}
}
