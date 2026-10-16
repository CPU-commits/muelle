package validate

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Las reglas de la credencial de un registry (contrato §2.10). Las usan deploy
// (el CRUD de /registries) y settings.Load (el registry inicial de install,
// MUELLE_INIT_REGISTRY_FILE), que no puede importar el dominio de un módulo.
// Ningún mensaje lleva el valor: los de settings.Load salen en el log de
// arranque, y no deben dar pistas de la credencial.

// registryHostRe es un host en su forma canónica: etiquetas [a-z0-9-] que no
// empiezan ni terminan en '-', separadas por '.', con ':puerto' opcional.
var registryHostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*(:[0-9]{1,5})?$`)

// dockerHubAliases tienen la forma canónica, pero reference.Domain da
// docker.io para toda imagen de Docker Hub (también index.docker.io/x): una
// credencial con uno de estos hosts nunca se usaría.
var dockerHubAliases = map[string]bool{"index.docker.io": true, "registry-1.docker.io": true, "registry.hub.docker.com": true}

// RegistryHost valida el host de un registry como lo da
// reference.Domain(reference.ParseNormalizedNamed(ref)): ghcr.io,
// registry.example.com:5000, localhost:5000 o docker.io. Sin normalizar: en
// minúsculas, sin esquema, ruta ni '/' final, con el puerto de 1 a 65535 y de
// hasta 255 caracteres.
func RegistryHost(field, s string) error {
	if s == "" || len(s) > 255 {
		return Errorf(field, "es obligatorio y de hasta 255 caracteres")
	}
	if !registryHostRe.MatchString(s) {
		return Errorf(field, "no es un host de registry válido: en minúsculas, sin esquema ni ruta (ej. ghcr.io o registry.example.com:5000)")
	}
	if _, port, ok := strings.Cut(s, ":"); ok {
		if n, _ := strconv.Atoi(port); n < 1 || n > 65535 {
			return Errorf(field, "el puerto debe estar entre 1 y 65535")
		}
	}
	if dockerHubAliases[s] {
		return Errorf(field, "para Docker Hub usa docker.io")
	}
	return nil
}

// RegistryUsername valida el usuario de un registry: de 1 a 255 caracteres,
// UTF-8 válido, sin caracteres de control ni ':' (la autenticación HTTP Basic
// separa el usuario de la contraseña en el primer ':', RFC 7617).
func RegistryUsername(field, s string) error {
	if s == "" || utf8.RuneCountInString(s) > 255 {
		return Errorf(field, "es obligatorio y de hasta 255 caracteres")
	}
	if !utf8.ValidString(s) {
		return Errorf(field, "no es UTF-8 válido")
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return Errorf(field, "no puede contener caracteres de control")
		}
		if r == ':' {
			return Errorf(field, "no puede contener ':'")
		}
	}
	return nil
}

// RegistryPassword valida la contraseña (o el token) de un registry: de 1 a
// 4096 bytes, sin saltos de línea.
func RegistryPassword(field, s string) error {
	if s == "" || len(s) > 4096 {
		return Errorf(field, "es obligatoria y de hasta 4096 bytes")
	}
	if strings.ContainsAny(s, "\n\r") {
		return Errorf(field, "no puede contener saltos de línea")
	}
	return nil
}
