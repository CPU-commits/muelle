package service

import (
	"strings"
	"testing"
)

// El más largo gana: si "abcdefgh" fuera primero, de "abcdefgh12" quedarían
// los "12" a la vista.
func TestMaskerOrder(t *testing.T) {
	for _, values := range [][]string{{"abcdefgh", "abcdefgh12"}, {"abcdefgh12", "abcdefgh"}} {
		m := NewMasker(values)
		if got := m.Mask("x=abcdefgh12 y=abcdefgh"); got != "x=*** y=***" {
			t.Errorf("%v: %q", values, got)
		}
	}
}

// Un valor de menos de 8 bytes no se enmascara: taparía texto que no tiene
// nada que ver (contrato §2.5).
func TestMaskerShortValues(t *testing.T) {
	m := NewMasker([]string{"1", "true", "3000", "1234567", "", "12345678"})
	in := "replicas 1 true en el puerto 3000: 1234567 y 12345678"
	if got := m.Mask(in); got != "replicas 1 true en el puerto 3000: 1234567 y ***" {
		t.Fatalf("%q", got)
	}
}

// Un valor de varias líneas se enmascara entero y, además, cada línea (sin
// los espacios de los bordes) de 8 bytes o más: el log se procesa por línea.
func TestMaskerMultiline(t *testing.T) {
	pem := "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASC\n  bG9uZ2xpbmVvZmJhc2U2NA==  \r\nab\n-----END PRIVATE KEY-----\n"
	m := NewMasker([]string{pem})
	if got := m.Mask("clave: " + pem + "fin"); got != "clave: ***fin" {
		t.Errorf("entero: %q", got)
	}
	for line, want := range map[string]string{
		"leí MIIEvQIBADANBgkqhkiG9w0BAQEFAASC del archivo": "leí *** del archivo",
		"bG9uZ2xpbmVvZmJhc2U2NA==":                         "***",
		"-----END PRIVATE KEY-----":                        "***",
		// Una línea de 2 bytes no se enmascara.
		"ab cd": "ab cd",
	} {
		if got := m.Mask(line); got != want {
			t.Errorf("Mask(%q) = %q, esperaba %q", line, got, want)
		}
	}
}

func TestMaskerEmpty(t *testing.T) {
	for _, m := range []Masker{{}, NewMasker(nil), NewMasker([]string{"", "corto"})} {
		if got := m.Mask("nada que tapar ***"); got != "nada que tapar ***" {
			t.Errorf("%q", got)
		}
	}
	// Los repetidos no cambian nada.
	m := NewMasker([]string{"secreto-largo", "secreto-largo"})
	if got := m.Mask("secreto-largo"); got != "***" {
		t.Errorf("%q", got)
	}
	if len(m.values) != 1 {
		t.Errorf("valores: %q", m.values)
	}
	if s := strings.Join(m.values, ""); s != "secreto-largo" {
		t.Errorf("valores: %q", s)
	}
}
