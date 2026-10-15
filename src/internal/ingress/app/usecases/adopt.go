package usecases

import (
	"context"
	"fmt"

	"github.com/CPU-commits/muelle/src/internal/ingress/app/dto"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// AdoptDomain es POST {app}/domains/{hostname}/adopt.
type AdoptDomain struct{ d DomainDeps }

func NewAdoptDomain(d DomainDeps) *AdoptDomain { return &AdoptDomain{d: d} }

// Execute, con el lock de Ops: solo un dominio en error + record_exists se
// adopta (si no, service.ErrNothingToAdopt: 409 nothing_to_adopt). Pasa a
// manejar el registro ajeno (dnsOps.adopt) y guarda; sin token o sin
// public_ip, o si Cloudflare falla, el dominio queda en error + cloudflare y
// no es un error de la petición. Después, fuera del lock, manda la
// configuración a Caddy, agenda la verificación de los 30 s y audita.
func (uc *AdoptDomain) Execute(ctx context.Context, ref dto.DomainRef) (dto.DomainOutput, error) {
	app, err := uc.d.app(ctx, ref.AppRef)
	if err != nil {
		return dto.DomainOutput{}, err
	}
	var dom entity.Domain
	var p shared.ProxySettings
	err = uc.d.Ops.Do(ctx, uc.d.DomainOp, func(opCtx, dbCtx context.Context) error {
		var err error
		if dom, err = uc.d.domain(dbCtx, app, ref.Hostname); err != nil {
			return err
		}
		if dom.Status != entity.StatusError || dom.ErrorCode != entity.CodeRecordExists {
			return service.ErrNothingToAdopt
		}
		if p, err = uc.d.settings(dbCtx); err != nil {
			return err
		}
		uc.d.dnsOps().adopt(opCtx, &dom, p)
		if err := uc.d.Repo.Update(dbCtx, dom); err != nil {
			return fmt.Errorf("ingress: guardando el dominio %s: %w", dom.Hostname, err)
		}
		return nil
	})
	if err != nil {
		return dto.DomainOutput{}, err
	}
	uc.d.logResult("domain.adopt", dom)
	uc.d.sync(ctx, false)
	uc.d.Certs.ScheduleFirst(dom.Hostname)
	uc.d.audit(ctx, "domain.adopt", app, dom.Hostname, nil)
	return toOutput(dom, p), nil
}
