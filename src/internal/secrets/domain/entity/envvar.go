package entity

import (
	"slices"
	"time"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

const (
	MaxVars       = 500      // variables por dueño
	MaxValueBytes = 64 << 10 // bytes por valor
)

// EnvVar es una variable con su valor en claro. El valor en claro solo vive
// en memoria: se guarda cifrado (EncryptedEnvVar).
type EnvVar struct {
	Key       string
	Value     []byte
	Secret    bool
	UpdatedAt time.Time
}

// EncryptedEnvVar es una variable tal como se guarda: Ciphertext va cifrado
// con la AAD de su dueño y su clave (domain/service.AAD).
type EncryptedEnvVar struct {
	Key        string
	Ciphertext []byte
	Secret     bool
	UpdatedAt  time.Time
}

// NewEnvVar valida la clave y el tamaño del valor. field es el campo del
// cuerpo donde viene la variable ("vars" o "set"), para que el error lo nombre.
func NewEnvVar(field, key string, value []byte, secret bool, at time.Time) (EnvVar, error) {
	if err := validate.EnvKey(field, key); err != nil {
		return EnvVar{}, err
	}
	if len(value) > MaxValueBytes {
		return EnvVar{}, validate.Errorf(field, "el valor de %q supera 64 KiB", key)
	}
	return EnvVar{Key: key, Value: value, Secret: secret, UpdatedAt: at}, nil
}

// NewEnvVars arma las variables de vars ordenadas por clave, todas con el
// mismo secret. No revisa MaxVars: eso depende de la operación (CheckCount).
func NewEnvVars(field string, vars map[string]string, secret bool, at time.Time) ([]EnvVar, error) {
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := make([]EnvVar, 0, len(keys))
	for _, k := range keys {
		v, err := NewEnvVar(field, k, []byte(vars[k]), secret, at)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// CheckCount revisa que un dueño no quede con más de MaxVars variables.
func CheckCount(field string, n int) error {
	if n > MaxVars {
		return validate.Errorf(field, "máximo %d variables", MaxVars)
	}
	return nil
}
