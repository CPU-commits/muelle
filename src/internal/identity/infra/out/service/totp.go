package service

import (
	"crypto/subtle"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
)

// totpIssuer es el nombre que muestran las apps de autenticación.
const totpIssuer = "muelle"

// totpOpts: códigos de 6 dígitos con SHA-1 cada 30 s (RFC 6238, lo que usan
// todas las apps de autenticación).
var totpOpts = totp.ValidateOpts{
	Period: uint(service.TOTPPeriod / time.Second), Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
}

// TOTP implementa service.TOTP con github.com/pquerna/otp, tolerando un
// período de desfase del reloj.
type TOTP struct{}

var _ service.TOTP = TOTP{}

func NewTOTP() TOTP { return TOTP{} }

func (TOTP) Generate(accountName string) (secret, url string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: totpIssuer, AccountName: accountName})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// Validate compara code con el del paso de at y los vecinos (±1) y devuelve
// el paso que coincide. pquerna/otp no lo informa, así que se calcula cada
// código con GenerateCodeCustom y se compara en tiempo constante.
func (TOTP) Validate(code, secret string, at time.Time) (int64, bool) {
	if len(code) != totpOpts.Digits.Length() {
		return 0, false
	}
	current := service.TOTPStep(at)
	for _, step := range []int64{current, current - 1, current + 1} {
		want, err := totp.GenerateCodeCustom(secret, time.Unix(step*int64(totpOpts.Period), 0), totpOpts)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}
