// Package validate contiene las reglas de nombres y el tipo de error de
// validación que la API traduce a 422.
package validate

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Error es un error de validación de un campo concreto.
type Error struct {
	Field   string
	Message string
}

func (e *Error) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return e.Field + ": " + e.Message
}

func Errorf(field, format string, args ...any) error {
	return &Error{Field: field, Message: fmt.Sprintf(format, args...)}
}

var (
	// Empiezan con letra, terminan con letra o número, solo [a-z0-9-].
	// El "_" queda libre como separador en los nombres de Swarm (§5.2 del spec).
	slug16Re   = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,14}[a-z0-9])?$`)
	slug12Re   = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,10}[a-z0-9])?$`)
	usernameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,31}$`)
	envKeyRe   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
)

// reservedProjects no se pueden usar como proyecto: la red de un stage se
// llama <proyecto>_<stage> y chocaría con muelle_panel o docker_gwbridge (§5.2 del spec).
var reservedProjects = map[string]bool{"muelle": true, "docker": true}

func ProjectSlug(s string) error {
	if err := slug("slug", s, slug16Re, 16); err != nil {
		return err
	}
	if reservedProjects[s] {
		return Errorf("slug", "%q está reservado", s)
	}
	return nil
}

func StageSlug(s string) error { return slug("slug", s, slug12Re, 12) }
func AppSlug(s string) error   { return slug("slug", s, slug16Re, 16) }

// VolumeName valida nombres de volúmenes y de servicios compose (mismas reglas que una app).
func VolumeName(field, s string) error { return slug(field, s, slug16Re, 16) }

func slug(field, s string, re *regexp.Regexp, max int) error {
	if !re.MatchString(s) {
		return Errorf(field, "%q no es válido: hasta %d caracteres, solo a-z, 0-9 y '-', empieza con letra y no termina en '-'", s, max)
	}
	return nil
}

func Username(s string) error {
	if !usernameRe.MatchString(s) {
		return Errorf("username", "entre 3 y 32 caracteres: a-z, 0-9, '.', '_' o '-'")
	}
	return nil
}

func Password(s string) error {
	n := utf8.RuneCountInString(s)
	if n < 12 {
		return Errorf("password", "debe tener al menos 12 caracteres")
	}
	if len(s) > 256 {
		return Errorf("password", "no puede superar 256 bytes")
	}
	return nil
}

// EnvKey valida la clave de una variable. field es el campo del cuerpo donde
// viene la clave ("vars", "set" o "unset"), para que el error lo nombre.
func EnvKey(field, key string) error {
	if !envKeyRe.MatchString(key) {
		return Errorf(field, "clave %q inválida: letras, números y '_', sin empezar con número, hasta 128", key)
	}
	// compose-go lee del entorno las variables COMPOSE_* (§5.2 del spec).
	if strings.HasPrefix(key, "COMPOSE_") {
		return Errorf(field, "clave %q inválida: el prefijo COMPOSE_ está reservado", key)
	}
	return nil
}

// Name valida un texto libre obligatorio (nombres de proyectos, tokens).
// Rechaza caracteres de control y de formato (invisibles, como U+200B o U+202E).
func Name(field, s string, max int) error {
	if strings.TrimSpace(s) == "" {
		return Errorf(field, "es obligatorio")
	}
	if utf8.RuneCountInString(s) > max {
		return Errorf(field, "no puede superar %d caracteres", max)
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return Errorf(field, "no puede contener caracteres de control ni invisibles")
		}
	}
	return nil
}
