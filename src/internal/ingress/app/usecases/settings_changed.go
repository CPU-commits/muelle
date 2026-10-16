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
	state     *ProxyState
	timeout   time.Duration
	log       *slog.Logger
}

// NewProxySettingsChanged recibe el plazo del hook (Timing.Hook, 2 min).
func NewProxySettingsChanged(recreator *Recreator, records *UpdateManagedRecords, panel *EnsurePanelDNS, syncer *Syncer,
	state *ProxyState, timeout time.Duration, log *slog.Logger) *ProxySettingsChanged {
	return &ProxySettingsChanged{recreator: recreator, records: records, panel: panel, syncer: syncer, state: state,
		timeout: timeout, log: log}
}

// Execute aplica lo que cambió (next.ChangedKeys(old)), con su propio plazo
// derivado con context.WithoutCancel (un cliente que se va no lo deja a
// medias), en este orden:
//  1. puertos o imagen → release := recreator.Hold() y defer release(): GET
//     /proxy dice recreating desde ya, y los 2 s de RecreateDelay empiezan
//     recién cuando Execute vuelve (D15). No recrea acá: la respuesta del
//     PATCH sale por ese mismo contenedor. Con otros puertos, además olvida
//     la última vuelta atrás (ports_rollback): es un intento nuevo;
//  2. public_ip o public_ipv6 → UpdateManagedRecords (cada dominio con
//     DomainOp, dentro del plazo del hook: D6);
//  3. public_ip, public_ipv6 o un token nuevo (no vacío) → EnsurePanelDNS
//     (lee los ajustes actuales: D30);
//  4. si no hubo 1 y cambió public_ip, public_ipv6, acme_email, el token o
//     panel_allowed_cidrs → syncer.Sync. Con 1, la configuración la manda la
//     recreación, con los ajustes de ese momento.
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

	recreate := changed(shared.SettingProxyHTTPPort, shared.SettingProxyHTTPSPort, shared.SettingCaddyImage)
	if recreate {
		if changed(shared.SettingProxyHTTPPort, shared.SettingProxyHTTPSPort) {
			u.state.ClearPortsRollback()
		}
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
	if !recreate && (ip || changed(shared.SettingACMEEmail, shared.SettingCloudflareToken, shared.SettingPanelAllowedCIDRs)) {
		_ = u.syncer.Sync(ctx, false) // el error ya quedó en el estado y en el log, y lo reintenta el Retrier
	}
}
