package dto

import "time"

// AppRef identifica una app por sus slugs ({app} en las rutas).
type AppRef struct{ Project, Stage, App string }

// CreateDomainInput es el cuerpo de POST {app}/domains (contrato §3.2).
type CreateDomainInput struct {
	AppRef        `json:"-"`
	Hostname      string `json:"hostname"`
	TargetService string `json:"target_service"`
	TargetPort    int    `json:"target_port"`
	// Proxied ausente vale false.
	Proxied *bool `json:"proxied"`
}

// UpdateDomainInput es el cuerpo de PATCH {app}/domains/{hostname}: lo que
// viene es lo que cambia, y {} no cambia nada. El hostname es el de la ruta:
// mandarlo en el cuerpo es un campo desconocido (400 invalid_json).
type UpdateDomainInput struct {
	AppRef        `json:"-"`
	Hostname      string  `json:"-"`
	Proxied       *bool   `json:"proxied"`
	TargetService *string `json:"target_service"`
	TargetPort    *int    `json:"target_port"`
}

// DomainRef identifica un dominio de una app.
type DomainRef struct {
	AppRef
	Hostname string
}

// RecordOutput es un registro DNS que necesita el dominio (records).
type RecordOutput struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

// DomainOutput es el dominio del contrato §3.1: todas las claves siempre,
// null donde no hay valor (sin omitempty: el panel mira la presencia). El
// AAAA que maneja muelle (cf_record_id_aaaa) no se expone.
type DomainOutput struct {
	Hostname      string     `json:"hostname"`
	TargetService string     `json:"target_service"`
	TargetPort    int        `json:"target_port"`
	Proxied       bool       `json:"proxied"`
	TLSMode       string     `json:"tls_mode"`
	CFZoneID      *string    `json:"cf_zone_id"`
	CFRecordID    *string    `json:"cf_record_id"`
	Status        string     `json:"status"`
	LastErrorCode *string    `json:"last_error_code"`
	LastError     *string    `json:"last_error"`
	CertIssuer    *string    `json:"cert_issuer"`
	CertExpiresAt *time.Time `json:"cert_expires_at"`
	// CertSource es qué certificado le pide muelle a Caddy: letsencrypt o
	// cloudflare_origin_ca (contrato §3.7). Calculado, nunca null.
	CertSource string `json:"cert_source"`
	// OriginCA es el estado del certificado de Cloudflare Origin CA; null
	// con la opción apagada, sin token o con el proxy deshabilitado.
	OriginCA  *OriginCAOutput `json:"origin_ca"`
	Records   []RecordOutput  `json:"records"` // nunca nil
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// OriginCAOutput es origin_ca (contrato §3.7): las cinco claves siempre.
// error_code, error y retry_at van solo con failed y con active (una
// renovación que falló); las horas, en UTC y con a lo más ms.
type OriginCAOutput struct {
	Status    string     `json:"status"` // active · issuing · failed · dns_only · not_eligible
	ErrorCode *string    `json:"error_code"`
	Error     *string    `json:"error"`
	RetryAt   *time.Time `json:"retry_at"`
	ExpiresAt *time.Time `json:"expires_at"`
}
