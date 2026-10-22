package dto

import "time"

// ProxyOutput es la respuesta de GET /api/v1/proxy (contrato §3.3). Los
// punteros nil salen como null: el panel decide por la presencia de cada
// clave, así que ninguna lleva omitempty.
type ProxyOutput struct {
	Status   string      `json:"status"`    // synced · out_of_sync · recreating · disabled
	Error    *string     `json:"error"`     // solo con out_of_sync
	SyncedAt *time.Time  `json:"synced_at"` // el último POST /load exitoso, en UTC y con a lo más ms
	Caddy    CaddyOutput `json:"caddy"`
	// PortsRollback es la última vez que muelle volvió a los puertos
	// anteriores porque muelle-caddy no arrancó con los nuevos; null si no
	// hubo (contrato §3.5).
	PortsRollback *PortsRollbackOutput `json:"ports_rollback"`
	// PanelCertificate es el certificado del panel; null sin panel_domain
	// (contrato §3.3).
	PanelCertificate *PanelCertificateOutput `json:"panel_certificate"`
}

// PanelCertificateOutput es el certificado del panel: hostname es
// panel_domain, y cert_source y origin_ca tienen las reglas de los de un
// dominio (el panel no tiene retry).
type PanelCertificateOutput struct {
	Hostname   string          `json:"hostname"`
	CertSource string          `json:"cert_source"`
	OriginCA   *OriginCAOutput `json:"origin_ca"`
}

// PortsRollbackOutput es una vuelta a los puertos anteriores del proxy.
type PortsRollbackOutput struct {
	Attempted ProxyPortsOutput `json:"attempted"` // los que no arrancaron
	Restored  ProxyPortsOutput `json:"restored"`  // a los que volvió (los de los ajustes)
	Error     string           `json:"error"`     // por qué no arrancó, en español
	At        time.Time        `json:"at"`        // en UTC y con a lo más ms
}

// ProxyPortsOutput son los puertos del host del proxy.
type ProxyPortsOutput struct {
	HTTPPort  int `json:"http_port"`
	HTTPSPort int `json:"https_port"`
}

// CaddyOutput es lo que corre de verdad (InspectContainer) y los puertos de
// los ajustes.
type CaddyOutput struct {
	Image     *string `json:"image"` // null si el contenedor no existe
	Running   bool    `json:"running"`
	HTTPPort  int     `json:"http_port"`
	HTTPSPort int     `json:"https_port"`
}
