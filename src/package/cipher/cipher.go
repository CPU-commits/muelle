// Package cipher define cómo los módulos cifran secretos sin depender de la implementación.
package cipher

import "errors"

// ErrDecrypt indica un valor alterado, otra clave maestra o una AAD distinta.
var ErrDecrypt = errors.New("cipher: no se pudo descifrar")

// Cipher cifra valores ligándolos a su fila y columna con aad (la lista de
// AAD canónicas está en el paquete shared/infra/out/crypto).
// aad nunca puede ser vacía: Encrypt hace panic y Decrypt devuelve ErrDecrypt.
type Cipher interface {
	Encrypt(plaintext []byte, aad string) []byte
	Decrypt(ciphertext []byte, aad string) ([]byte, error)
}
