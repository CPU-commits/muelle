// Package crypto cifra valores con AES-256-GCM usando la clave maestra.
//
// Cada valor se cifra con un nonce aleatorio y con datos asociados (AAD) que
// lo ligan a su fila, así un valor cifrado copiado a otra fila no descifra.
// Cada fila y columna cifrada usa una AAD única con el formato
// <tabla>:<dueño>:<clave> (ej. "env:app:12:DATABASE_URL"). Solo la última
// parte puede contener ':', para que dos AAD distintas nunca se confundan.
// La AAD nunca es vacía: Encrypt hace panic y Decrypt devuelve cipher.ErrDecrypt.
package crypto

import (
	"crypto/aes"
	stdcipher "crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/CPU-commits/muelle/src/package/cipher"
)

const KeySize = 32

type Cipher struct {
	aead stdcipher.AEAD
}

var _ cipher.Cipher = (*Cipher)(nil)

func New(key []byte) (*Cipher, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("crypto: la clave debe tener %d bytes, tiene %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := stdcipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt devuelve nonce || ciphertext. Una AAD vacía es un error de
// programación, así que hace panic.
func (c *Cipher) Encrypt(plaintext []byte, aad string) []byte {
	if aad == "" {
		panic("crypto: AAD vacía")
	}
	nonce := make([]byte, c.aead.NonceSize())
	rand.Read(nonce)
	return c.aead.Seal(nonce, nonce, plaintext, []byte(aad))
}

func (c *Cipher) Decrypt(data []byte, aad string) ([]byte, error) {
	n := c.aead.NonceSize()
	if aad == "" || len(data) < n {
		return nil, cipher.ErrDecrypt
	}
	plaintext, err := c.aead.Open(nil, data[:n], data[n:], []byte(aad))
	if err != nil {
		return nil, cipher.ErrDecrypt
	}
	return plaintext, nil
}

// LoadKeyFile lee una clave en base64 estándar. Se toleran saltos de línea en
// cualquier lado (base64 los ignora) y espacios solo en los extremos.
func LoadKeyFile(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("crypto: leyendo la clave maestra: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("crypto: la clave maestra no es base64 válido: %w", err)
	}
	if len(key) != KeySize {
		return nil, fmt.Errorf("crypto: la clave maestra debe tener %d bytes, tiene %d", KeySize, len(key))
	}
	return key, nil
}

// GenerateKey devuelve una clave nueva en base64, lista para guardar en un archivo.
func GenerateKey() string {
	key := make([]byte, KeySize)
	rand.Read(key)
	return base64.StdEncoding.EncodeToString(key)
}
