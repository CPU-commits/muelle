package entity

// Claves de la tabla settings que maneja el módulo. La de require_2fa, que
// también lee identity, es shared.SettingRequire2FA (src/shared/domain/entity).
const (
	// KeyMasterKeyCheck guarda un valor de control cifrado para detectar una clave maestra distinta.
	KeyMasterKeyCheck = "master_key_check"
)

// MasterKeyCheckAAD liga el valor de control a su fila.
const MasterKeyCheckAAD = "settings:" + KeyMasterKeyCheck

// Settings son los ajustes editables. En la Fase 1 solo existe require_2fa.
type Settings struct {
	Require2FA bool
}
