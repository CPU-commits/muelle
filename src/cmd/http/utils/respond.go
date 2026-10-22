package utils

import (
	"context"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RateLimitRetryAfter es el Retry-After de toda respuesta 429. Los errores
// todavía no traen cuánto falta para que se libere el límite, así que es fijo.
const RateLimitRetryAfter = 60 * time.Second

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

// writeError es WriteError con el logger del Kit. Un 429 lleva Retry-After
// (RateLimitRetryAfter, en segundos).
func (k *Kit) writeError(w http.ResponseWriter, status int, code, message string) {
	if status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", strconv.Itoa(int(RateLimitRetryAfter/time.Second)))
	}
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

// Fail responde el error de la petición r: *validate.Error → 422 "invalid"
// con ve.Error(); un error registrado → su status (un 429, con Retry-After);
// cualquier otro → 500 "internal" y log.Error con el método y la ruta de r.
// Un context.Canceled con el contexto de r cancelado es un cliente que se fue
// (el panel cambió de pestaña con un pedido en curso): responde lo mismo, que
// nadie lee, pero va al log en Debug, no como error interno.
func (k *Kit) Fail(w http.ResponseWriter, r *http.Request, err error) {
	if ve, ok := errors.AsType[*validate.Error](err); ok {
		k.writeError(w, http.StatusUnprocessableEntity, "invalid", ve.Error())
		return
	}
	if e, ok := k.lookup(err); ok {
		k.writeError(w, e.status, e.code, e.message)
		return
	}
	if errors.Is(err, context.Canceled) && r.Context().Err() != nil {
		k.log.Debug("el cliente se fue", "method", r.Method, "path", logPath(r))
	} else {
		k.log.Error("error interno", "method", r.Method, "path", logPath(r), "err", err)
	}
	k.writeError(w, http.StatusInternalServerError, "internal", "error interno")
}

// Classify da el código y el mensaje con que Fail respondería err, sin
// escribir nada: *validate.Error es "invalid" con ve.Error(), y un error
// registrado, su código y su mensaje. ok es false para cualquier otro (el
// "internal" de Fail), y para nil. El servidor MCP lo usa para responder los
// mismos errores que REST (contrato §8.4).
func (k *Kit) Classify(err error) (code, message string, ok bool) {
	if err == nil {
		return "", "", false
	}
	if ve, isVE := errors.AsType[*validate.Error](err); isVE {
		return "invalid", ve.Error(), true
	}
	if e, found := k.lookup(err); found {
		return e.code, e.message, true
	}
	return "", "", false
}

// logPath es la ruta de r para el log, sin secretos: el valor del comodín
// {token} (el link de una invitación lo lleva en la ruta) se reemplaza por
// "{token}".
func logPath(r *http.Request) string {
	if tok := r.PathValue("token"); tok != "" {
		return strings.ReplaceAll(r.URL.Path, tok, "{token}")
	}
	return r.URL.Path
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
