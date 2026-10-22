package utils

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// Classify da el código y el mensaje que daría Fail, sin escribir nada: el
// servidor MCP responde los mismos que REST (contrato §8.4).
func TestClassify(t *testing.T) {
	k, _ := newTestKit()
	errLocal := errors.New("local")
	k.RegisterError(errLocal, http.StatusConflict, "local_conflict", "un conflicto local")
	cases := []struct {
		name      string
		err       error
		code, msg string
		ok        bool
	}{
		{"validación", validate.Errorf("since", "no es una fecha"), "invalid", "since: no es una fecha", true},
		{"validación envuelta", fmt.Errorf("x: %w", validate.Errorf("offset", "pasa el largo")), "invalid", "offset: pasa el largo", true},
		{"registrado", errBadToken, "bad_token", "el token no vale", true},
		{"registrado envuelto", fmt.Errorf("y: %w", errLocal), "local_conflict", "un conflicto local", true},
		{"no registrado", errors.New("otro"), "", "", false},
		{"nil", nil, "", "", false},
	}
	for _, c := range cases {
		code, msg, ok := k.Classify(c.err)
		if code != c.code || msg != c.msg || ok != c.ok {
			t.Errorf("%s: Classify = %q %q %v, esperaba %q %q %v", c.name, code, msg, ok, c.code, c.msg, c.ok)
		}
	}
}
