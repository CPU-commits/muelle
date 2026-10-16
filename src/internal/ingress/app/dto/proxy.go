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
}

// CaddyOutput es lo que corre de verdad (InspectContainer) y los puertos de
// los ajustes.
type CaddyOutput struct {
	Image     *string `json:"image"` // null si el contenedor no existe
	Running   bool    `json:"running"`
	HTTPPort  int     `json:"http_port"`
	HTTPSPort int     `json:"https_port"`
}
