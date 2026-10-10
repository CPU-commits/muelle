package utils

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"log/slog"
	"net/http"
	"slices"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// internalBody es la respuesta cuando ni siquiera se puede serializar la respuesta.
const internalBody = `{"error":{"code":"internal","message":"error interno"}}` + "\n"

// WriteJSON responde v como JSON (json/v2, con las claves de los map ordenadas).
// Los bytes de UTF-8 inválido en un string salen como U+FFFD. Si v no se puede
// serializar responde 500, en vez de una respuesta a medias, y lo registra con
// slog.Default(); lo que responde el propio Kit usa el logger del Kit.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	writeJSON(slog.Default(), w, status, v)
}

// WriteError usa el formato del spec: {"error": {"code": "...", "message": "..."}}.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

func writeJSON(log *slog.Logger, w http.ResponseWriter, status int, v any) {
	body, err := jsonv2.Marshal(v, jsonv2.Deterministic(true), jsontext.AllowInvalidUTF8(true))
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		log.Error("respuesta JSON", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(internalBody))
		return
	}
	w.WriteHeader(status)
	w.Write(append(body, '\n'))
}

// writeError es WriteError con el logger del Kit.
func (k *Kit) writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(k.log, w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

// RegisterError asocia un error (comparado con errors.Is) a un status, código y mensaje.
// Si varios errores registrados coinciden, gana el último registrado: los
// módulos registran después que los comunes, así que sus errores más
// específicos tienen prioridad.
func (k *Kit) RegisterError(target error, status int, code, message string) {
	if target == nil {
		panic("utils: RegisterError con un error nil")
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.errors = append(k.errors, registeredError{target: target, status: status, code: code, message: message})
}

// Fail responde el error: *validate.Error → 422 "invalid" con ve.Error();
// un error registrado → su status; cualquier otro → 500 "internal" y log.Error.
func (k *Kit) Fail(w http.ResponseWriter, err error) {
	if ve, ok := errors.AsType[*validate.Error](err); ok {
		k.writeError(w, http.StatusUnprocessableEntity, "invalid", ve.Error())
		return
	}
	if e, ok := k.lookup(err); ok {
		k.writeError(w, e.status, e.code, e.message)
		return
	}
	k.log.Error("error interno", "err", err)
	k.writeError(w, http.StatusInternalServerError, "internal", "error interno")
}

// lookup busca el último error registrado que coincide con err.
func (k *Kit) lookup(err error) (registeredError, bool) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	for _, e := range slices.Backward(k.errors) {
		if errors.Is(err, e.target) {
			return e, true
		}
	}
	return registeredError{}, false
}
