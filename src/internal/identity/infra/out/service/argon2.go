// Package service tiene los adaptadores de salida de identity: argon2id para
// las contraseñas, TOTP con pquerna/otp, y los lectores SQL de lo que
// identity necesita de catalog (alcance de tokens) y de admin (require_2fa).
package service

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
)

// Parámetros de argon2id de §10.1 del spec: m=19 MiB, t=2, p=1, el mínimo que
// recomienda OWASP.
const (
	argonMemory  = 19 * 1024 // KiB
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16

	// maxConcurrent es cuántos hashes se calculan a la vez como máximo: cada
	// uno usa argonMemory de RAM, y un pico de logins no debe romper el
	// presupuesto de memoria (§1 del spec).
	maxConcurrent = 2

	// Límites de un hash guardado: uno corrupto o adulterado en la base no
	// puede agotar la RAM ni la CPU, ni debilitar la comparación con una clave
	// o un salt cortos. Fuera de estos rangos el hash es inválido.
	maxMemory  = 256 * 1024 // KiB (256 MiB)
	maxTime    = 10
	minKeyLen  = 16
	minSaltLen = 8
)

var errBadHash = errors.New("identity: hash de contraseña con formato inválido")

// Argon2Hasher implementa service.PasswordHasher con argon2id en formato PHC.
// Hash y Verify comparten un semáforo de capacidad maxConcurrent.
type Argon2Hasher struct {
	sem chan struct{}
	// key es argon2.IDKey; los tests lo reemplazan para medir la concurrencia.
	key func(password, salt []byte, time, memory uint32, threads uint8, keyLen uint32) []byte
}

var _ service.PasswordHasher = (*Argon2Hasher)(nil)

func NewArgon2Hasher() *Argon2Hasher {
	return &Argon2Hasher{sem: make(chan struct{}, maxConcurrent), key: argon2.IDKey}
}

// Hash devuelve $argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>.
func (h *Argon2Hasher) Hash(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	rand.Read(salt)
	key := h.derive([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verify compara en tiempo constante usando los parámetros guardados en el hash.
func (h *Argon2Hasher) Verify(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false, errBadHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errBadHash
	}
	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil ||
		time < 1 || time > maxTime || threads < 1 || memory > maxMemory {
		return false, errBadHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < minSaltLen {
		return false, errBadHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) < minKeyLen {
		return false, errBadHash
	}
	got := h.derive([]byte(password), salt, time, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

func (h *Argon2Hasher) derive(password, salt []byte, time, memory uint32, threads uint8, keyLen uint32) []byte {
	h.sem <- struct{}{}
	defer func() { <-h.sem }()
	return h.key(password, salt, time, memory, threads, keyLen)
}
