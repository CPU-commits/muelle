package entity

// Claves de la tabla settings que maneja el módulo.
const (
	// KeyRequire2FA guarda "true" o "false"; identity lee este mismo formato.
	KeyRequire2FA = "require_2fa"
	// KeyMasterKeyCheck guarda un valor de control cifrado para detectar una clave maestra distinta.
	KeyMasterKeyCheck = "master_key_check"
)

// MasterKeyCheckAAD liga el valor de control a su fila.
const MasterKeyCheckAAD = "settings:" + KeyMasterKeyCheck

// Settings son los ajustes editables. En la Fase 1 solo existe require_2fa.
type Settings struct {
	Require2FA bool
}

// FormatBool es el formato en que se guarda un ajuste booleano.
func FormatBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// ParseBool lee un ajuste booleano guardado. Todo lo que no sea "true" es false.
func ParseBool(v string) bool { return v == "true" }
