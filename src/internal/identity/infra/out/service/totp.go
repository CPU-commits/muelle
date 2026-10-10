package service

import (
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
)

// totpIssuer es el nombre que muestran las apps de autenticación.
const totpIssuer = "muelle"

// TOTP implementa service.TOTP con github.com/pquerna/otp: códigos de 6
// dígitos con SHA-1 cada 30 s, tolerando un período de desfase.
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

func (TOTP) Validate(code, secret string, at time.Time) bool {
	ok, err := totp.ValidateCustom(code, secret, at, totp.ValidateOpts{
		Period: 30, Skew: 1, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && ok
}
