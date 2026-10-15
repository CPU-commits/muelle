package service

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// ErrNotStageNetwork: ya existe una red de Docker con el nombre de la red del
// stage (proyecto_stage) que no es overlay attachable. No es de muelle: no se
// usa ni se borra. Se responde 409 network_conflict.
var ErrNotStageNetwork = errors.New("ya existe una red de Docker con el nombre del stage que no es overlay attachable; bórrala o renómbrala")

var (
	// ErrNothingToAdopt: adopt sobre un dominio que no está en error con
	// record_exists (409 nothing_to_adopt).
	ErrNothingToAdopt = errors.New("no hay un registro que adoptar")
	// ErrHostnameTaken: el hostname ya lo usa un dominio de muelle (409 conflict).
	ErrHostnameTaken = fmt.Errorf("ese hostname ya está en uso en muelle: %w", repository.ErrConflict)
	// ErrDNSNotFound: Cloudflare respondió 404 (el registro o la zona no existen).
	ErrDNSNotFound = errors.New("Cloudflare: no existe")
)

// DNSError es un pedido que Cloudflare rechazó (un 4xx que no es 404 ni 429,
// o success: false). Message es el de Cloudflare, sin el token.
type DNSError struct {
	Status, Code int
	Message      string
}

func (e *DNSError) Error() string {
	return "Cloudflare rechazó el pedido (" + strconv.Itoa(e.Status) + "): " + e.Message
}

// ErrCaddyUnreachable: no se pudo hablar con la admin API, o con el TLS de Caddy.
var ErrCaddyUnreachable = errors.New("muelle no pudo conectarse con Caddy")

// CaddyRejected: Caddy no aceptó la configuración (POST /load respondió con
// error) y conserva la anterior. Message ya viene sin el token (***) y
// cortado a 500 caracteres.
type CaddyRejected struct{ Message string }

func (e *CaddyRejected) Error() string { return "Caddy no aceptó la configuración: " + e.Message }

// ErrPlaceholderInSettings: el email ACME o el token de Cloudflare tienen { o },
// que Caddy expandiría como placeholders (D16 del plan de la Fase 3). No se
// manda nada.
var ErrPlaceholderInSettings = errors.New("el email ACME o el token de Cloudflare tienen { o }, que Caddy tomaría como placeholders")
