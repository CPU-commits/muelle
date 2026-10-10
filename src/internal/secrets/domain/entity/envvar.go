package entity

import (
	"bytes"
	"maps"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

const (
	MaxVars       = 500      // variables por dueño
	MaxValueBytes = 64 << 10 // bytes por valor
	// MaxOwnerBytes es el total por dueño (claves + valores). Sin él, 500
	// valores de 64 KiB darían un GET de 32 MB y un entorno que supera el
	// ARG_MAX de Linux al desplegar.
	MaxOwnerBytes = 1 << 20
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

// NewEnvVar valida la clave y el valor. field es el campo del cuerpo donde
// viene la variable ("vars" o "set"), para que el error lo nombre.
//
// El valor tiene que ser texto UTF-8 válido y sin NUL: las respuestas JSON no
// pueden llevar UTF-8 inválido (GET fallaría con un 500), y el entorno de un
// proceso no puede llevar NUL (el deploy no podría entregarla).
func NewEnvVar(field, key string, value []byte, secret bool, at time.Time) (EnvVar, error) {
	if err := validate.EnvKey(field, key); err != nil {
		return EnvVar{}, err
	}
	if len(value) > MaxValueBytes {
		return EnvVar{}, validate.Errorf(field, "el valor de %q supera 64 KiB", key)
	}
	if !utf8.Valid(value) {
		return EnvVar{}, validate.Errorf(field, "el valor de %q no es UTF-8 válido", key)
	}
	if bytes.IndexByte(value, 0) >= 0 {
		return EnvVar{}, validate.Errorf(field, "el valor de %q no puede contener el carácter NUL", key)
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

// Size es lo que ocupa la variable en el entorno: clave + valor, en bytes.
func (v EnvVar) Size() int { return len(v.Key) + len(v.Value) }

// TotalSize suma el Size de vars.
func TotalSize(vars []EnvVar) int {
	n := 0
	for _, v := range vars {
		n += v.Size()
	}
	return n
}

// CheckLimits revisa que las variables con que queda un dueño no superen
// MaxVars ni MaxOwnerBytes.
func CheckLimits(field string, vars []EnvVar) error {
	if err := CheckCount(field, len(vars)); err != nil {
		return err
	}
	if TotalSize(vars) > MaxOwnerBytes {
		return validate.Errorf(field, "las variables suman más de 1 MiB (claves y valores)")
	}
	return nil
}

// ApplyPatch devuelve, ordenadas por clave, las variables con que queda un
// dueño que tiene current después de agregar o reemplazar set y borrar unset.
// No modifica current.
func ApplyPatch(current, set []EnvVar, unset []string) []EnvVar {
	byKey := make(map[string]EnvVar, len(current)+len(set))
	for _, v := range current {
		byKey[v.Key] = v
	}
	for _, k := range unset {
		delete(byKey, k)
	}
	for _, v := range set {
		byKey[v.Key] = v
	}
	out := make([]EnvVar, 0, len(byKey))
	for _, k := range slices.Sorted(maps.Keys(byKey)) {
		out = append(out, byKey[k])
	}
	return out
}
