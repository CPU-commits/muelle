// Package dto tiene los datos que entran y salen de los casos de uso de admin.
package dto

import (
	"encoding/json/jsontext"
	"fmt"
	"log/slog"
	"time"
)

// SettingsOutput es GET /settings y la respuesta de PATCH. Todas las claves
// vienen siempre (sin omitempty): el panel usa su presencia para saber que la
// Fase 3 está.
type SettingsOutput struct {
	Require2FA bool `json:"require_2fa"`
	// CloudflareTokenSet dice si hay token; el token nunca vuelve.
	CloudflareTokenSet bool `json:"cloudflare_token_set"`
	// CloudflareZones son las zonas que ve el token; null si no se sabe (o no hay token).
	CloudflareZones *int `json:"cloudflare_zones"`
	// PublicIP y PublicIPv6: "" si no están configuradas (sin IPv6, sin registros AAAA).
	PublicIP   string `json:"public_ip"`
	PublicIPv6 string `json:"public_ipv6"`
	ACMEEmail  string `json:"acme_email"`
	// PanelDomain es de solo lectura: sale de MUELLE_PANEL_ORIGIN ("" en desarrollo).
	PanelDomain string `json:"panel_domain"`
	// PanelAllowedCIDRs nunca es null: [] es cualquier IP.
	PanelAllowedCIDRs []string `json:"panel_allowed_cidrs"`
	ProxyHTTPPort     int      `json:"proxy_http_port"`
	ProxyHTTPSPort    int      `json:"proxy_https_port"`
	// ClientIP es la IP con la que muelle ve la petición (la que compara
	// Caddy con panel_allowed_cidrs). No es un ajuste.
	ClientIP string `json:"client_ip"`
}

// GetSettingsInput: ClientIP es la IP de la petición (utils.ClientIP).
type GetSettingsInput struct {
	ClientIP string
}

// PatchSettingsInput: un campo nil no se toca. El cuerpo se lee en modo
// estricto: una clave desconocida, o de solo lectura (panel_domain), es 400 invalid_json.
type PatchSettingsInput struct {
	Require2FA *bool `json:"require_2fa"`
	// CloudflareToken: "" borra el token.
	CloudflareToken *string `json:"cloudflare_token"`
	// PublicIP y PublicIPv6: "" las borra.
	PublicIP          *string   `json:"public_ip"`
	PublicIPv6        *string   `json:"public_ipv6"`
	ACMEEmail         *string   `json:"acme_email"`
	PanelAllowedCIDRs *[]string `json:"panel_allowed_cidrs"`
	ProxyHTTPPort     *int      `json:"proxy_http_port"`
	ProxyHTTPSPort    *int      `json:"proxy_https_port"`
	// ConfirmLockout guarda una lista de panel_allowed_cidrs que deja fuera
	// a ClientIP (si no, 409 would_lock_out).
	ConfirmLockout bool `json:"confirm_lockout"`
	// ClientIP lo pone el handler (utils.ClientIP), no viene en el cuerpo.
	ClientIP string `json:"-"`
}

// InitialSettings es la configuración inicial de install (MUELLE_INIT_*, Fase
// 5). Se aplica solo en el primer arranque de la base: cada valor que no es
// cero se guarda si la clave todavía no está. El token va en claro: String,
// GoString y LogValue lo ocultan, también dentro de StartupSettings.
type InitialSettings struct {
	PublicIP        string
	PublicIPv6      string
	ACMEEmail       string
	ProxyHTTPPort   int
	ProxyHTTPSPort  int
	CloudflareToken string
	CaddyImage      string
}

// String es InitialSettings con el token como "***" (si hay).
func (i InitialSettings) String() string {
	if i.CloudflareToken != "" {
		i.CloudflareToken = "***"
	}
	type plain InitialSettings // sin métodos: no vuelve a llamar a String
	return fmt.Sprintf("%+v", plain(i))
}

// GoString es String para %#v.
func (i InitialSettings) GoString() string { return i.String() }

// LogValue es String para slog: el JSONHandler serializaría el struct.
func (i InitialSettings) LogValue() slog.Value { return slog.StringValue(i.String()) }

// StartupSettings es lo que el contenedor le pasa a admin al arrancar.
type StartupSettings struct {
	// PanelDomain es el host de MUELLE_PANEL_ORIGIN si es un dominio; si no, "".
	PanelDomain string
	Init        InitialSettings
}

// LogValue es StartupSettings para slog, con el token oculto (fmt usa
// InitialSettings.String en el campo).
func (s StartupSettings) LogValue() slog.Value {
	type plain StartupSettings
	return slog.StringValue(fmt.Sprintf("%+v", plain(s)))
}

// ListAuditInput: Limit nil = 100 y Before nil = desde el último evento.
type ListAuditInput struct {
	Limit  *int
	Before *int64
}

type AuditEventOutput struct {
	ID        int64          `json:"id"`
	ActorType string         `json:"actor_type"`
	ActorID   *int64         `json:"actor_id"`
	Action    string         `json:"action"`
	Target    string         `json:"target"`
	Details   jsontext.Value `json:"details"`
	At        time.Time      `json:"at"`
}

// BackupOutput dice qué pasó con el backup del día.
type BackupOutput struct {
	Name    string // nombre del archivo del día
	Created bool   // false si ya existía
	Removed int    // copias viejas borradas
}
