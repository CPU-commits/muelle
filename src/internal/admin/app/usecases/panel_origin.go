package usecases

import (
	"context"

	"github.com/CPU-commits/muelle/src/internal/admin/domain/repository"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// GetPanelOrigin da el origen en que muelle-caddy sirve el panel según los
// ajustes (shared.ProxySettings.PanelOrigin): https://<panel_domain>, con el
// puerto HTTPS del proxy si no es 443. cmd/http lo acepta como Origin además
// de MUELLE_PANEL_ORIGIN, así cambiar los puertos del proxy no deja fuera del
// panel (spec §16).
type GetPanelOrigin struct {
	settings repository.SettingsRepository
}

func NewGetPanelOrigin(settings repository.SettingsRepository) *GetPanelOrigin {
	return &GetPanelOrigin{settings: settings}
}

// Execute lee solo panel_domain y proxy_https_port (no descifra el token).
// "" sin dominio del panel; un valor guardado que no se entiende es un error.
func (u *GetPanelOrigin) Execute(ctx context.Context) (string, error) {
	raw := map[string]string{}
	for _, k := range []string{shared.SettingPanelDomain, shared.SettingProxyHTTPSPort} {
		v, _, ok, err := get(ctx, u.settings, k)
		if err != nil {
			return "", err
		}
		if ok {
			raw[k] = string(v)
		}
	}
	p, err := shared.DecodeProxySettings(raw, "")
	if err != nil {
		return "", err
	}
	return p.PanelOrigin(), nil
}
