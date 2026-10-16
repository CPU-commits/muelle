package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/app/dto"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// DomainDeps es lo que comparten los casos de uso de los dominios (contrato
// §3.1 y §3.2).
type DomainDeps struct {
	Repo     repository.Domains
	Catalog  service.Catalog
	Settings service.Settings
	DNS      service.DNS
	Syncer   *Syncer
	Certs    *CertChecker
	Ops      *Ops
	Clock    clock.Clock
	Audit    audit.Recorder
	Log      *slog.Logger
	// DomainOp es el plazo de crear, editar, borrar y adoptar (30 s), y
	// DomainRetry el de retry (20 s), desde antes de esperar el lock (D6).
	DomainOp, DomainRetry time.Duration
}

func (d DomainDeps) dnsOps() dnsOps { return dnsOps{dns: d.DNS, clock: d.Clock} }

// app es la app de ref; repository.ErrNotFound (404) si no existe. Se lee
// sin la cancelación del cliente, como todo lo que sigue (D6): un cliente
// que ya se fue no deja la operación sin hacer.
func (d DomainDeps) app(ctx context.Context, ref dto.AppRef) (service.App, error) {
	a, err := d.Catalog.App(context.WithoutCancel(ctx), ref.Project, ref.Stage, ref.App)
	if err != nil {
		return service.App{}, fmt.Errorf("ingress: la app %s/%s/%s: %w", ref.Project, ref.Stage, ref.App, err)
	}
	return a, nil
}

// domain es el dominio hostname de app; repository.ErrNotFound si no existe
// o es de otra app (contrato §3.2).
func (d DomainDeps) domain(ctx context.Context, app service.App, hostname string) (entity.Domain, error) {
	dom, err := d.Repo.Get(ctx, hostname)
	if err != nil {
		return entity.Domain{}, fmt.Errorf("ingress: el dominio %s: %w", hostname, err)
	}
	if dom.AppID != app.ID {
		return entity.Domain{}, fmt.Errorf("ingress: el dominio %s no es de esa app: %w", hostname, sharedrepo.ErrNotFound)
	}
	return dom, nil
}

func (d DomainDeps) settings(ctx context.Context) (shared.ProxySettings, error) {
	p, err := d.Settings.ProxySettings(context.WithoutCancel(ctx))
	if err != nil {
		return shared.ProxySettings{}, fmt.Errorf("ingress: leyendo los ajustes del proxy: %w", err)
	}
	return p, nil
}

// sync manda la configuración a Caddy fuera del lock y sin el ctx del
// cliente (D6). Su error ya queda en el estado del proxy (GET /proxy) y lo
// reintenta el Retrier: la petición no falla por Caddy (contrato §3.2).
func (d DomainDeps) sync(ctx context.Context, force bool) {
	_ = d.Syncer.Sync(context.WithoutCancel(ctx), force)
}

// audit registra la acción sobre el dominio, con la app en details.
func (d DomainDeps) audit(ctx context.Context, action string, app service.App, hostname string, extra map[string]string) {
	details := map[string]string{"app": app.Project + "/" + app.Stage + "/" + app.Slug}
	for k, v := range extra {
		details[k] = v
	}
	d.Audit.Record(ctx, action, "domain:"+hostname, details)
}

// logResult deja en el log un dominio que quedó en error por el DNS.
func (d DomainDeps) logResult(action string, dom entity.Domain) {
	if dom.Status == entity.StatusError && dom.ErrorCode != entity.CodeCertificate {
		d.Log.Warn("ingress: el DNS del dominio quedó en error", "action", action, "hostname", dom.Hostname,
			"code", string(dom.ErrorCode), "error", dom.LastError)
	}
}

// checkFormTarget: en una app de formulario, el destino es el servicio de
// la app, que se llama como ella (contrato §3.2). En una compose no se
// verifica al guardar: lo verifica el render (§6.3 del spec).
func checkFormTarget(app service.App, target string) error {
	if app.Kind == service.KindService && target != app.Slug {
		return validate.Errorf("target_service", "en una app de formulario tiene que ser %q, el nombre de la app", app.Slug)
	}
	return nil
}

// toOutput arma el dominio del contrato §3.1: "" es null, y records sale de
// los ajustes actuales.
func toOutput(d entity.Domain, p shared.ProxySettings) dto.DomainOutput {
	str := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	out := dto.DomainOutput{
		Hostname: d.Hostname, TargetService: d.TargetService, TargetPort: d.TargetPort, Proxied: d.Proxied,
		TLSMode: string(d.TLSMode), CFZoneID: str(d.CFZoneID), CFRecordID: str(d.CFRecordID), Status: string(d.Status),
		LastErrorCode: str(string(d.ErrorCode)), LastError: str(d.LastError), CertIssuer: str(d.CertIssuer),
		Records:   []dto.RecordOutput{},
		CreatedAt: d.CreatedAt.UTC(), UpdatedAt: d.UpdatedAt.UTC(),
	}
	if d.CertExpiresAt != nil {
		t := d.CertExpiresAt.UTC()
		out.CertExpiresAt = &t
	}
	for _, r := range entity.Records(d.Hostname, p.PublicIP, p.PublicIPv6) {
		out.Records = append(out.Records, dto.RecordOutput{Type: r.Type, Name: r.Name, Content: r.Content})
	}
	return out
}

// ListDomains es GET {app}/domains.
type ListDomains struct{ d DomainDeps }

func NewListDomains(d DomainDeps) *ListDomains { return &ListDomains{d: d} }

// Execute devuelve los dominios de la app, por hostname ([] sin dominios),
// con records según los ajustes actuales.
func (uc *ListDomains) Execute(ctx context.Context, ref dto.AppRef) ([]dto.DomainOutput, error) {
	app, err := uc.d.app(ctx, ref)
	if err != nil {
		return nil, err
	}
	ds, err := uc.d.Repo.ListByApp(ctx, app.ID)
	if err != nil {
		return nil, fmt.Errorf("ingress: leyendo los dominios: %w", err)
	}
	p, err := uc.d.settings(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.DomainOutput, 0, len(ds))
	for _, dom := range ds {
		out = append(out, toOutput(dom, p))
	}
	return out, nil
}

// CreateDomain es POST {app}/domains.
type CreateDomain struct{ d DomainDeps }

func NewCreateDomain(d DomainDeps) *CreateDomain { return &CreateDomain{d: d} }

// Execute crea el dominio (D5): la app (404); la validación (422: el
// hostname, el destino, el dominio del panel y la regla de la app de
// formulario); con el lock de Ops, guarda la fila pending (409 si el
// hostname ya existe, sin tocar Cloudflare), hace el DNS (ensure) y la
// guarda otra vez. Un error de Cloudflare no falla la petición: queda en el
// dominio. Después, fuera del lock, manda la configuración a Caddy, agenda
// la verificación de los 30 s y audita.
func (uc *CreateDomain) Execute(ctx context.Context, in dto.CreateDomainInput) (dto.DomainOutput, error) {
	app, err := uc.d.app(ctx, in.AppRef)
	if err != nil {
		return dto.DomainOutput{}, err
	}
	dom, err := entity.NewDomain(app.ID, in.Hostname, in.TargetService, in.TargetPort, in.Proxied != nil && *in.Proxied, uc.d.Clock.Now())
	if err != nil {
		return dto.DomainOutput{}, err
	}
	p, err := uc.d.settings(ctx)
	if err != nil {
		return dto.DomainOutput{}, err
	}
	if p.PanelDomain != "" && dom.Hostname == p.PanelDomain {
		return dto.DomainOutput{}, validate.Errorf("hostname", "es el dominio del panel")
	}
	if err := checkFormTarget(app, dom.TargetService); err != nil {
		return dto.DomainOutput{}, err
	}
	err = uc.d.Ops.Do(ctx, uc.d.DomainOp, func(opCtx, dbCtx context.Context) error {
		if err := uc.d.Repo.Create(dbCtx, &dom); err != nil {
			return fmt.Errorf("ingress: guardando el dominio %s: %w", dom.Hostname, err)
		}
		if p, err = uc.d.settings(dbCtx); err != nil {
			return err
		}
		uc.d.dnsOps().ensure(opCtx, &dom, p)
		if err := uc.d.Repo.Update(dbCtx, dom); err != nil {
			return fmt.Errorf("ingress: guardando el dominio %s: %w", dom.Hostname, err)
		}
		return nil
	})
	if err != nil {
		return dto.DomainOutput{}, err
	}
	uc.d.logResult("domain.create", dom)
	uc.d.sync(ctx, false)
	uc.d.Certs.ScheduleFirst(dom.Hostname)
	uc.d.audit(ctx, "domain.create", app, dom.Hostname, nil)
	return toOutput(dom, p), nil
}

// UpdateDomain es PATCH {app}/domains/{hostname}.
type UpdateDomain struct{ d DomainDeps }

func NewUpdateDomain(d DomainDeps) *UpdateDomain { return &UpdateDomain{d: d} }

// Execute aplica lo que viene en in, con el lock de Ops y el dominio leído
// dentro: el destino (422 si no es válido, o si en una app de formulario no
// es su servicio) y proxied (422 sin registro propio, D22; si no, cambia los
// registros: setProxied). Lo que no cambia nada (como {}) no guarda, no
// audita y no mueve updated_at. Si algo cambió, guarda, manda la
// configuración a Caddy fuera del lock y audita.
func (uc *UpdateDomain) Execute(ctx context.Context, in dto.UpdateDomainInput) (dto.DomainOutput, error) {
	app, err := uc.d.app(ctx, in.AppRef)
	if err != nil {
		return dto.DomainOutput{}, err
	}
	var dom entity.Domain
	var p shared.ProxySettings
	changed := false
	err = uc.d.Ops.Do(ctx, uc.d.DomainOp, func(opCtx, dbCtx context.Context) error {
		var err error
		if dom, err = uc.d.domain(dbCtx, app, in.Hostname); err != nil {
			return err
		}
		if p, err = uc.d.settings(dbCtx); err != nil {
			return err
		}
		now := uc.d.Clock.Now()
		target := in.TargetService != nil || in.TargetPort != nil
		svc, port := dom.TargetService, dom.TargetPort
		if in.TargetService != nil {
			svc = *in.TargetService
		}
		if in.TargetPort != nil {
			port = *in.TargetPort
		}
		if target {
			if err := entity.ValidateTarget(svc, port); err != nil {
				return err
			}
			if err := checkFormTarget(app, svc); err != nil {
				return err
			}
		}
		if in.Proxied != nil && !dom.Managed() {
			return validate.Errorf("proxied", "muelle no maneja el registro DNS de este dominio")
		}
		if target {
			c, err := dom.SetTarget(svc, port, now)
			if err != nil {
				return err
			}
			changed = c
		}
		if in.Proxied != nil && *in.Proxied != dom.Proxied {
			uc.d.dnsOps().setProxied(opCtx, &dom, p, *in.Proxied)
			changed = true
		}
		if !changed {
			return nil
		}
		dom.Touch(now)
		if err := uc.d.Repo.Update(dbCtx, dom); err != nil {
			return fmt.Errorf("ingress: guardando el dominio %s: %w", dom.Hostname, err)
		}
		return nil
	})
	if err != nil {
		return dto.DomainOutput{}, err
	}
	if changed {
		uc.d.logResult("domain.update", dom)
		uc.d.sync(ctx, false)
		uc.d.audit(ctx, "domain.update", app, dom.Hostname, nil)
	}
	return toOutput(dom, p), nil
}

// DeleteDomain es DELETE {app}/domains/{hostname}.
type DeleteDomain struct{ d DomainDeps }

func NewDeleteDomain(d DomainDeps) *DeleteDomain { return &DeleteDomain{d: d} }

// Execute, con el lock de Ops, borra primero los registros propios en
// Cloudflare (remove) y después la fila. Si Cloudflare no responde o
// rechaza el borrado, devuelve ese error (envuelve
// entity.ErrCloudflareUnavailable: 502) y no borra nada en muelle. Sin
// token, borra solo en muelle, y la auditoría lo dice con "dns": "kept".
func (uc *DeleteDomain) Execute(ctx context.Context, ref dto.DomainRef) error {
	app, err := uc.d.app(ctx, ref.AppRef)
	if err != nil {
		return err
	}
	var kept bool
	err = uc.d.Ops.Do(ctx, uc.d.DomainOp, func(opCtx, dbCtx context.Context) error {
		dom, err := uc.d.domain(dbCtx, app, ref.Hostname)
		if err != nil {
			return err
		}
		p, err := uc.d.settings(dbCtx)
		if err != nil {
			return err
		}
		if kept, err = uc.d.dnsOps().remove(opCtx, dom, p); err != nil {
			return err
		}
		if err := uc.d.Repo.Delete(dbCtx, dom.ID); err != nil {
			return fmt.Errorf("ingress: borrando el dominio %s: %w", dom.Hostname, err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	uc.d.sync(ctx, false)
	var extra map[string]string
	if kept {
		extra = map[string]string{"dns": "kept"}
	}
	uc.d.audit(ctx, "domain.delete", app, ref.Hostname, extra)
	return nil
}
