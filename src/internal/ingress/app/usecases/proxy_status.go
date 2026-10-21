package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/app/dto"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
)

// GetProxyStatus es GET /api/v1/proxy (contrato §3.3): el estado en memoria
// del proxy y lo que corre de verdad.
type GetProxyStatus struct {
	settings service.Settings
	runtime  service.CaddyRuntime
	state    *ProxyState
	defImage string
}

// NewGetProxyStatus recibe la imagen de Caddy de esta versión
// (naming.CaddyImageFor), que decide si el proxy está habilitado.
func NewGetProxyStatus(settings service.Settings, runtime service.CaddyRuntime, state *ProxyState, defImage string) *GetProxyStatus {
	return &GetProxyStatus{settings: settings, runtime: runtime, state: state, defImage: defImage}
}

// Execute lee los ajustes y, con el proxy habilitado, inspecciona
// muelle-caddy: con Docker caído devuelve su error (503). Deshabilitado
// (desarrollo) no consulta Docker. ports_rollback es la última vuelta a los
// puertos anteriores que registró el estado.
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
	return out, nil
}
