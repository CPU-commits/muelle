package http

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
)

// decodeEmptyBody acepta un cuerpo vacío (o solo espacios) o {} (contrato
// §3.2: adopt y retry, como el rollback de §2.7). Cualquier otra cosa (null,
// [], un campo) responde 400 invalid_json y devuelve false.
func decodeEmptyBody(w http.ResponseWriter, r *http.Request) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, utils.MaxBodyBytes))
	if err != nil {
		msg := "JSON inválido"
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			msg = "el cuerpo supera 1 MiB"
		}
		utils.WriteError(w, http.StatusBadRequest, "invalid_json", msg)
		return false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return true
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return utils.DecodeJSON(w, r, &struct{}{})
}
