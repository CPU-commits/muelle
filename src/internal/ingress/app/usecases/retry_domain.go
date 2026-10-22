package usecases

import (
	"context"
	"fmt"

	"github.com/CPU-commits/muelle/src/internal/ingress/app/dto"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// RetryDomain es POST {app}/domains/{hostname}/retry: sincrónico, con a lo
// más 20 s (Timing.DomainRetry).
type RetryDomain struct{ d DomainDeps }

func NewRetryDomain(d DomainDeps) *RetryDomain { return &RetryDomain{d: d} }

// Execute decide con el lock de Ops, en el orden de D23 del plan de la
// Fase 3 (contrato §3.2):
//  1. error con cloudflare o record_exists: rehace el DNS (ensure) y guarda;
//  2. si no, sin zona y sin registros propios: busca la zona (findZone) y,
//     si la encuentra, crea los registros y guarda;
//  3. si no: verifica el certificado ahora (checkLocked, con retry). Un
//     certificate que vuelve a pending pide reenviar la configuración
//     forzando la recarga, así Caddy reintenta la emisión.
//
// Después, fuera del lock: reinicia los reintentos del certificado de Origin
// CA de ese dominio (contrato §3.2; la falla queda hasta la pasada), manda la
// configuración a Caddy (forzada en el caso 3), despierta al reconciliador
// de Origin CA, agenda la verificación de los 30 s después de 1 y 2, y
// audita. Responde con el estado resultante.
func (uc *RetryDomain) Execute(ctx context.Context, ref dto.DomainRef) (dto.DomainOutput, error) {
	app, err := uc.d.app(ctx, ref.AppRef)
	if err != nil {
		return dto.DomainOutput{}, err
	}
	var dom entity.Domain
	var p shared.ProxySettings
	var redone, repush bool
	err = uc.d.Ops.Do(ctx, uc.d.DomainRetry, func(opCtx, dbCtx context.Context) error {
		var err error
		if dom, err = uc.d.domain(dbCtx, app, ref.Hostname); err != nil {
			return err
		}
		if p, err = uc.d.settings(dbCtx); err != nil {
			return err
		}
		ops := uc.d.dnsOps()
		switch {
		case dom.Status == entity.StatusError && (dom.ErrorCode == entity.CodeCloudflare || dom.ErrorCode == entity.CodeRecordExists):
			ops.ensure(opCtx, &dom, p)
			redone = true
		case dom.CFZoneID == "" && !dom.Managed():
			redone = ops.findZone(opCtx, &dom, p)
		}
		if redone {
			if err := uc.d.Repo.Update(dbCtx, dom); err != nil {
				return fmt.Errorf("ingress: guardando el dominio %s: %w", dom.Hostname, err)
			}
			return nil
		}
		repush, err = uc.d.Certs.checkLocked(opCtx, dbCtx, &dom)
		return err
	})
	if err != nil {
		return dto.DomainOutput{}, err
	}
	uc.d.logResult("domain.retry", dom)
	if err := uc.d.Origin.ResetBackoff(context.WithoutCancel(ctx), dom.Hostname); err != nil {
		uc.d.Log.Warn("ingress: reiniciando los reintentos del certificado de Cloudflare", "hostname", dom.Hostname, "error", err)
	}
	uc.d.syncAndKick(ctx, repush)
	if redone {
		uc.d.Certs.ScheduleFirst(dom.Hostname)
	}
	uc.d.audit(ctx, "domain.retry", app, dom.Hostname, nil)
	return uc.d.output(ctx, dom, p)
}
