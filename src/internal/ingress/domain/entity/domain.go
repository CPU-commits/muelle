// Package entity tiene las entidades de ingress: el dominio de una app, con
// sus estados (contrato §3.1), y los registros DNS que necesita.
package entity

import (
	"time"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// Status es el estado de un dominio (contrato §3.1).
type Status string

const (
	// StatusPending: esperando un certificado válido (o creado sin verificar todavía).
	StatusPending Status = "pending"
	// StatusActive: con un certificado válido que vence en más de 14 días.
	StatusActive Status = "active"
	// StatusError: con ErrorCode y LastError, que dicen por qué.
	StatusError Status = "error"
)

// ErrorCode dice por qué un dominio está en error, y con eso el panel elige
// la acción (contrato §3.1).
type ErrorCode string

const (
	// CodeRecordExists: ya hay un registro de ese nombre que muelle no creó (→ Adoptar).
	CodeRecordExists ErrorCode = "record_exists"
	// CodeCloudflare: Cloudflare falló, o falta el token o la IP pública (→ Reintentar).
	CodeCloudflare ErrorCode = "cloudflare"
	// CodeCertificate: el certificado es inválido o vence pronto (→ Reintentar).
	CodeCertificate ErrorCode = "certificate"
)

// TLSMode es cómo emite Caddy el certificado del dominio (§8 del spec).
type TLSMode string

const (
	// TLSCloudflare: ACME con el desafío DNS-01 en Cloudflare. Necesita CFZoneID.
	TLSCloudflare TLSMode = "cloudflare"
	// TLSHTTP: ACME con HTTP-01 o TLS-ALPN-01 (necesita 80/443 desde afuera).
	TLSHTTP TLSMode = "http"
)

// Domain es una fila de domains (§5.1, contrato §3.1). Un string vacío es NULL.
type Domain struct {
	ID, AppID      int64
	Hostname       string
	TargetService  string
	TargetPort     int
	Proxied        bool
	TLSMode        TLSMode
	CFZoneID       string
	CFRecordID     string // el A que maneja muelle
	CFRecordIDAAAA string // el AAAA que maneja muelle (nunca sin CFRecordID)
	Status         Status
	ErrorCode      ErrorCode // solo con StatusError
	LastError      string    // solo con StatusError
	CertIssuer     string
	CertExpiresAt  *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time // el último cambio (contrato §3.1); el plazo de 10 min cuenta desde acá
}

// NewDomain valida un dominio nuevo y lo deja pending, con tls_mode http
// provisorio y sin registros: el DNS se hace después de guardarlo (D5 del
// plan de la Fase 3). Los errores son *validate.Error en hostname,
// target_service o target_port.
func NewDomain(appID int64, hostname, service string, port int, proxied bool, now time.Time) (Domain, error) {
	if err := validate.Hostname("hostname", hostname); err != nil {
		return Domain{}, err
	}
	if err := ValidateTarget(service, port); err != nil {
		return Domain{}, err
	}
	return Domain{
		AppID:         appID,
		Hostname:      hostname,
		TargetService: service,
		TargetPort:    port,
		Proxied:       proxied,
		TLSMode:       TLSHTTP,
		Status:        StatusPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

// ValidateTarget valida el destino: el nombre corto de un servicio (la regla
// de un servicio compose, validate.VolumeName) y su puerto.
func ValidateTarget(service string, port int) error {
	if err := validate.VolumeName("target_service", service); err != nil {
		return err
	}
	return validate.Port("target_port", port)
}

// SetTarget cambia el destino. Uno igual no es un cambio (changed false) y
// no mueve UpdatedAt; uno inválido no cambia nada.
func (d *Domain) SetTarget(service string, port int, now time.Time) (changed bool, err error) {
	if err := ValidateTarget(service, port); err != nil {
		return false, err
	}
	if d.TargetService == service && d.TargetPort == port {
		return false, nil
	}
	d.TargetService, d.TargetPort = service, port
	d.UpdatedAt = now
	return true, nil
}

// Fail deja el dominio en error con su código y su mensaje (en español).
func (d *Domain) Fail(code ErrorCode, msg string, now time.Time) {
	d.Status, d.ErrorCode, d.LastError = StatusError, code, msg
	d.UpdatedAt = now
}

// MarkPending lo deja pending, sin error. El plazo de 10 min del certificado
// vuelve a contar desde now.
func (d *Domain) MarkPending(now time.Time) {
	d.Status, d.ErrorCode, d.LastError = StatusPending, "", ""
	d.UpdatedAt = now
}

// MarkActive lo deja active, sin error.
func (d *Domain) MarkActive(now time.Time) {
	d.Status, d.ErrorCode, d.LastError = StatusActive, "", ""
	d.UpdatedAt = now
}

// Touch marca un cambio que no es de estado (un PATCH, adoptar, rehacer el DNS).
func (d *Domain) Touch(now time.Time) { d.UpdatedAt = now }

// SetCertificate guarda lo que vio la verificación en un certificado válido.
// No mueve UpdatedAt (contrato §3.1). El vencimiento queda en UTC y a
// milisegundos, como en la base; changed dice si algo cambió.
func (d *Domain) SetCertificate(issuer string, expires time.Time) (changed bool) {
	exp := expires.UTC().Truncate(time.Millisecond)
	if d.CertIssuer == issuer && d.CertExpiresAt != nil && d.CertExpiresAt.Equal(exp) {
		return false
	}
	d.CertIssuer, d.CertExpiresAt = issuer, &exp
	return true
}

// Managed dice si muelle maneja el registro A del dominio en Cloudflare.
func (d Domain) Managed() bool { return d.CFRecordID != "" }
