package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// Las reglas de los certificados de Cloudflare Origin CA (contrato §3.7,
// diseño de Origin CA §4, §5, §9 y §10). Son puras: las usan, con la misma
// regla, BuildProxyConfig, la salida de la API, la verificación y el
// reconciliador, así la API nunca dice algo distinto de lo que se le manda a
// Caddy.

const (
	// OriginCAValidityDays es la validez que se pide (requested_validity):
	// el mismo ciclo que Let's Encrypt (§5.2 del diseño).
	OriginCAValidityDays = 90
	// OriginCARenewBefore: el active se renueva cuando faltan menos de 30
	// días. Deja de servirse cuando faltan 14 (RenewMargin).
	OriginCARenewBefore = 30 * 24 * time.Hour
	// originMessageMax es el largo máximo, en runas, del mensaje de
	// Cloudflare que cita un mensaje de muelle.
	originMessageMax = 300
)

// Las causas de una falla, en español (§9.3 del diseño).
const (
	msgPermissionCause = "el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona de %s (Cloudflare: %s); "
	msgRetryDomain     = "agrégalo al token y pulsa Reintentar"
	msgRetryPanel      = "agrégalo al token: muelle lo reintenta solo cada 6 h"
)

// OriginPEM es el certificado de Origin CA que se le carga a Caddy para un
// host (apps.tls.certificates.load_pem). KeyPEM va en claro: String,
// GoString, LogValue y MarshalJSON la ocultan.
type OriginPEM struct{ CertPEM, KeyPEM string }

func (p OriginPEM) redacted() OriginPEM {
	if p.KeyPEM != "" {
		p.KeyPEM = "***"
	}
	return p
}

type plainOriginPEM OriginPEM

// String oculta la clave.
func (p OriginPEM) String() string { return fmt.Sprintf("%+v", plainOriginPEM(p.redacted())) }

// GoString es String para %#v.
func (p OriginPEM) GoString() string { return p.String() }

// LogValue es String para slog.
func (p OriginPEM) LogValue() slog.Value { return slog.StringValue(p.String()) }

// MarshalJSON es el struct con la clave como "***".
func (p OriginPEM) MarshalJSON() ([]byte, error) { return json.Marshal(plainOriginPEM(p.redacted())) }

// OriginInputs es lo que BuildProxyConfig necesita de Origin CA: los
// certificados active por hostname (con la clave descifrada), lo observado
// de cada hostname y la hora.
type OriginInputs struct {
	Certs map[string]entity.OriginCert
	Hosts map[string]entity.OriginHost
	Now   time.Time
}

// OriginTarget es un hostname que puede usar un certificado de Origin CA:
// el de un dominio de una app o el del panel (§4.1 del diseño).
type OriginTarget struct {
	Hostname string
	// ZoneID es la zona de Cloudflare del host (cf_zone_id, o
	// panel_cf_zone_id); "" si no cae en una.
	ZoneID string
	// RecordID es el A que maneja muelle (cf_record_id); "" en el panel.
	RecordID string
	Panel    bool
	// Eligible: un dominio con tls cloudflare, zona y su registro manejado
	// por muelle; el panel, con su zona.
	Eligible bool
	// Proxied es lo que pidió muelle (domains.proxied). El panel no tiene un
	// proxied en muelle: siempre true, y lo decide lo observado.
	Proxied bool
}

// Candidate dice si el host pide un certificado de Origin CA, sin mirar
// Cloudflare: elegible y proxied en muelle.
func (t OriginTarget) Candidate() bool { return t.Eligible && t.Proxied }

// DomainOriginTarget es el host de un dominio de una app. El status del
// dominio no cuenta: uno en error + cloudflare sigue siendo candidato.
func DomainOriginTarget(d entity.Domain) OriginTarget {
	return OriginTarget{
		Hostname: d.Hostname, ZoneID: d.CFZoneID, RecordID: d.CFRecordID,
		Eligible: d.TLSMode == entity.TLSCloudflare && d.CFZoneID != "" && d.Managed(),
		Proxied:  d.Proxied,
	}
}

// PanelOriginTarget es el host del panel: califica si cae en una zona,
// aunque muelle no maneje su registro (solo lo lee, §4.2 del diseño).
func PanelOriginTarget(hostname, zoneID string) OriginTarget {
	return OriginTarget{Hostname: hostname, ZoneID: zoneID, Panel: true, Eligible: hostname != "" && zoneID != "", Proxied: true}
}

// OriginOn dice si los certificados de Cloudflare están en uso: la opción
// encendida, con token y con el proxy habilitado (proxyEnabled,
// ProxySettings.Enabled). Si no, no hay candidatos y la API dice
// origin_ca: null.
func OriginOn(p shared.ProxySettings, proxyEnabled bool) bool {
	return p.CloudflareOriginCA && p.CloudflareToken != "" && proxyEnabled
}

// ObserveRecords es lo observado con los registros de un nombre (§4.1):
// proxied si hay al menos un A, AAAA o CNAME y todos tienen el proxy (con
// managedID, el A de muelle, entre ellos si no es ""); dns_only si alguno no
// lo tiene; missing si no hay ninguno, o si falta el A de muelle.
func ObserveRecords(records []Record, managedID string) entity.Observation {
	seen, managed := 0, false
	for _, r := range records {
		if r.Type != "A" && r.Type != "AAAA" && r.Type != "CNAME" {
			continue
		}
		seen++
		if !r.Proxied {
			return entity.ObservedDNSOnly
		}
		if r.ID == managedID {
			managed = true
		}
	}
	if seen == 0 || (managedID != "" && !managed) {
		return entity.ObservedMissing
	}
	return entity.ObservedProxied
}

// OriginServes dice si el host se sirve con Origin CA (§4.1), con la opción
// en uso: es candidato, lo observado es proxied y tiene un active de ese
// host y de esa zona que vence en más de 14 días (RenewMargin).
func OriginServes(t OriginTarget, h entity.OriginHost, active *entity.OriginCert, now time.Time) bool {
	return t.Candidate() && h.Observed == entity.ObservedProxied && active != nil &&
		active.Hostname == t.Hostname && active.ZoneID == t.ZoneID && active.ExpiresAt.Sub(now) > RenewMargin
}

// NeedsOriginCert dice si hay que emitir (sin un active, o uno de otra zona
// o de otro hostname) o renovar (el active vence en menos de 30 días, §5.3).
func NeedsOriginCert(t OriginTarget, active *entity.OriginCert, now time.Time) bool {
	return active == nil || active.Hostname != t.Hostname || active.ZoneID != t.ZoneID ||
		active.ExpiresAt.Sub(now) < OriginCARenewBefore
}

// OriginView es origin_ca (contrato §3.7). ErrorCode "" es null.
type OriginView struct {
	Status    entity.OriginStatus
	ErrorCode entity.OriginErrorCode
	Error     string
	RetryAt   *time.Time
	ExpiresAt *time.Time
}

// OriginCAView arma origin_ca de un host (§10.1 del diseño), el primero que
// aplica:
//  1. sin la opción en uso (on, OriginOn): nil;
//  2. no es elegible: not_eligible;
//  3. un dominio con proxied false en muelle: dns_only;
//  4. lo observado es dns_only: dns_only;
//  5. lo observado es missing o unreadable: not_eligible;
//  6. se sirve (OriginServes): active, con la falla si la renovación falló;
//  7. tiene una falla: failed;
//  8. si no: issuing (también sin mirar Cloudflare todavía).
//
// error_code, error y retry_at van solo con failed y active. expires_at es
// el del active de ese hostname, con cualquier status. El mensaje se arma
// con el status (la plantilla de emisión o la de renovación, §9.3).
func OriginCAView(on bool, t OriginTarget, active *entity.OriginCert, h entity.OriginHost, now time.Time) *OriginView {
	if !on {
		return nil
	}
	v := &OriginView{}
	if active != nil {
		exp := active.ExpiresAt
		v.ExpiresAt = &exp
	}
	switch {
	case !t.Eligible:
		v.Status = entity.OriginStatusNotEligible
	case !t.Proxied:
		v.Status = entity.OriginStatusDNSOnly
	case h.Observed == entity.ObservedDNSOnly:
		v.Status = entity.OriginStatusDNSOnly
	case h.Observed == entity.ObservedMissing || h.Observed == entity.ObservedUnreadable:
		v.Status = entity.OriginStatusNotEligible
	case OriginServes(t, h, active, now):
		v.Status = entity.OriginStatusActive
		if h.Failed() {
			exp := active.ExpiresAt
			v.withFailure(h, "muelle no pudo renovar el certificado de Cloudflare, que vence el "+shortDate(exp)+": "+
				h.LastError+"; si no lo renueva antes del "+shortDate(exp.Add(-RenewMargin))+", vuelve a Let's Encrypt")
		}
	case h.Failed():
		v.Status = entity.OriginStatusFailed
		v.withFailure(h, "muelle no pudo emitir el certificado de Cloudflare: "+h.LastError+"; mientras tanto usa Let's Encrypt")
	default:
		v.Status = entity.OriginStatusIssuing
	}
	return v
}

func (v *OriginView) withFailure(h entity.OriginHost, msg string) {
	v.ErrorCode, v.Error = h.ErrorCode, msg
	if h.NextAttemptAt != nil {
		at := *h.NextAttemptAt
		v.RetryAt = &at
	}
}

// Los valores de cert_source (contrato §3.1).
const (
	CertSourceLetsEncrypt  = "letsencrypt"
	CertSourceCloudflareCA = "cloudflare_origin_ca"
)

// CertSource es cert_source: cloudflare_origin_ca si y solo si origin_ca
// es active; si no (también con origin_ca null), letsencrypt.
func CertSource(v *OriginView) string {
	if v != nil && v.Status == entity.OriginStatusActive {
		return CertSourceCloudflareCA
	}
	return CertSourceLetsEncrypt
}

// OriginCertError: el certificado que devolvió Cloudflare no sirve (no va
// con la clave, no es para el hostname o no vale ahora, §5.1). No se guarda.
type OriginCertError struct{ Reason string }

func (e *OriginCertError) Error() string {
	return "Cloudflare devolvió un certificado que no sirve: " + e.Reason
}

// ClassifyOriginError clasifica un error de la emisión (§9.1 del diseño):
//   - Cloudflare no responde (envuelve shared.ErrCloudflareUnavailable):
//     cloudflare;
//   - un *DNSError con HTTP 401 o 403, o con el código 10000 o 1010:
//     permission (también "API Access" apagado o la zona de otra cuenta);
//   - un *DNSError con 1100, 1101, 1200 o 1201 (fallas internas de
//     Cloudflare): cloudflare;
//   - otro *DNSError, o un *OriginCertError: rejected.
//
// Cualquier otro error (un plazo vencido, por ejemplo) cuenta como
// cloudflare: se reintenta pronto.
func ClassifyOriginError(err error) entity.OriginErrorCode {
	if errors.Is(err, shared.ErrCloudflareUnavailable) {
		return entity.OriginCloudflare
	}
	if _, ok := errors.AsType[*OriginCertError](err); ok {
		return entity.OriginRejected
	}
	de, ok := errors.AsType[*DNSError](err)
	if !ok {
		return entity.OriginCloudflare
	}
	switch {
	case de.Status == http.StatusUnauthorized || de.Status == http.StatusForbidden || de.Code == 10000 || de.Code == 1010:
		return entity.OriginPermission
	case de.Code == 1100 || de.Code == 1101 || de.Code == 1200 || de.Code == 1201:
		return entity.OriginCloudflare
	}
	return entity.OriginRejected
}

// OriginBackoff es cuánto espera el host después de su falla número
// failures (§9.2): con cloudflare 1, 2, 4… min hasta 1 h; con permission o
// rejected, 6 h fijo (no sirve insistir hasta que cambie el token o la zona).
func OriginBackoff(failures int, code entity.OriginErrorCode) time.Duration {
	if code != entity.OriginCloudflare {
		return 6 * time.Hour
	}
	d := time.Minute
	for i := 1; i < failures && d < time.Hour; i++ {
		d *= 2
	}
	return min(d, time.Hour)
}

// OriginFailureCause es la causa de una falla (§9.3), lo que se guarda en
// origin_ca_hosts.last_error: según ClassifyOriginError, con el mensaje de
// Cloudflare cortado a 300 runas. El del panel, que no tiene retry, dice
// que se reintenta solo. Nunca trae el token: el cliente de Cloudflare ya lo
// saca de sus errores.
func OriginFailureCause(err error, t OriginTarget) string {
	switch ClassifyOriginError(err) {
	case entity.OriginPermission:
		retry := msgRetryDomain
		if t.Panel {
			retry = msgRetryPanel
		}
		return fmt.Sprintf(msgPermissionCause, t.Hostname, cfMessage(err)) + retry
	case entity.OriginRejected:
		if ce, ok := errors.AsType[*OriginCertError](err); ok {
			return "Cloudflare devolvió un certificado que no sirve (" + truncateRunes(ce.Reason, originMessageMax) + ")"
		}
		return "Cloudflare lo rechazó (" + cfMessage(err) + ")"
	}
	return "Cloudflare no respondió (" + unavailableDetail(err) + "); se reintenta solo"
}

// cfMessage es el mensaje de un *DNSError, cortado a 300 runas.
func cfMessage(err error) string {
	if de, ok := errors.AsType[*DNSError](err); ok && de.Message != "" {
		return truncateRunes(de.Message, originMessageMax)
	}
	return "sin detalle"
}

// unavailableDetail es lo que sigue a "Cloudflare no responde: " en el
// error del cliente, con "respondió 503" como "HTTP 503", cortado a 300
// runas.
func unavailableDetail(err error) string {
	msg := err.Error()
	if _, after, ok := strings.Cut(msg, shared.ErrCloudflareUnavailable.Error()+": "); ok {
		msg = after
	}
	if rest, ok := strings.CutPrefix(msg, "respondió "); ok && rest != "" && rest[0] >= '0' && rest[0] <= '9' {
		msg = "HTTP " + rest
	}
	if msg == "" {
		msg = "sin detalle"
	}
	return truncateRunes(msg, originMessageMax)
}

// ZonesPhrase nombra las zonas, ordenadas: "en la zona a.cl", "en las zonas
// a.cl y b.cl", y con tres o más las dos primeras y cuántas faltan ("en las
// zonas a.cl, b.cl y 3 más", contrato §4.1).
func ZonesPhrase(names []string) string {
	names = slices.Sorted(slices.Values(names))
	switch len(names) {
	case 0:
		return ""
	case 1:
		return "en la zona " + names[0]
	case 2:
		return "en las zonas " + names[0] + " y " + names[1]
	}
	return "en las zonas " + names[0] + ", " + names[1] + " y " + strconv.Itoa(len(names)-2) + " más"
}

// truncateRunes corta s a n runas, con "…" si sobra.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	i := 0
	for range n {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return s[:i] + "…"
}
