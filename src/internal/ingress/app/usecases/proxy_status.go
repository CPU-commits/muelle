package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/app/dto"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// GetProxyStatus es GET /api/v1/proxy (contrato §3.3): el estado en memoria
// del proxy y lo que corre de verdad.
type GetProxyStatus struct {
	settings service.Settings
	runtime  service.CaddyRuntime
	state    *ProxyState
	origin   *EnsureOriginCA
	clock    clock.Clock
	defImage string
}

// NewGetProxyStatus recibe el reconciliador de Origin CA (de donde lee el
// certificado del panel), el reloj y la imagen de Caddy de esta versión
// (naming.CaddyImageFor), que decide si el proxy está habilitado.
func NewGetProxyStatus(settings service.Settings, runtime service.CaddyRuntime, state *ProxyState, origin *EnsureOriginCA,
	clk clock.Clock, defImage string) *GetProxyStatus {
	return &GetProxyStatus{settings: settings, runtime: runtime, state: state, origin: origin, clock: clk, defImage: defImage}
}

// Execute lee los ajustes y, con el proxy habilitado, inspecciona
// muelle-caddy: con Docker caído devuelve su error (503). Deshabilitado
// (desarrollo) no consulta Docker. ports_rollback es la última vuelta a los
// puertos anteriores que registró el estado, y panel_certificate el
// certificado del panel (null sin panel_domain; origin_ca null con la opción
// apagada, sin token o con el proxy deshabilitado).
func (u *GetProxyStatus) Execute(ctx context.Context) (dto.ProxyOutput, error) {
	p, err := u.settings.ProxySettings(ctx)
	if err != nil {
		return dto.ProxyOutput{}, fmt.Errorf("ingress: leyendo los ajustes del proxy: %w", err)
	}
	enabled := p.Enabled(u.defImage)
	out := dto.ProxyOutput{Caddy: dto.CaddyOutput{HTTPPort: p.HTTPPort, HTTPSPort: p.HTTPSPort}}
	if enabled {
		image, running, found, err := u.runtime.Status(ctx)
		if err != nil {
			return dto.ProxyOutput{}, err
		}
		if found {
			out.Caddy.Image, out.Caddy.Running = &image, running
		}
	}
	status, msg, syncedAt := u.state.Snapshot(enabled)
	out.Status = string(status)
	if msg != "" {
		out.Error = &msg
	}
	if syncedAt != nil {
		t := syncedAt.UTC().Truncate(time.Millisecond)
		out.SyncedAt = &t
	}
	if r := u.state.LastPortsRollback(); r != nil {
		out.PortsRollback = &dto.PortsRollbackOutput{
			Attempted: dto.ProxyPortsOutput{HTTPPort: r.Attempted.HTTPPort, HTTPSPort: r.Attempted.HTTPSPort},
			Restored:  dto.ProxyPortsOutput{HTTPPort: r.Restored.HTTPPort, HTTPSPort: r.Restored.HTTPSPort},
			Error:     r.Error,
			At:        r.At.UTC().Truncate(time.Millisecond),
		}
	}
	if p.PanelDomain != "" {
		pc, err := u.panelCertificate(ctx, p, enabled)
		if err != nil {
			return dto.ProxyOutput{}, err
		}
		out.PanelCertificate = pc
	}
	return out, nil
}

// panelCertificate es el certificado del panel, con la regla de un dominio
// (service.OriginCAView) y su zona (panel_cf_zone_id).
func (u *GetProxyStatus) panelCertificate(ctx context.Context, p shared.ProxySettings, enabled bool) (*dto.PanelCertificateOutput, error) {
	var v *service.OriginView
	if service.OriginOn(p, enabled) {
		dns, err := u.settings.PanelDNS(ctx)
		if err != nil {
			return nil, fmt.Errorf("ingress: leyendo la zona del panel: %w", err)
		}
		active, h, err := u.origin.SnapshotFor(ctx, p.PanelDomain)
		if err != nil {
			return nil, err
		}
		v = service.OriginCAView(true, service.PanelOriginTarget(p.PanelDomain, dns.ZoneID), active, h, u.clock.Now())
	}
	return &dto.PanelCertificateOutput{Hostname: p.PanelDomain, CertSource: service.CertSource(v), OriginCA: originOutput(v)}, nil
}
