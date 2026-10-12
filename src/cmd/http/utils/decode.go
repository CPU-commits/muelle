package utils

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
)

// DecodeJSON lee el body con json/v2 estricto (máx. MaxBodyBytes): rechaza campos
// desconocidos, claves duplicadas, UTF-8 inválido y datos después del valor.
// También rechaza un body que no es un objeto JSON (null, [], "x", 1…): todos
// los cuerpos de la API son objetos, y un null dejaría dst sin tocar.
// Si falla responde 400 "invalid_json" con un mensaje en español sin detalles internos de Go, y devuelve false.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", decodeMessage(err))
		return false
	}
	// Un body vacío sigue de largo para que json/v2 dé el error de siempre.
	if trimmed := bytes.TrimLeft(body, " \t\r\n"); len(trimmed) > 0 && trimmed[0] != '{' {
		WriteError(w, http.StatusBadRequest, "invalid_json", "JSON inválido: el cuerpo debe ser un objeto")
		return false
	}
	// json/v2 ya rechaza por defecto las claves duplicadas y el UTF-8 inválido,
	// y Unmarshal exige un único valor hasta el final del body.
	if err := jsonv2.Unmarshal(body, dst, jsonv2.RejectUnknownMembers(true)); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", decodeMessage(err))
		return false
	}
	return true
}

// decodeMessage traduce un error de json/v2 a un mensaje para el cliente. Solo
// nombra el campo del propio JSON; nunca tipos ni mensajes internos de Go.
func decodeMessage(err error) string {
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return "el cuerpo supera 1 MiB"
	}
	if se, ok := errors.AsType[*jsonv2.SemanticError](err); ok {
		field := fieldName(se.JSONPointer)
		switch {
		case errors.Is(se.Err, jsonv2.ErrUnknownName):
			return fmt.Sprintf("JSON inválido: el campo %q no existe", field)
		case errors.Is(se.Err, io.ErrUnexpectedEOF):
			return "JSON inválido: el cuerpo está vacío o incompleto"
		case field == "":
			return "JSON inválido: el cuerpo no tiene la forma esperada"
		default:
			return fmt.Sprintf("JSON inválido: el campo %q tiene un tipo de valor incorrecto", field)
		}
	}
	if se, ok := errors.AsType[*jsontext.SyntacticError](err); ok {
		switch {
		case errors.Is(se.Err, jsontext.ErrDuplicateName):
			return fmt.Sprintf("JSON inválido: el campo %q está repetido", fieldName(se.JSONPointer))
		case errors.Is(se.Err, io.ErrUnexpectedEOF):
			return "JSON inválido: el cuerpo está vacío o incompleto"
		}
		return fmt.Sprintf("JSON inválido cerca del byte %d", se.ByteOffset)
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "JSON inválido: el cuerpo está vacío o incompleto"
	}
	return "JSON inválido"
}

// fieldName convierte un JSON Pointer (/healthcheck/path) a la forma de los
// errores de validación (healthcheck.path).
func fieldName(p jsontext.Pointer) string {
	return strings.Join(slices.Collect(p.Tokens()), ".")
}
