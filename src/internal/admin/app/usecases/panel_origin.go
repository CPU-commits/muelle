package usecases

import (
	"context"
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/internal/admin/domain/repository"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// PanelOriginTTL es cuánto vale el origen del panel en caché. PATCH /settings
// y el arranque lo invalidan, así que el TTL solo cubre un cambio hecho fuera
// de muelle (el UPDATE a mano de los puertos, contrato §4.1).
const PanelOriginTTL = 30 * time.Second

// GetPanelOrigin da el origen en que muelle-caddy sirve el panel según los
// ajustes (shared.ProxySettings.PanelOrigin): https://<panel_domain>, con el
// puerto HTTPS del proxy si no es 443. cmd/http lo acepta como Origin, así
// cambiar los puertos del proxy no deja fuera del panel (spec §16).
//
// cmd/http lo pregunta en cada petición que modifica algo: lo guarda en
// memoria hasta PanelOriginTTL o hasta Invalidate, que llaman PatchSettings
// (cuando cambia algún ajuste del proxy) y admin.Module al arrancar.
type GetPanelOrigin struct {
	settings repository.SettingsRepository
	clock    clock.Clock

	mu      sync.Mutex
	gen     uint64 // sube con cada Invalidate
	cached  bool
	origin  string
	expires time.Time
}

func NewGetPanelOrigin(settings repository.SettingsRepository, clk clock.Clock) *GetPanelOrigin {
	if clk == nil {
		clk = clock.System{}
	}
	return &GetPanelOrigin{settings: settings, clock: clk}
}

// Execute da el origen en caché o, si no hay, lo lee: solo panel_domain y
// proxy_https_port (no descifra el token). "" sin dominio del panel; un
// valor guardado que no se entiende es un error, que no queda en caché.
func (u *GetPanelOrigin) Execute(ctx context.Context) (string, error) {
	now := u.clock.Now()
	u.mu.Lock()
	if u.cached && now.Before(u.expires) {
		o := u.origin
		u.mu.Unlock()
		return o, nil
	}
	gen := u.gen
	u.mu.Unlock()

	o, err := u.load(ctx)
	if err != nil {
		return "", err
	}
	u.mu.Lock()
	// Si hubo un Invalidate mientras leía, lo leído puede ser de antes del
	// cambio: vale para esta petición, pero no se guarda.
	if u.gen == gen {
		u.cached, u.origin, u.expires = true, o, now.Add(PanelOriginTTL)
	}
	u.mu.Unlock()
	return o, nil
}

// Invalidate descarta el origen en caché: la próxima petición lo lee de la base.
func (u *GetPanelOrigin) Invalidate() {
	u.mu.Lock()
	u.gen++
	u.cached = false
	u.mu.Unlock()
}

func (u *GetPanelOrigin) load(ctx context.Context) (string, error) {
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
