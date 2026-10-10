// Package crypto cifra valores con AES-256-GCM usando la clave maestra.
//
// Cada valor se cifra con un nonce aleatorio y con datos asociados (AAD) que
// lo ligan a su fila, así un valor cifrado copiado a otra fila no descifra.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

const KeySize = 32

var ErrDecrypt = errors.New("crypto: no se pudo descifrar")

type Cipher struct {
	aead cipher.AEAD
}

func New(key []byte) (*Cipher, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("crypto: la clave debe tener %d bytes, tiene %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt devuelve nonce || ciphertext.
func (c *Cipher) Encrypt(plaintext []byte, aad string) []byte {
	nonce := make([]byte, c.aead.NonceSize())
	rand.Read(nonce)
	return c.aead.Seal(nonce, nonce, plaintext, []byte(aad))
}

func (c *Cipher) Decrypt(data []byte, aad string) ([]byte, error) {
	n := c.aead.NonceSize()
	if len(data) < n {
		return nil, ErrDecrypt
	}
	plaintext, err := c.aead.Open(nil, data[:n], data[n:], []byte(aad))
	if err != nil {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}

// LoadKeyFile lee una clave en base64 estándar (se toleran espacios y saltos de línea).
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
