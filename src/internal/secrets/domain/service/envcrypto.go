// Package service tiene las reglas de dominio de secrets (cifrar, enmascarar
// y mezclar variables) y sus puertos hacia catalog.
package service

import (
	"cmp"
	"crypto/subtle"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/CPU-commits/muelle/src/internal/secrets/domain/entity"
	"github.com/CPU-commits/muelle/src/package/cipher"
)

// AAD liga cada valor cifrado a su dueño y su clave (§12 del spec):
// env:app:<id>:<clave> o env:stage:<id>:<clave>.
//
// El flag secret no forma parte de la AAD, porque el formato lo fija el spec
// (y la lista canónica de shared/infra/out/crypto). Pasar una variable de
// secreta a no secreta, para que GET muestre su valor, exige escribir
// directamente en el archivo de la base, y quien puede hacerlo ya es root (§12):
// tiene la clave maestra en el mismo host. Por la API, secret solo cambia
// reescribiendo el valor (PUT o PATCH), que nunca revela el anterior.
func AAD(o entity.Owner, key string) string {
	return fmt.Sprintf("env:%s:%d:%s", o.Kind, o.ID, key)
}

// MaskedVar es una variable tal como la ve la API. Value es nil si es secreta.
type MaskedVar struct {
	Key       string
	Value     *string
	Secret    bool
	UpdatedAt time.Time
}

// ciphertextOverhead es lo que el cifrado agrega a cada valor guardado. Sale
// del formato de shared/infra/out/crypto (AES-256-GCM): nonce de 12 bytes
// adelante y tag de 16 bytes al final, así que len(cifrado) = len(valor) + 28.
// TestCiphertextOverheadMatchesCrypto lo verifica contra el cifrado real; si
// el formato cambia, cambia esta constante.
const ciphertextOverhead = 12 + 16

// EnvCrypto cifra y descifra las variables de un dueño.
type EnvCrypto struct {
	c cipher.Cipher
}

func NewEnvCrypto(c cipher.Cipher) *EnvCrypto { return &EnvCrypto{c: c} }

// StoredSize es el Size en claro (clave + valor) de una variable guardada,
// medido sin descifrarla. Así medir no descifra con el lock de escritura
// tomado, y una fila rota no impide medir las demás.
func (e *EnvCrypto) StoredSize(r entity.EncryptedEnvVar) int {
	return len(r.Key) + max(len(r.Ciphertext)-ciphertextOverhead, 0)
}

// Unchanged dice si guardar v sobre la variable guardada stored no cambiaría
// nada: mismo secret y mismo valor. Descifra solo esa fila. Si no descifra,
// cuenta como cambiada, para que sobrescribirla la arregle.
func (e *EnvCrypto) Unchanged(o entity.Owner, stored entity.EncryptedEnvVar, v entity.EnvVar) bool {
	if stored.Secret != v.Secret || e.StoredSize(stored) != v.Size() {
		return false
	}
	plain, err := e.c.Decrypt(stored.Ciphertext, AAD(o, stored.Key))
	if err != nil {
		return false
	}
	// Comparación en tiempo constante: el valor guardado puede ser un secreto.
	return subtle.ConstantTimeCompare(plain, v.Value) == 1
}

// Seal cifra los valores de vars con la AAD de o.
func (e *EnvCrypto) Seal(o entity.Owner, vars []entity.EnvVar) ([]entity.EncryptedEnvVar, error) {
	if err := o.Check(); err != nil {
		return nil, err
	}
	out := make([]entity.EncryptedEnvVar, 0, len(vars))
	for _, v := range vars {
		out = append(out, entity.EncryptedEnvVar{
			Key:        v.Key,
			Ciphertext: e.c.Encrypt(v.Value, AAD(o, v.Key)),
			Secret:     v.Secret,
			UpdatedAt:  v.UpdatedAt,
		})
	}
	return out, nil
}

// Open descifra todas las variables de o, secretas o no. Lo usa la mezcla
// para el deploy; la API nunca recibe estos valores (usa Mask).
func (e *EnvCrypto) Open(o entity.Owner, rows []entity.EncryptedEnvVar) ([]entity.EnvVar, error) {
	if err := o.Check(); err != nil {
		return nil, err
	}
	out := make([]entity.EnvVar, 0, len(rows))
	for _, r := range rows {
		plain, err := e.decrypt(o, r)
		if err != nil {
			return nil, err
		}
		out = append(out, entity.EnvVar{Key: r.Key, Value: plain, Secret: r.Secret, UpdatedAt: r.UpdatedAt})
	}
	return out, nil
}

// Mask devuelve las variables como las ve la API: descifra solo las que no
// son secretas. El valor de una secreta ni siquiera se descifra.
func (e *EnvCrypto) Mask(o entity.Owner, rows []entity.EncryptedEnvVar) ([]MaskedVar, error) {
	if err := o.Check(); err != nil {
		return nil, err
	}
	out := make([]MaskedVar, 0, len(rows))
	for _, r := range rows {
		v := MaskedVar{Key: r.Key, Secret: r.Secret, UpdatedAt: r.UpdatedAt}
		if !r.Secret {
			plain, err := e.decrypt(o, r)
			if err != nil {
				return nil, err
			}
			s := string(plain)
			v.Value = &s
		}
		out = append(out, v)
	}
	return out, nil
}

func (e *EnvCrypto) decrypt(o entity.Owner, r entity.EncryptedEnvVar) ([]byte, error) {
	plain, err := e.c.Decrypt(r.Ciphertext, AAD(o, r.Key))
	if err != nil {
		// La clave no es secreta; el valor nunca va en el error.
		return nil, fmt.Errorf("secrets: descifrando %s de %s %d: %w", r.Key, o.Kind, o.ID, err)
	}
	return plain, nil
}

// SecretValues son los valores de las variables secretas de los sets, sin
// repetidos ni vacíos, de los más largos a los más cortos (a igual largo, en
// orden alfabético): enmascarar primero el más largo evita dejar a la vista
// la parte de un secreto que contiene a otro.
func SecretValues(sets ...[]entity.EnvVar) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, set := range sets {
		for _, v := range set {
			if s := string(v.Value); v.Secret && s != "" && !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	slices.SortFunc(out, func(a, b string) int {
		if c := cmp.Compare(len(b), len(a)); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})
	return out
}

// Merge combina las variables del stage con las de la app; la app gana (§5.2 del spec).
func Merge(stage, app []entity.EnvVar) map[string]string {
	out := make(map[string]string, len(stage)+len(app))
	for _, set := range [][]entity.EnvVar{stage, app} {
		for _, v := range set {
			out[v.Key] = string(v.Value)
		}
	}
	return out
}
