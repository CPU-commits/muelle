package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestTOTPGenerateAndValidate(t *testing.T) {
	tp := NewTOTP()
	secret, url, err := tp.Generate("felipe")
	if err != nil || secret == "" {
		t.Fatalf("secret=%q err=%v", secret, err)
	}
	if !strings.HasPrefix(url, "otpauth://totp/muelle:felipe?") || !strings.Contains(url, "secret="+secret) {
		t.Fatalf("url=%s", url)
	}
	other, _, _ := tp.Generate("felipe")
	if other == secret {
		t.Fatal("secretos repetidos")
	}

	at := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	code, err := totp.GenerateCode(secret, at)
	if err != nil {
		t.Fatal(err)
	}
	if !tp.Validate(code, secret, at) {
		t.Fatal("el código del momento debe ser válido")
	}
	// Se tolera un período de 30 s de desfase, no más.
	if !tp.Validate(code, secret, at.Add(30*time.Second)) || !tp.Validate(code, secret, at.Add(-30*time.Second)) {
		t.Fatal("con un período de desfase debe ser válido")
	}
	if tp.Validate(code, secret, at.Add(90*time.Second)) {
		t.Fatal("tres períodos después ya no es válido")
	}
	if tp.Validate(wrongCode(t, secret, at), secret, at) {
		t.Fatal("un código incorrecto no es válido")
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef"} {
		if tp.Validate(bad, secret, at) {
			t.Errorf("%q no debe ser válido", bad)
		}
	}
	if tp.Validate(code, "no-es-base32!", at) {
		t.Fatal("con un secreto inválido no es válido")
	}
}

// wrongCode devuelve un código de 6 dígitos que no es válido en at (ni en los
// períodos vecinos).
func wrongCode(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	valid := map[string]bool{}
	for _, d := range []time.Duration{-30 * time.Second, 0, 30 * time.Second} {
		c, _ := totp.GenerateCode(secret, at.Add(d))
		valid[c] = true
	}
	for i := 0; ; i++ {
		if c := fmt.Sprintf("%06d", i); !valid[c] {
			return c
		}
	}
}
