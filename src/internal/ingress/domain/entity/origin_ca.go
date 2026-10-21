package entity

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// Los certificados de Cloudflare Origin CA (contrato §3.7, diseño de Origin
// CA §4 y §5): los que emitió muelle (origin_ca_certs) y lo que vio de cada
// hostname en Cloudflare, con la última falla (origin_ca_hosts).

// OriginCertState es el estado de un certificado de Origin CA.
type OriginCertState string

const (
	// OriginActive: el vigente de su hostname (a lo más uno). Se sirve si el
	// host califica y vence en más de 14 días.
	OriginActive OriginCertState = "active"
	// OriginRetired: ya no se sirve, y se revoca después de que Caddy cargó
	// una configuración sin él (§6 del diseño). Su clave ya se borró.
	OriginRetired OriginCertState = "retired"
)

// OriginCert es una fila de origin_ca_certs. KeyPEM va en claro (PKCS#8 en
// PEM; "" en uno retirado): String, GoString, LogValue y MarshalJSON la
// ocultan, como el token en ProxySettings.
type OriginCert struct {
	ID       int64
	Hostname string
	ZoneID   string
	// CFCertID es el id de Cloudflare (el serial en decimal), para revocarlo.
	CFCertID string
	// CertPEM es la hoja, en PEM.
	CertPEM   string
	KeyPEM    string
	ExpiresAt time.Time // el NotAfter de la hoja
	State     OriginCertState
	CreatedAt time.Time
	RetiredAt *time.Time // solo con OriginRetired
	// RevokeFailures y NextRevokeAt son los reintentos de la revocación.
	RevokeFailures int
	NextRevokeAt   *time.Time
}

// redacted es c con la clave como "***" (si hay).
func (c OriginCert) redacted() OriginCert {
	if c.KeyPEM != "" {
		c.KeyPEM = "***"
	}
	return c
}

// plainOriginCert es OriginCert sin métodos: String y MarshalJSON lo usan
// para no volver a llamarse.
type plainOriginCert OriginCert

// String oculta la clave.
func (c OriginCert) String() string { return fmt.Sprintf("%+v", plainOriginCert(c.redacted())) }

// GoString es String para %#v.
func (c OriginCert) GoString() string { return c.String() }

// LogValue es String para slog: el JSONHandler serializaría el struct.
func (c OriginCert) LogValue() slog.Value { return slog.StringValue(c.String()) }

// MarshalJSON es el struct con la clave como "***", para un OriginCert
// dentro de otro valor que se serializa. Nada se guarda ni se responde así.
func (c OriginCert) MarshalJSON() ([]byte, error) {
	return json.Marshal(plainOriginCert(c.redacted()))
}

// Observation es lo que vio muelle en Cloudflare de los registros A, AAAA y
// CNAME de un hostname (origin_ca_hosts.observed). "" es que todavía no los
// leyó.
type Observation string

const (
	// ObservedProxied: hay al menos uno y todos tienen el proxy (en un
	// dominio, con el A de muelle entre ellos).
	ObservedProxied Observation = "proxied"
	// ObservedDNSOnly: alguno no tiene el proxy (la nube gris).
	ObservedDNSOnly Observation = "dns_only"
	// ObservedMissing: no hay ninguno, o en un dominio no está el A de muelle.
	ObservedMissing Observation = "missing"
	// ObservedUnreadable: Cloudflare rechazó la lectura.
	ObservedUnreadable Observation = "unreadable"
)

// Valid dice si o es uno de los valores que guarda la base.
func (o Observation) Valid() bool {
	switch o {
	case ObservedProxied, ObservedDNSOnly, ObservedMissing, ObservedUnreadable:
		return true
	}
	return false
}

// OriginErrorCode dice por qué falló la última emisión o renovación
// (origin_ca.error_code, contrato §3.7).
type OriginErrorCode string

const (
	// OriginPermission: al token le falta Zone → SSL and Certificates → Edit
	// en esa zona, el usuario dueño del token no tiene "API Access", o la
	// zona es de otra cuenta.
	OriginPermission OriginErrorCode = "permission"
	// OriginCloudflare: Cloudflare no respondió (sin red, 5xx, 429).
	OriginCloudflare OriginErrorCode = "cloudflare"
	// OriginRejected: Cloudflare rechazó el pedido por otro motivo, o
	// devolvió un certificado que no sirve.
	OriginRejected OriginErrorCode = "rejected"
)

// Valid dice si c es uno de los códigos que guarda la base.
func (c OriginErrorCode) Valid() bool {
	switch c {
	case OriginPermission, OriginCloudflare, OriginRejected:
		return true
	}
	return false
}

// OriginHost es una fila de origin_ca_hosts: lo último que vio muelle en
// Cloudflare de un hostname y la última emisión que falló. Un string vacío
// es NULL. Con Failures > 0 siempre hay ErrorCode, LastError y NextAttemptAt
// (los CHECK de la 0006).
type OriginHost struct {
	Hostname   string
	Observed   Observation
	ObservedAt *time.Time
	Failures   int
	ErrorCode  OriginErrorCode
	// LastError es solo la causa, en español: el mensaje entero se arma al
	// leer, según el status (§9.3 del diseño).
	LastError     string
	NextAttemptAt *time.Time
	UpdatedAt     time.Time
}

// Failed dice si la última emisión o renovación falló.
func (h OriginHost) Failed() bool { return h.Failures > 0 }

// OriginStatus es origin_ca.status (contrato §3.7).
type OriginStatus string

const (
	OriginStatusActive      OriginStatus = "active"
	OriginStatusIssuing     OriginStatus = "issuing"
	OriginStatusFailed      OriginStatus = "failed"
	OriginStatusDNSOnly     OriginStatus = "dns_only"
	OriginStatusNotEligible OriginStatus = "not_eligible"
)
