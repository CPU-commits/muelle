package service

import (
	"context"
	"time"
)

// PasswordHasher calcula y verifica hashes de contraseñas (argon2id, §10.1).
type PasswordHasher interface {
	// Hash devuelve el hash en formato PHC, con los parámetros adentro.
	Hash(password string) (string, error)
	// Verify compara en tiempo constante usando los parámetros del propio hash.
	// Un hash con formato inválido devuelve error.
	Verify(hash, password string) (bool, error)
}

// TOTP genera secretos y valida códigos de 2FA.
type TOTP interface {
	// Generate crea un secreto nuevo para accountName y devuelve el secreto
	// (base32) y la URL otpauth:// para el código QR.
	Generate(accountName string) (secret, url string, err error)
	// Validate dice si code es válido para secret en el instante at
	// (tolerando un período de desfase) y devuelve su paso (TOTPStep), para
	// que TOTPReplayGuard no lo acepte dos veces.
	Validate(code, secret string, at time.Time) (step int64, ok bool)
}

// ScopeResolver traduce el alcance de un token (slugs de catalog) a ids.
type ScopeResolver interface {
	// Resolve devuelve el id del proyecto y, si stage no es vacío, el del
	// stage dentro de ese proyecto. Si alguno no existe devuelve
	// repository.ErrNotFound (shared/domain/repository).
	Resolve(ctx context.Context, project, stage string) (projectID int64, stageID *int64, err error)
}

// SettingsReader lee los ajustes que maneja admin.
type SettingsReader interface {
	// Require2FA dice si está activo require_2fa. Si nunca se configuró, es false.
	Require2FA(ctx context.Context) (bool, error)
}
