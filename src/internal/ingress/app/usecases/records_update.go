package usecases

import (
	"context"
	"errors"
	"fmt"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// UpdateManagedRecords lleva los registros DNS que maneja muelle a la IP
// pública cuando cambia (contrato §3.5). Lo llama ProxySettingsChanged.
type UpdateManagedRecords struct{ d DomainDeps }

func NewUpdateManagedRecords(d DomainDeps) *UpdateManagedRecords { return &UpdateManagedRecords{d: d} }

// Execute: old y next solo deciden si hay algo que hacer: un public_ip nuevo
// (borrarlo no toca ningún registro) o un cambio de public_ipv6. Entonces,
// para cada dominio con su A propio, en su propio Ops.Do (DomainOp, dentro
// del plazo de ctx: D6), vuelve a leer el dominio (uno que ya no está, o que
// ya no tiene registros propios, se salta) y los ajustes guardados en ese
// momento (D30), y le aplica records. Los errores de Cloudflare quedan en
// cada dominio (error + cloudflare) y en el log; devuelve solo los de la base.
func (u *UpdateManagedRecords) Execute(ctx context.Context, old, next shared.ProxySettings) error {
	ip4 := next.PublicIP != old.PublicIP && next.PublicIP != ""
	ip6 := next.PublicIPv6 != old.PublicIPv6
	if !ip4 && !ip6 {
		return nil
	}
	all, err := u.d.Repo.List(context.WithoutCancel(ctx))
	if err != nil {
		return fmt.Errorf("ingress: leyendo los dominios: %w", err)
	}
	var errs []error
	for _, listed := range all {
		if !listed.Managed() {
			continue
		}
		err := u.d.Ops.Do(ctx, u.d.DomainOp, func(opCtx, dbCtx context.Context) error {
			dom, err := u.d.Repo.Get(dbCtx, listed.Hostname)
			if errors.Is(err, sharedrepo.ErrNotFound) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("ingress: el dominio %s: %w", listed.Hostname, err)
			}
			if !dom.Managed() {
				return nil
			}
			p, err := u.d.settings(dbCtx)
			if err != nil {
				return err
			}
			u.records(opCtx, &dom, p)
			if err := u.d.Repo.Update(dbCtx, dom); err != nil {
				return fmt.Errorf("ingress: guardando el dominio %s: %w", dom.Hostname, err)
			}
			u.d.logResult("public_ip", dom)
			return nil
		})
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// records lleva los registros propios de d a los ajustes p, solo el
// contenido (nunca el proxied del A, que puede haber cambiado el usuario):
//   - sin token: error + cloudflare con el mensaje del token (D18), sin
//     llamar a Cloudflare;
//   - el A, a public_ip si hay; si ya no está, D29 (se borra el AAAA propio
//     y se olvidan los dos) y error + cloudflare, así retry lo crea;
//   - el AAAA, a public_ipv6 (syncAAAA: lo crea si falta y no hay uno
//     ajeno, lo borra sin IPv6).
//
// Si todo sale bien con public_ip, un dominio en error + cloudflare vuelve a
// pending. Uno que no estaba en error no cambia de estado ni de updated_at.
func (u *UpdateManagedRecords) records(ctx context.Context, d *entity.Domain, p shared.ProxySettings) {
	now := u.d.Clock.Now()
	ops := u.d.dnsOps()
	if p.CloudflareToken == "" {
		d.Fail(entity.CodeCloudflare, msgNoToken, now)
		return
	}
	if p.PublicIP != "" {
		ip := p.PublicIP
		_, err := u.d.DNS.UpdateRecord(ctx, p.CloudflareToken, d.CFZoneID, d.CFRecordID, service.RecordChange{Content: &ip})
		if errors.Is(err, service.ErrDNSNotFound) {
			kept := ops.dropManaged(ctx, d, p)
			d.Fail(entity.CodeCloudflare, msgRecordGone(typeA, d.Hostname, kept), now)
			return
		}
		if err != nil {
			d.Fail(entity.CodeCloudflare, cloudflareMsg(err), now)
			return
		}
	}
	if !ops.syncAAAA(ctx, d, p, now) {
		return
	}
	if p.PublicIP != "" && d.Status == entity.StatusError && d.ErrorCode == entity.CodeCloudflare {
		d.MarkPending(now)
	}
}
