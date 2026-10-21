package compose

import (
	"errors"
	"strings"

	"github.com/compose-spec/compose-go/v2/template"
)

// UndefinedError es una variable sin definir y sin valor por defecto (§5.2
// regla 3 del diseño): ${X} o $X.
type UndefinedError struct{ Name string }

func (e *UndefinedError) Error() string {
	return "la variable " + e.Name + " no está definida: agrégala en las variables de la app o del stage, o usa ${" + e.Name + ":-valor}"
}

// RequiredError es ${X:?motivo} (o ${X?motivo}) sin la variable. Reason es el
// texto del compose, nunca un valor.
type RequiredError struct{ Name, Reason string }

func (e *RequiredError) Error() string {
	if e.Reason == "" {
		return "falta la variable " + e.Name
	}
	return "falta la variable " + e.Name + ": " + e.Reason
}

// TemplateError es una variable mal escrita, o un $ dentro del valor de un
// ${X<op>valor}. Reason lleva el pedazo del compose, nunca un valor.
//
// Ninguno de los tres envuelve *template.InvalidTemplateError: si lo hiciera,
// compose-go usaría otro formato para el mensaje y interpolationError no
// encontraría la ruta.
type TemplateError struct{ Reason string }

func (e *TemplateError) Error() string { return e.Reason }

// strictSubstitute reemplaza las variables de s con m (§5.2 regla 3, §6.3).
// Es la Interpolate.Substitute del loader. Acepta lo mismo que compose-go:
// $$ (un $ literal), $NAME, ${NAME} y ${NAME<op>valor} con op en :- - :+ + :? ?
// (NAME es [_a-zA-Z][_a-zA-Z0-9]*); un $ seguido de otra cosa ($1, un $ al
// final) queda literal. El cierre de ${…} se busca contando llaves, como
// getFirstBraceClosingIndex de compose-go (no es exportada).
// Diferencias, a propósito (compose-go las resuelve sin error):
//   - una variable sin definir y sin op es *UndefinedError, también si viene
//     después de otra con op en el mismo string (${A:-x}-${B});
//   - un $ dentro del valor de un op es *TemplateError, aunque la variable
//     exista: ${A:-${B}} no se resuelve (compose-go lo haría sin ser estricto).
//
// :? y ? sin valor dan *RequiredError con el mensaje del usuario; ${}, ${A B},
// ${1A} y un ${ sin cerrar, *TemplateError. Nunca escribe con logrus.
func strictSubstitute(s string, m template.Mapping) (string, error) {
	return substitute(s, m)
}

// checkTemplate es strictSubstitute sin buscar valores (Validate): solo los
// *TemplateError.
func checkTemplate(s string) error {
	_, err := substitute(s, nil)
	return err
}

// substitute hace el trabajo de los dos: con m nil no busca valores (toda
// variable vale "" y ninguna falta).
func substitute(s string, m template.Mapping) (string, error) {
	if !strings.Contains(s, "$") {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '$' {
			b.WriteByte(s[i])
			i++
			continue
		}
		if i+1 == len(s) {
			b.WriteByte('$')
			break
		}
		switch c := s[i+1]; {
		case c == '$':
			b.WriteByte('$')
			i += 2
		case isNameStart(c):
			j := i + 2
			for j < len(s) && isNameChar(s[j]) {
				j++
			}
			v, err := lookup(m, s[i+1:j])
			if err != nil {
				return "", err
			}
			b.WriteString(v)
			i = j
		case c == '{':
			end := closingBrace(s[i:])
			if end < 0 {
				return "", &TemplateError{Reason: "falta la } que cierra la variable: " + excerpt(s[i:])}
			}
			v, err := braced(s[i+2:i+end], m)
			if err != nil {
				return "", err
			}
			b.WriteString(v)
			i += end + 1
		default:
			b.WriteByte('$')
			i++
		}
	}
	return b.String(), nil
}

// ops son los modificadores de ${NAME<op>valor}, los de dos caracteres antes
// que los de uno.
var ops = []string{":-", ":+", ":?", "-", "+", "?"}

// braced resuelve lo de adentro de ${…}.
func braced(expr string, m template.Mapping) (string, error) {
	n := 0
	if len(expr) > 0 && isNameStart(expr[0]) {
		n = 1
		for n < len(expr) && isNameChar(expr[n]) {
			n++
		}
	}
	if n == 0 {
		return "", &TemplateError{Reason: "variable mal escrita: ${" + excerpt(expr) + "}"}
	}
	name, rest := expr[:n], expr[n:]
	if rest == "" {
		return lookup(m, name)
	}
	op := ""
	for _, o := range ops {
		if strings.HasPrefix(rest, o) {
			op = o
			break
		}
	}
	if op == "" {
		return "", &TemplateError{Reason: "variable mal escrita: ${" + excerpt(expr) + "}"}
	}
	val := rest[len(op):]
	if strings.Contains(val, "$") {
		return "", &TemplateError{Reason: "un valor por defecto no puede tener $: ${" + excerpt(expr) + "}"}
	}
	if m == nil {
		return "", nil
	}
	v, set := m(name)
	empty := !set || v == ""
	switch op {
	case ":-":
		if empty {
			return val, nil
		}
	case "-":
		if !set {
			return val, nil
		}
	case ":+":
		if !empty {
			return val, nil
		}
		return "", nil
	case "+":
		if set {
			return val, nil
		}
		return "", nil
	case ":?":
		if empty {
			return "", &RequiredError{Name: name, Reason: val}
		}
	case "?":
		if !set {
			return "", &RequiredError{Name: name, Reason: val}
		}
	}
	return v, nil
}

func lookup(m template.Mapping, name string) (string, error) {
	if m == nil {
		return "", nil
	}
	v, ok := m(name)
	if !ok {
		return "", &UndefinedError{Name: name}
	}
	return v, nil
}

// closingBrace es el índice de la } que cierra el ${ con que empieza s, o -1.
// Cuenta llaves, como getFirstBraceClosingIndex de compose-go, así que
// ${A:-{"x":1}} cierra en la última. (compose-go además salta el carácter que
// sigue a cada {; en un ${…} válido ese carácter nunca es una llave, así que
// el resultado es el mismo, y ${} da un mensaje más claro.)
func closingBrace(s string) int {
	open := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			open++
		case '}':
			open--
			if open == 0 {
				return i
			}
		}
	}
	return -1
}

func isNameStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isNameChar(c byte) bool { return isNameStart(c) || c >= '0' && c <= '9' }

// excerpt corta un pedazo del compose para un mensaje.
func excerpt(s string) string {
	const max = 64
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && s[cut]&0xC0 == 0x80 { // no partir un carácter UTF-8
		cut--
	}
	return s[:cut] + "…"
}

// interpolationError busca, en el árbol de err que devolvió compose-go, el
// primer error de strictSubstitute (en orden: compose-go los junta con
// errors.Join, ordenados por ruta) y la ruta donde ocurrió. compose-go lo
// envuelve en un tipo que no exporta, cuyo mensaje es "error while
// interpolating <ruta>: <nuestro error>": la ruta sale de ese texto, y ok es
// false si no tiene esa forma.
func interpolationError(err error) (path string, inner error, ok bool) {
	var node error
	var walk func(e error) bool
	walk = func(e error) bool {
		if e == nil {
			return false
		}
		if u := errors.Unwrap(e); u != nil && isSubstituteError(u) {
			node, inner = e, u
			return true
		}
		switch x := e.(type) {
		case interface{ Unwrap() []error }:
			for _, c := range x.Unwrap() {
				if walk(c) {
					return true
				}
			}
		case interface{ Unwrap() error }:
			return walk(x.Unwrap())
		}
		return false
	}
	if !walk(err) {
		return "", nil, false
	}
	rest, ok1 := strings.CutPrefix(node.Error(), "error while interpolating ")
	path, ok2 := strings.CutSuffix(rest, ": "+inner.Error())
	if !ok1 || !ok2 || path == "" {
		return "", nil, false
	}
	return path, inner, true
}

func isSubstituteError(err error) bool {
	switch err.(type) {
	case *UndefinedError, *RequiredError, *TemplateError:
		return true
	}
	return false
}
