package http

import (
	"net/http"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
)

// registerErrors asocia los errores de identity a sus respuestas HTTP. Solo
// registra los errores propios: los comunes (ErrNotFound, ErrConflict,
// ErrUnauthenticated…) los registra cmd/http, y el Kit es compartido.
func registerErrors(k *utils.Kit) {
	k.RegisterError(service.ErrInviteInvalid, http.StatusBadRequest, "invalid_invite", "la invitación no existe, ya se usó o venció")
	k.RegisterError(service.ErrInvalidCredentials, http.StatusUnauthorized, "invalid_credentials", "usuario o contraseña incorrectos")
	k.RegisterError(service.ErrLocked, http.StatusLocked, "locked", "usuario bloqueado temporalmente por intentos fallidos")
	k.RegisterError(service.ErrRateLimited, http.StatusTooManyRequests, "rate_limited", "demasiados intentos, espera unos minutos")
	k.RegisterError(service.ErrInvalidCode, http.StatusUnauthorized, "invalid_code", "código incorrecto")
	k.RegisterError(service.ErrNoPendingTOTP, http.StatusConflict, "no_pending_totp", "primero inicia la configuración de 2FA")
	k.RegisterError(service.ErrAlreadySetUp, http.StatusConflict, "already_set_up", "muelle ya tiene usuarios; usa una invitación")
	k.RegisterError(service.ErrCannotDeleteSelf, http.StatusConflict, "cannot_delete_self", "no puedes borrar tu propio usuario")
}
