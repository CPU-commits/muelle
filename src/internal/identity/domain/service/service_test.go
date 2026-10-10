package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/package/clock"
)

func TestRandomToken(t *testing.T) {
	plain, hash := RandomToken(APITokenPrefix)
	if !strings.HasPrefix(plain, "mk_") || len(plain) != 3+43 {
		t.Fatalf("plain=%q (%d)", plain, len(plain))
	}
	if hash != HashToken(plain) || len(hash) != 64 {
		t.Fatalf("hash=%q", hash)
	}
	other, _ := RandomToken(APITokenPrefix)
	if other == plain {
		t.Fatal("tokens repetidos")
	}
	session, _ := RandomToken("")
	if len(session) != 43 || strings.ContainsAny(session, "+/=") {
		t.Fatalf("token de sesión %q: debe ser base64url sin relleno", session)
	}
}

func TestLoginLimiter(t *testing.T) {
	c := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	l := NewLoginLimiter(3, 15*time.Minute, c)
	for i := 0; i < 3; i++ {
		if !l.Allow("1.1.1.1") {
			t.Fatalf("intento %d rechazado", i+1)
		}
	}
	if l.Allow("1.1.1.1") {
		t.Fatal("el cuarto intento debió rechazarse")
	}
	if !l.Allow("2.2.2.2") {
		t.Fatal("otra IP no debe verse afectada")
	}
	c.Advance(15 * time.Minute)
	if !l.Allow("1.1.1.1") {
		t.Fatal("la ventana nueva debe permitir")
	}
}

// La IP viene de un header (X-Real-IP), así que puede ser cualquier texto:
// el mapa tiene un tamaño máximo y las claves un largo máximo.
func TestLoginLimiterIsBounded(t *testing.T) {
	c := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	l := NewLoginLimiter(2, 15*time.Minute, c)
	l.maxIPs = 8

	for i := range 100 {
		if !l.Allow(fmt.Sprintf("10.0.0.%d", i)) {
			t.Fatalf("IP nueva %d rechazada", i)
		}
		if n := len(l.hits); n > l.maxIPs {
			t.Fatalf("el mapa creció a %d entradas (máximo %d)", n, l.maxIPs)
		}
	}

	// Con el mapa lleno, una IP que ya está en él no desaloja a nadie y conserva su cuenta.
	l = NewLoginLimiter(2, 15*time.Minute, c)
	l.maxIPs = 8
	l.Allow("1.1.1.1")
	for i := range 7 {
		l.Allow(fmt.Sprintf("10.0.0.%d", i))
	}
	l.Allow("1.1.1.1")
	if l.Allow("1.1.1.1") {
		t.Fatal("el tercer intento de 1.1.1.1 debió rechazarse")
	}
	if n := len(l.hits); n != 8 {
		t.Fatalf("%d entradas, esperaba 8", n)
	}

	// Pasada la ventana, una IP nueva con el mapa lleno barre las vencidas
	// antes de desalojar ventanas vigentes.
	c.Advance(15 * time.Minute)
	l.Allow("2.2.2.2")
	if n := len(l.hits); n != 1 {
		t.Fatalf("quedaron %d entradas, esperaba solo 2.2.2.2", n)
	}
}

// Con el mapa lleno se desalojan las ventanas que vencen antes, nunca
// cualquiera.
func TestLoginLimiterEvictsSoonestEnding(t *testing.T) {
	c := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	l := NewLoginLimiter(1, 15*time.Minute, c)
	l.maxIPs = 100
	for i := range 100 {
		l.Allow(fmt.Sprintf("10.0.0.%d", i)) // la ventana de cada una vence un segundo después que la anterior
		c.Advance(time.Second)
	}
	l.Allow("10.0.1.1")
	// Se fueron las 25 más viejas (hasta dejar 3/4) y quedaron las demás.
	for i := range 100 {
		_, ok := l.hits[fmt.Sprintf("10.0.0.%d", i)]
		if want := i >= 25; ok != want {
			t.Fatalf("10.0.0.%d: en el mapa=%v, esperaba %v", i, ok, want)
		}
	}
	if l.Allow("10.0.0.99") {
		t.Fatal("la IP con la ventana más nueva sigue limitada")
	}
}

func TestLoginLimiterCapsKeyLength(t *testing.T) {
	c := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	l := NewLoginLimiter(1, 15*time.Minute, c)
	long := strings.Repeat("a", 10000)
	if !l.Allow(long + "x") {
		t.Fatal("primer intento rechazado")
	}
	// Mismo prefijo de maxIPKey bytes: cuenta como la misma clave.
	if l.Allow(long + "y") {
		t.Fatal("dos claves con el mismo prefijo deben compartir la ventana")
	}
	for k := range l.hits {
		if len(k) > maxIPKey {
			t.Fatalf("clave de %d bytes guardada", len(k))
		}
	}
}

// ipKey agrupa una IPv4 sola y una IPv6 por su /64: un cliente IPv6 suele
// tener un /64 entero y podría rotar direcciones para saltarse los límites.
func TestIPKey(t *testing.T) {
	cases := map[string]string{
		"203.0.113.9":            "203.0.113.9",
		"::ffff:203.0.113.9":     "203.0.113.9",
		"2001:db8:1:2:3:4:5:6":   "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff::1":   "2001:db8:1:2::/64",
		"2001:db8:1:3::1":        "2001:db8:1:3::/64",
		"fe80::1%eth0":           "fe80::/64",
		"sin-puerto":             "sin-puerto",
		strings.Repeat("x", 100): strings.Repeat("x", maxIPKey),
		"":                       "",
	}
	for ip, want := range cases {
		if got := ipKey(ip); got != want {
			t.Errorf("ipKey(%q) = %q, esperaba %q", ip, got, want)
		}
	}
}

func TestLoginLimiterGroupsIPv6By64(t *testing.T) {
	c := clock.NewManual(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	l := NewLoginLimiter(1, 15*time.Minute, c)
	if !l.Allow("2001:db8:1:2::1") {
		t.Fatal("primer intento rechazado")
	}
	if l.Allow("2001:db8:1:2::ffff") {
		t.Fatal("otra dirección del mismo /64 comparte el límite")
	}
	if !l.Allow("2001:db8:1:3::1") {
		t.Fatal("otro /64 no se ve afectado")
	}
}

func TestLoginLimiterDefaults(t *testing.T) {
	if LoginMaxPerIP != 20 || LoginWindow != 15*time.Minute {
		t.Fatalf("el spec pide 20 intentos cada 15 min: %d / %v", LoginMaxPerIP, LoginWindow)
	}
}

func TestLockoutFor(t *testing.T) {
	cases := map[int]time.Duration{
		0: 0, 4: 0, 5: time.Minute, 6: 2 * time.Minute, 7: 4 * time.Minute,
		10: 32 * time.Minute, 11: time.Hour, 50: time.Hour, 1000: time.Hour,
	}
	for n, want := range cases {
		if got := LockoutFor(n); got != want {
			t.Errorf("LockoutFor(%d) = %v, want %v", n, got, want)
		}
	}
}

func TestPolicyConstants(t *testing.T) {
	if SessionIdle != 12*time.Hour || SessionMax != 7*24*time.Hour || InviteTTL != 24*time.Hour || MFAPendingTTL != 5*time.Minute {
		t.Fatalf("idle=%v max=%v invite=%v mfa=%v", SessionIdle, SessionMax, InviteTTL, MFAPendingTTL)
	}
}

func TestTOTPAAD(t *testing.T) {
	if got := TOTPSecretAAD(12); got != "users:totp:12" {
		t.Fatalf("secret AAD=%q", got)
	}
	if got := TOTPPendingAAD(12); got != "users:totp_pending:12" {
		t.Fatalf("pending AAD=%q", got)
	}
}
