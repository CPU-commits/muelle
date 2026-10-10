package utils

import (
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
// Si falla responde 400 "invalid_json" con un mensaje en español sin detalles internos de Go, y devuelve false.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	// json/v2 ya rechaza por defecto las claves duplicadas y el UTF-8 inválido,
	// y UnmarshalRead exige un único valor hasta el final del body.
	err := jsonv2.UnmarshalRead(http.MaxBytesReader(w, r.Body, MaxBodyBytes), dst, jsonv2.RejectUnknownMembers(true))
	if err != nil {
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
