package ingress

import (
	"cmp"
	"context"
	"crypto/x509"
	"net"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
)

// Options cambian cómo ingress llega a Docker, Caddy y Cloudflare. El cero es
// producción (D3 del plan de la Fase 3); solo los tests las cambian: los de
// integración no pueden usar los nombres fijos, el puerto 443 ni la API real
// de Cloudflare.
type Options struct {
	// CaddyContainer es el nombre de muelle-caddy, el que crea ingress y el
	// que conectan las redes de stage ("": naming.CaddyContainer).
	CaddyContainer string
	// PanelNetwork es la red de muelle y Caddy ("": naming.PanelNetwork).
	PanelNetwork string
	// Los volúmenes de muelle-caddy ("": naming.Caddy*Volume).
	CaddyDataVolume, CaddyConfigVolume, CaddyAdminVolume string
	// CaddyAdminDial abre la conexión con la admin API (nil: el socket unix
	// naming.CaddyAdminSocket).
	CaddyAdminDial func(ctx context.Context) (net.Conn, error)
	// CaddyTLSAddr es donde se verifican los certificados ("":
	// <CaddyContainer>:443, por muelle_panel).
	CaddyTLSAddr string
	// TLSRoots verifican los certificados (nil: las raíces del sistema).
	TLSRoots *x509.CertPool
	// InternalCA usa la CA interna de Caddy (tls internal) en vez de ACME:
	// solo los tests de integración.
	InternalCA bool
	// NetworkPrefix: el job "proxy" solo borra redes de stage huérfanas con
	// este prefijo ("": todas).
	NetworkPrefix string
	// CloudflareURL es la base de la API ("": cloudflare.DefaultBaseURL).
	// Los tests siempre pasan la de un cftest.
	CloudflareURL string
	Timing        Timing
}

// Timing son los plazos de ingress (contrato §3; un campo en cero toma su
// valor por defecto).
type Timing struct {
	RetryMin, RetryMax time.Duration // 1 s y 60 s: los reintentos de /load
	RecreateDelay      time.Duration // 2 s: la recreación después del PATCH /settings
	FirstCheck         time.Duration // 30 s: la verificación después de crear, adoptar o reintentar
	PendingEvery       time.Duration // 1 min: el barrido de los pending
	DomainOp           time.Duration // 30 s: crear, editar, borrar, adoptar, EnsurePanelDNS y cada registro del hook (D6)
	DomainRetry        time.Duration // 20 s: retry
	Hook               time.Duration // 2 min: ProxySettingsChanged
	OriginPass         time.Duration // 5 min: una pasada de los certificados de Cloudflare (Origin CA)
	OriginRevokeGrace  time.Duration // 1 min: del retiro de un certificado de Origin CA a su revocación
}

// withDefaults completa los campos vacíos con los de producción.
func (o Options) withDefaults() Options {
	o.CaddyContainer = cmp.Or(o.CaddyContainer, naming.CaddyContainer)
	o.PanelNetwork = cmp.Or(o.PanelNetwork, naming.PanelNetwork)
	o.CaddyDataVolume = cmp.Or(o.CaddyDataVolume, naming.CaddyDataVolume)
	o.CaddyConfigVolume = cmp.Or(o.CaddyConfigVolume, naming.CaddyConfigVolume)
	o.CaddyAdminVolume = cmp.Or(o.CaddyAdminVolume, naming.CaddyAdminVolume)
	o.CaddyTLSAddr = cmp.Or(o.CaddyTLSAddr, net.JoinHostPort(o.CaddyContainer, "443"))
	o.CloudflareURL = cmp.Or(o.CloudflareURL, cloudflare.DefaultBaseURL)
	t := &o.Timing
	t.RetryMin = cmp.Or(t.RetryMin, time.Second)
	t.RetryMax = cmp.Or(t.RetryMax, 60*time.Second)
	t.RecreateDelay = cmp.Or(t.RecreateDelay, 2*time.Second)
	t.FirstCheck = cmp.Or(t.FirstCheck, 30*time.Second)
	t.PendingEvery = cmp.Or(t.PendingEvery, time.Minute)
	t.DomainOp = cmp.Or(t.DomainOp, 30*time.Second)
	t.DomainRetry = cmp.Or(t.DomainRetry, 20*time.Second)
	t.Hook = cmp.Or(t.Hook, 2*time.Minute)
	t.OriginPass = cmp.Or(t.OriginPass, 5*time.Minute)
	t.OriginRevokeGrace = cmp.Or(t.OriginRevokeGrace, time.Minute)
	return o
}
