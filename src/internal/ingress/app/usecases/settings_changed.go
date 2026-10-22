package usecases

import (
	"context"
	"log/slog"
	"slices"
	"time"

	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// ProxySettingsChanged aplica los ajustes del proxy que admin acaba de
// guardar (PATCH /settings, contrato §3.5), antes de que el PATCH responda.
type ProxySettingsChanged struct {
	recreator *Recreator
	records   *UpdateManagedRecords
	panel     *EnsurePanelDNS
	syncer    *Syncer
	origin    *EnsureOriginCA
	state     *ProxyState
	timeout   time.Duration
	log       *slog.Logger
}

// NewProxySettingsChanged recibe el reconciliador de Origin CA y el plazo
// del hook (Timing.Hook, 2 min).
func NewProxySettingsChanged(recreator *Recreator, records *UpdateManagedRecords, panel *EnsurePanelDNS, syncer *Syncer,
	origin *EnsureOriginCA, state *ProxyState, timeout time.Duration, log *slog.Logger) *ProxySettingsChanged {
	return &ProxySettingsChanged{recreator: recreator, records: records, panel: panel, syncer: syncer, origin: origin, state: state,
		timeout: timeout, log: log}
}

// Execute aplica lo que cambió (next.ChangedKeys(old)), con su propio plazo
// derivado con context.WithoutCancel (un cliente que se va no lo deja a
// medias), en este orden:
//  1. puertos → release := recreator.Hold() y defer release(): GET
//     /proxy dice recreating desde ya, y los 2 s de RecreateDelay empiezan
//     recién cuando Execute vuelve (D15). No recrea acá: la respuesta del
//     PATCH sale por ese mismo contenedor. Además olvida la última vuelta
//     atrás (ports_rollback): es un intento nuevo;
//  2. public_ip o public_ipv6 → UpdateManagedRecords (cada dominio con
//     DomainOp, dentro del plazo del hook: D6);
//  3. public_ip, public_ipv6 o un token nuevo (no vacío) → EnsurePanelDNS
//     (lee los ajustes actuales: D30);
//  4. si no hubo 1 y cambió public_ip, public_ipv6, acme_email, el token,
//     panel_allowed_cidrs o cloudflare_origin_ca → syncer.Sync. Con 1, la
//     configuración la manda la recreación, con los ajustes de ese momento.
//     Al apagar cloudflare_origin_ca, así, los dominios vuelven a Let's
//     Encrypt antes de que responda el PATCH;
//  5. apagar cloudflare_origin_ca → origin.Forget (lo observado y las
//     fallas; los retirados se revocan igual);
//  6. cloudflare_origin_ca, o el token con la opción encendida →
//     origin.ResetBackoff de todos y origin.Kick, sin esperar la pasada
//     (contrato §3.5 y §3.7).
//
// Nunca devuelve error: los errores quedan en el log, en el estado del proxy
// (GET /proxy) y en el de cada dominio.
func (u *ProxySettingsChanged) Execute(ctx context.Context, old, next shared.ProxySettings) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), u.timeout)
	defer cancel()
	keys := next.ChangedKeys(old)
	changed := func(ks ...string) bool {
		return slices.ContainsFunc(ks, func(k string) bool { return slices.Contains(keys, k) })
	}
	u.log.Info("ingress: aplicando los ajustes del proxy", "claves", keys)

	recreate := changed(shared.SettingProxyHTTPPort, shared.SettingProxyHTTPSPort)
	if recreate {
		u.state.ClearPortsRollback()
		release := u.recreator.Hold()
		defer release()
	}
	ip := changed(shared.SettingPublicIP, shared.SettingPublicIPv6)
	if ip {
		if err := u.records.Execute(ctx, old, next); err != nil {
			u.log.Error("ingress: actualizando los registros DNS de los dominios", "error", err)
		}
	}
	if ip || (changed(shared.SettingCloudflareToken) && next.CloudflareToken != "") {
		if _, err := u.panel.Execute(ctx); err != nil {
			u.log.Warn("ingress: el registro DNS del panel", "error", err)
		}
	}
	if !recreate && (ip || changed(shared.SettingACMEEmail, shared.SettingCloudflareToken, shared.SettingPanelAllowedCIDRs,
		shared.SettingCloudflareOriginCA)) {
		_ = u.syncer.Sync(ctx, false) // el error ya quedó en el estado y en el log, y lo reintenta el Retrier
	}
	if changed(shared.SettingCloudflareOriginCA) && !next.CloudflareOriginCA {
		if err := u.origin.Forget(ctx); err != nil {
			u.log.Error("ingress: olvidando lo que se vio en Cloudflare", "error", err)
		}
	}
	if changed(shared.SettingCloudflareOriginCA) || (changed(shared.SettingCloudflareToken) && next.CloudflareOriginCA) {
		if err := u.origin.ResetBackoff(ctx, ""); err != nil {
			u.log.Error("ingress: reiniciando los reintentos de los certificados de Cloudflare", "error", err)
		}
		u.origin.Kick()
	}
}
