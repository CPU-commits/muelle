package service

import (
	"cmp"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
)

// Route es la ruta de un dominio: hostname → upstream por la red del stage.
type Route struct {
	Hostname, Upstream string
	TLS                entity.TLSMode
}

// PanelRoute es la ruta del panel (contrato §3.4).
type PanelRoute struct {
	Hostname, Upstream string
	AllowedCIDRs       []string // [] = cualquiera; nunca nil
	TLS                entity.TLSMode
}

// ProxyConfig es la configuración completa de Caddy como datos (D1 del plan
// de la Fase 3): qué host va a qué upstream, con qué TLS, y las IPs del
// panel. El JSON de Caddy lo arma el adaptador (caddy.Render). El token va
// en claro: String, GoString, LogValue y MarshalJSON lo ocultan, como en
// shared.ProxySettings.
type ProxyConfig struct {
	Panel           *PanelRoute // nil sin panel_domain
	Routes          []Route     // por hostname; nunca nil
	ACMEEmail       string
	CloudflareToken string
	InternalCA      bool // solo integración: la CA interna de Caddy en vez de ACME
}

// BuildProxyConfig arma la configuración desde los ajustes, la zona del panel
// (panel_cf_zone_id, "" si no cae en una zona visible), los dominios y las
// apps (por id). Es pura (contrato §3.4 y §8 del spec):
//   - el panel va a muelle_muelle:8080, con cloudflare si hay token y zona;
//   - cada dominio va a <proyecto>_<stage>_<app>_<servicio>:<puerto>, en
//     cualquier estado (DNS-01 puede emitir igual);
//   - sin token, todo va por http, aunque el dominio diga cloudflare;
//   - se saltan el dominio cuyo hostname es el del panel (la ruta del panel
//     gana) y el que no tiene su app en apps.
func BuildProxyConfig(p shared.ProxySettings, panelZoneID string, domains []entity.Domain, apps map[int64]App, internalCA bool) ProxyConfig {
	cfg := ProxyConfig{
		Routes:          []Route{},
		ACMEEmail:       p.ACMEEmail,
		CloudflareToken: p.CloudflareToken,
		InternalCA:      internalCA,
	}
	hasToken := p.CloudflareToken != ""
	if p.PanelDomain != "" {
		tls := entity.TLSHTTP
		if hasToken && panelZoneID != "" {
			tls = entity.TLSCloudflare
		}
		cidrs := slices.Clone(p.PanelAllowedCIDRs)
		if cidrs == nil {
			cidrs = []string{}
		}
		cfg.Panel = &PanelRoute{
			Hostname:     p.PanelDomain,
			Upstream:     naming.Upstream(naming.MuelleService, naming.MuellePort),
			AllowedCIDRs: cidrs,
			TLS:          tls,
		}
	}
	for _, d := range domains {
		if p.PanelDomain != "" && strings.EqualFold(d.Hostname, p.PanelDomain) {
			continue
		}
		app, ok := apps[d.AppID]
		if !ok {
			continue
		}
		tls := d.TLSMode
		if !hasToken || tls != entity.TLSCloudflare {
			tls = entity.TLSHTTP
		}
		cfg.Routes = append(cfg.Routes, Route{
			Hostname: d.Hostname,
			Upstream: naming.Upstream(naming.Service(app.Project, app.Stage, app.Slug, d.TargetService), d.TargetPort),
			TLS:      tls,
		})
	}
	slices.SortStableFunc(cfg.Routes, func(a, b Route) int { return cmp.Compare(a.Hostname, b.Hostname) })
	return cfg
}

// redacted es c con el token como "***" (si hay).
func (c ProxyConfig) redacted() ProxyConfig {
	if c.CloudflareToken != "" {
		c.CloudflareToken = "***"
	}
	return c
}

// plainProxyConfig es ProxyConfig sin métodos: MarshalJSON lo usa para no
// volver a llamarse.
type plainProxyConfig ProxyConfig

// String oculta el token. Muestra el panel por valor (un %+v de un puntero
// solo daría su dirección).
func (c ProxyConfig) String() string {
	r := c.redacted()
	panel := "<nil>"
	if r.Panel != nil {
		panel = fmt.Sprintf("%+v", *r.Panel)
	}
	return fmt.Sprintf("{Panel:%s Routes:%+v ACMEEmail:%s CloudflareToken:%s InternalCA:%t}",
		panel, r.Routes, r.ACMEEmail, r.CloudflareToken, r.InternalCA)
}

// GoString es String para %#v.
func (c ProxyConfig) GoString() string { return c.String() }

// LogValue es String para slog: el JSONHandler serializaría el struct.
func (c ProxyConfig) LogValue() slog.Value { return slog.StringValue(c.String()) }

// MarshalJSON es el struct con el token como "***", para un ProxyConfig
// dentro de otro valor que se serializa. Lo que se manda a Caddy no sale de
// acá: lo arma caddy.Render.
func (c ProxyConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(plainProxyConfig(c.redacted()))
}
