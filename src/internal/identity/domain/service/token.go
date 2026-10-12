package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// APITokenPrefix identifica los tokens de API (§10.1 del spec).
const APITokenPrefix = "mk_"

// RandomToken genera 32 bytes aleatorios en base64url con el prefijo dado y
// devuelve el token en claro y su SHA-256, que es lo único que se guarda.
// Con 256 bits de entropía no hace falta un hash lento.
func RandomToken(prefix string) (plain, hash string) {
	b := make([]byte, 32)
	rand.Read(b)
	plain = prefix + base64.RawURLEncoding.EncodeToString(b)
	return plain, HashToken(plain)
}

// HashToken es el SHA-256 en hexadecimal de un token de sesión, invitación o API.
func HashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
