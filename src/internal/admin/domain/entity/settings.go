package entity

import shared "github.com/CPU-commits/muelle/src/shared/domain/entity"

// Claves de la tabla settings que maneja el módulo. La de require_2fa, que
// también lee identity, es shared.SettingRequire2FA (src/shared/domain/entity).
const (
	// KeyMasterKeyCheck guarda un valor de control cifrado para detectar una clave maestra distinta.
	KeyMasterKeyCheck = "master_key_check"
)

// MasterKeyCheckAAD liga el valor de control a su fila.
const MasterKeyCheckAAD = "settings:" + KeyMasterKeyCheck

// Settings son los ajustes que se leen y editan por /settings.
type Settings struct {
	Require2FA bool
	// Proxy son los del proxy y el DNS (§5.1), que aplica ingress.
	Proxy shared.ProxySettings
	// CloudflareZones son las zonas que ve el token; nil si no se sabe.
	CloudflareZones *int
}
