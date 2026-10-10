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
	// Validate devuelve el paso del código, para no aceptarlo dos veces.
	want := at.Unix() / 30
	if step, ok := tp.Validate(code, secret, at); !ok || step != want {
		t.Fatalf("el código del momento: step=%d ok=%v, esperaba %d", step, ok, want)
	}
	// Se tolera un período de 30 s de desfase, no más; el paso es el del código.
	for _, d := range []time.Duration{30 * time.Second, -30 * time.Second} {
		if step, ok := tp.Validate(code, secret, at.Add(d)); !ok || step != want {
			t.Fatalf("con %v de desfase: step=%d ok=%v", d, step, ok)
		}
	}
	if _, ok := tp.Validate(code, secret, at.Add(90*time.Second)); ok {
		t.Fatal("tres períodos después ya no es válido")
	}
	next, _ := totp.GenerateCode(secret, at.Add(30*time.Second))
	if step, ok := tp.Validate(next, secret, at.Add(30*time.Second)); !ok || step != want+1 {
		t.Fatalf("el código siguiente: step=%d ok=%v", step, ok)
	}
	if _, ok := tp.Validate(wrongCode(t, secret, at), secret, at); ok {
		t.Fatal("un código incorrecto no es válido")
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef", " " + code[1:]} {
		if _, ok := tp.Validate(bad, secret, at); ok {
			t.Errorf("%q no debe ser válido", bad)
		}
	}
	if _, ok := tp.Validate(code, "no-es-base32!", at); ok {
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
