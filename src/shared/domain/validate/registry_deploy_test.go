package validate

import (
	"errors"
	"strings"
	"testing"
)

// wantRegistryError exige un *Error en field cuyo mensaje contenga want (si
// no es "") y no contenga value (ningún mensaje de un registry lleva el
// valor: settings.Load los muestra al arrancar).
func wantRegistryError(t *testing.T, err error, field, want, value string) {
	t.Helper()
	ve, ok := errors.AsType[*Error](err)
	if !ok {
		t.Fatalf("%q: esperaba un *validate.Error, llegó %v", value, err)
	}
	if ve.Field != field {
		t.Errorf("%q: field %q, esperaba %q", value, ve.Field, field)
	}
	if want != "" && !strings.Contains(ve.Message, want) {
		t.Errorf("%q: mensaje %q, esperaba que contenga %q", value, ve.Message, want)
	}
	if value != "" && strings.Contains(ve.Message, value) {
		t.Errorf("el mensaje %q trae el valor %q", ve.Message, value)
	}
}

// El host de una credencial es el que da reference.Domain para una imagen
// (contrato §2.10), en su forma canónica y sin normalizar.
func TestRegistryHost(t *testing.T) {
	good := []string{
		"ghcr.io", "docker.io", "registry.example.com:5000", "localhost:5000", "localhost",
		"a", "my-registry.internal", "10.0.0.1:5000", "r.io:1", "r.io:65535",
		strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 61), // 253
	}
	for _, s := range good {
		if err := RegistryHost("host", s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	bad := []string{
		"", "GHCR.io", "https://ghcr.io", "ghcr.io/", "ghcr.io/x", "ghcr.io:0", "ghcr.io:70000",
		"ghcr.io:", "ghcr.io:123456", "-x.io", "x-.io", "a..b", ".ghcr.io", "ghcr.io.", "ghcr io",
		"ghcr_io", "ghcr.io:50a", "[::1]:5000", "ñ.io", "ghcr.io\n",
		strings.Repeat("a.", 127) + "io", // 256
	}
	for _, s := range bad {
		err := RegistryHost("host", s)
		if err == nil {
			t.Errorf("%q: debió fallar", s)
			continue
		}
		wantRegistryError(t, err, "host", "", "")
	}
	// Los alias de Docker Hub: una imagen de Docker Hub siempre da docker.io,
	// así que una credencial con otro host nunca se usaría.
	for _, s := range []string{"index.docker.io", "registry-1.docker.io", "registry.hub.docker.com"} {
		err := RegistryHost("host", s)
		if err == nil {
			t.Errorf("%q: debió fallar", s)
			continue
		}
		if err.Error() != "host: para Docker Hub usa docker.io" {
			t.Errorf("%q: mensaje %q", s, err.Error())
		}
	}
	// El campo es el que pasa quien llama (settings.Load usa el suyo).
	wantRegistryError(t, RegistryHost("MUELLE_INIT_REGISTRY_FILE.host", "GHCR.io"), "MUELLE_INIT_REGISTRY_FILE.host", "", "")
}

func TestRegistryUsername(t *testing.T) {
	for _, s := range []string{"bot", "x", "CPU-commits", "user@example.com", "ñandú", strings.Repeat("u", 255), strings.Repeat("ñ", 255)} {
		if err := RegistryUsername("username", s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	const secretish = "usuario-que-no-sale"
	for _, s := range []string{
		"", strings.Repeat("u", 256), secretish + "\x00", secretish + "\n", secretish + "\x7f", secretish + "\u0085",
		secretish + "\t", secretish + ":x", "\xff" + secretish,
	} {
		err := RegistryUsername("username", s)
		if err == nil {
			t.Errorf("%q: debió fallar", s)
			continue
		}
		wantRegistryError(t, err, "username", "", secretish)
	}
}

func TestRegistryPassword(t *testing.T) {
	for _, s := range []string{"x", "ghp_" + strings.Repeat("A", 36), "con espacios y ñ", "con\ttab", strings.Repeat("p", 4096)} {
		if err := RegistryPassword("password", s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	const secret = "ghp_contraseña-que-nunca-sale"
	for _, s := range []string{"", strings.Repeat("p", 4097), secret + "\n", secret + "\r", "\n" + secret, secret + "\r\n"} {
		err := RegistryPassword("password", s)
		if err == nil {
			t.Errorf("%q: debió fallar", s)
			continue
		}
		wantRegistryError(t, err, "password", "", secret)
		if strings.Contains(err.Error(), secret) {
			t.Errorf("el error %q trae la contraseña", err.Error())
		}
	}
}
