package usecases

import (
	"context"
	"sync"
	"time"
)

// Ops serializa lo que escribe dominios o registros DNS (D6 del plan de la
// Fase 3, contrato §3.2: "van de a una y no dependen del cliente"): crear,
// editar, borrar, adoptar y reintentar un dominio, aplicar una verificación
// de certificado, el registro del panel y cada registro que actualiza el
// hook de la IP pública. El valor cero sirve.
type Ops struct{ mu sync.Mutex }

// Do corre fn con el lock tomado. Antes de esperar el lock arma opCtx, que
// no se cancela con ctx (un cliente que se va no deja la operación a medias)
// y vence limit después de ahora, o antes si ctx vence antes (el plazo del
// hook manda). Así el que esperó llega con menos tiempo, y nadie tiene el
// lock sin un plazo. fn usa opCtx para Cloudflare, Caddy y el TLS, y dbCtx
// (sin cancelación ni plazo) para la base: un plazo vencido en Cloudflare
// igual guarda el error del dominio. Devuelve lo que devuelve fn.
func (o *Ops) Do(ctx context.Context, limit time.Duration, fn func(opCtx, dbCtx context.Context) error) error {
	dbCtx := context.WithoutCancel(ctx)
	deadline := time.Now().Add(limit)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	opCtx, cancel := context.WithDeadline(dbCtx, deadline)
	defer cancel()
	o.mu.Lock()
	defer o.mu.Unlock()
	return fn(opCtx, dbCtx)
}
