package usecases

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

func completeFrom(pendingToken, code, ip string) dto.CompleteTOTPInput {
	return dto.CompleteTOTPInput{SessionHash: service.HashToken(pendingToken), Code: code, IP: ip}
}

// La sonda de la revisión: con la contraseña, un atacante abre una sesión
// pendiente y prueba códigos desde muchas IPs (cada una en su /64). Al 5º
// código malo la sesión se borra, y ni el código correcto sirve con ella.
func TestTOTPBruteForceStoppedPerSession(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	secret := f.enableTOTPFor(t, u.ID)
	res, err := f.login.Execute(ctx, login("felipe", testPassword, "203.0.113.9"))
	if err != nil || !res.MFARequired {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	wrong := f.wrongCode(t, secret)
	for i := range 5 {
		ip := fmt.Sprintf("2001:db8:%x::1", i+1)
		if _, err := f.completeTOTP.Execute(ctx, completeFrom(res.Token, wrong, ip)); !errors.Is(err, service.ErrInvalidCode) {
			t.Fatalf("código malo %d: got %v", i+1, err)
		}
	}
	if _, err := f.completeTOTP.Execute(ctx, completeFrom(res.Token, f.code(t, secret), "2001:db8:ff::1")); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("la sesión pendiente debió borrarse: got %v", err)
	}
	if n := f.countFullSessions(t); n != 0 {
		t.Fatalf("sesiones completas=%d", n)
	}
}

// Con muchas sesiones pendientes desde muchas IPs (pocos intentos en cada
// una), el contador por usuario corta igual: al 5º código malo el usuario
// queda bloqueado para el 2FA desde cualquier IP.
func TestTOTPBruteForceStoppedPerUser(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	secret := f.enableTOTPFor(t, u.ID)
	wrong := f.wrongCode(t, secret)
	for i := range 5 {
		ip := fmt.Sprintf("2001:db8:%x::1", i+1)
		res, err := f.login.Execute(ctx, login("felipe", testPassword, ip))
		if err != nil {
			t.Fatalf("login %d: %v", i+1, err)
		}
		if _, err := f.completeTOTP.Execute(ctx, completeFrom(res.Token, wrong, ip)); !errors.Is(err, service.ErrInvalidCode) {
			t.Fatalf("código malo %d: got %v", i+1, err)
		}
	}
	res, _ := f.login.Execute(ctx, login("felipe", testPassword, "198.51.100.7"))
	if _, err := f.completeTOTP.Execute(ctx, completeFrom(res.Token, f.code(t, secret), "198.51.100.7")); !errors.Is(err, service.ErrLocked) {
		t.Fatalf("el usuario queda bloqueado desde cualquier IP: got %v", err)
	}
}

// El usuario legítimo entra con su código cuando vence el bloqueo.
func TestTOTPLockExpiresForLegitimateUser(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	secret := f.enableTOTPFor(t, u.ID)
	attacker, _ := f.login.Execute(ctx, login("felipe", testPassword, "203.0.113.9"))
	wrong := f.wrongCode(t, secret)
	for i := range 5 {
		f.completeTOTP.Execute(ctx, completeFrom(attacker.Token, wrong, fmt.Sprintf("2001:db8:%x::1", i+1)))
	}

	res, err := f.login.Execute(ctx, login("felipe", testPassword, "198.51.100.7"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.completeTOTP.Execute(ctx, completeFrom(res.Token, f.code(t, secret), "198.51.100.7")); !errors.Is(err, service.ErrLocked) {
		t.Fatalf("durante el bloqueo: got %v", err)
	}
	f.advance(service.LockoutFor(5))
	if _, err := f.completeTOTP.Execute(ctx, completeFrom(res.Token, f.code(t, secret), "198.51.100.7")); err != nil {
		t.Fatalf("vencido el bloqueo, el código correcto entra: %v", err)
	}
}
