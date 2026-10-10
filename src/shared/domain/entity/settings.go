package entity

// SettingRequire2FA es la clave de la tabla settings que dice si muelle exige
// 2FA (§10.1 del spec). La escribe admin y la lee identity, los dos con
// FormatBool y ParseBool: este es el único lugar donde está el contrato.
const SettingRequire2FA = "require_2fa"

// FormatBool es el formato en que se guarda un ajuste booleano: "true" o "false".
func FormatBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// ParseBool lee un ajuste booleano guardado. Todo lo que no sea exactamente
// "true" es false, así un valor raro no activa nada.
func ParseBool(v string) bool { return v == "true" }
