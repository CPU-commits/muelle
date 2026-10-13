package http

import (
	"net/http"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/service"
)

// registerErrors traduce los errores del dominio a respuestas HTTP.
// ErrWrongMasterKey no está: solo ocurre al arrancar, nunca en una petición.
func registerErrors(k *utils.Kit) {
	k.RegisterError(service.ErrTOTPRequiredFirst, http.StatusConflict, "totp_required_first", service.ErrTOTPRequiredFirst.Error())
	k.RegisterError(service.ErrWouldLockOut, http.StatusConflict, "would_lock_out", service.ErrWouldLockOut.Error())
}
