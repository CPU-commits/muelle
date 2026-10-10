package http_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
)

func TestSetupCreatesFirstUser(t *testing.T) {
	e := newEnv(t)
	token, created, err := e.mod.EnsureSetupInvite(context.Background())
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}

	w := e.do("POST", "/api/v1/auth/setup", map[string]string{"token": token, "username": "felipe", "password": "corta"})
	expectCode(t, w, http.StatusUnprocessableEntity, "invalid")

	w = e.do("POST", "/api/v1/auth/setup", map[string]string{"token": token, "username": "felipe", "password": testPassword})
	httptestutil.Expect(t, w, http.StatusCreated)
	if got := httptestutil.Decode[map[string]string](t, w); got["username"] != "felipe" {
		t.Fatalf("body=%v", got)
	}
	session := httptestutil.SessionFrom(t, w)

	w = e.do("GET", "/api/v1/auth/me", nil, httptestutil.WithSession(session))
	httptestutil.Expect(t, w, http.StatusOK)
	if me := httptestutil.Decode[map[string]any](t, w); me["username"] != "felipe" || me["totp_enabled"] != false || me["needs_totp_setup"] != false {
		t.Fatalf("me=%v", me)
	}

	// Con un usuario creado, setup ya no funciona.
	w = e.do("POST", "/api/v1/auth/setup", map[string]string{"token": token, "username": "otro", "password": testPassword})
	expectCode(t, w, http.StatusConflict, "already_set_up")
}

func TestSetupWithBadToken(t *testing.T) {
	e := newEnv(t)
	w := e.do("POST", "/api/v1/auth/setup", map[string]string{"token": "inventado", "username": "felipe", "password": testPassword})
	expectCode(t, w, http.StatusBadRequest, "invalid_invite")
}

func TestSessionCookieFlags(t *testing.T) {
	e := newEnv(t)
	e.user("admin")
	w := e.do("POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": testPassword})
	httptestutil.Expect(t, w, http.StatusOK)
	c := w.Result().Cookies()[0]
	if c.Name != "muelle_session" || !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/" ||
		!c.Expires.Equal(e.clock.Now().Add(7*24*time.Hour)) {
		t.Fatalf("cookie=%+v", c)
	}
	if httptestutil.Decode[map[string]bool](t, w)["mfa_required"] {
		t.Fatal("sin TOTP no se pide código")
	}
}

func TestLoginErrors(t *testing.T) {
	e := newEnv(t)
	e.user("admin")
	w := e.do("POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": "mala-contraseña-x"})
	expectCode(t, w, http.StatusUnauthorized, "invalid_credentials")
	w = e.do("POST", "/api/v1/auth/login", `{"username":"admin","password":"x","extra":1}`)
	expectCode(t, w, http.StatusBadRequest, "invalid_json")
	// json/v2 compara los nombres de los campos exactamente.
	w = e.do("POST", "/api/v1/auth/login", `{"Username":"admin","password":"x"}`)
	expectCode(t, w, http.StatusBadRequest, "invalid_json")
}

func TestLoginLockedAndRateLimited(t *testing.T) {
	e := newEnv(t)
	e.user("admin")
	for range 5 {
		e.do("POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": "mala-contraseña-x"})
	}
	w := e.do("POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": testPassword})
	expectCode(t, w, http.StatusLocked, "locked")

	// 20 intentos por IP cada 15 min: van 6, y X-Real-IP es la IP del cliente.
	ip := func(r *http.Request) { r.Header.Set("X-Real-IP", "203.0.113.9") }
	for range 20 {
		e.do("POST", "/api/v1/auth/login", map[string]string{"username": "nadie", "password": "x"}, ip)
	}
	w = e.do("POST", "/api/v1/auth/login", map[string]string{"username": "nadie", "password": "x"}, ip)
	expectCode(t, w, http.StatusTooManyRequests, "rate_limited")
}

func TestCSRFOriginCheck(t *testing.T) {
	e := newEnv(t)
	e.user("admin")
	body := map[string]string{"username": "admin", "password": testPassword}

	w := e.do("POST", "/api/v1/auth/login", body, httptestutil.WithOrigin(""))
	httptestutil.Expect(t, w, http.StatusForbidden)
	w = e.do("POST", "/api/v1/auth/login", body, httptestutil.WithOrigin("https://evil.example"))
	expectCode(t, w, http.StatusForbidden, "bad_origin")
	// GET no necesita Origin.
	session := e.login()
	w = e.do("GET", "/api/v1/auth/me", nil, httptestutil.WithSession(session), httptestutil.WithOrigin(""))
	httptestutil.Expect(t, w, http.StatusOK)
}

func TestLogout(t *testing.T) {
	e := newEnv(t)
	session := e.login()
	w := e.do("POST", "/api/v1/auth/logout", nil, httptestutil.WithSession(session))
	httptestutil.Expect(t, w, http.StatusNoContent)
	if c := w.Result().Cookies(); len(c) != 1 || c[0].Name != "muelle_session" || c[0].Value != "" || c[0].MaxAge >= 0 {
		t.Fatalf("logout debe borrar la cookie: %+v", c)
	}
	w = e.do("GET", "/api/v1/auth/me", nil, httptestutil.WithSession(session))
	httptestutil.Expect(t, w, http.StatusUnauthorized)
	w = e.do("GET", "/api/v1/auth/me", nil)
	httptestutil.Expect(t, w, http.StatusUnauthorized)
}

func TestTOTPEndToEnd(t *testing.T) {
	e := newEnv(t)
	session := e.login()

	w := e.do("POST", "/api/v1/auth/totp/enable", map[string]string{"code": "123456"}, httptestutil.WithSession(session))
	expectCode(t, w, http.StatusConflict, "no_pending_totp")

	w = e.do("POST", "/api/v1/auth/totp/setup", nil, httptestutil.WithSession(session))
	httptestutil.Expect(t, w, http.StatusOK)
	setup := httptestutil.Decode[map[string]string](t, w)
	secret := setup["secret"]
	if secret == "" || setup["url"] == "" {
		t.Fatalf("setup=%v", setup)
	}
	w = e.do("POST", "/api/v1/auth/totp/enable", map[string]string{"code": e.wrongCode(secret)}, httptestutil.WithSession(session))
	expectCode(t, w, http.StatusUnauthorized, "invalid_code")
	w = e.do("POST", "/api/v1/auth/totp/enable", map[string]string{"code": e.code(secret)}, httptestutil.WithSession(session))
	httptestutil.Expect(t, w, http.StatusNoContent)

	// Login nuevo: queda pendiente del código.
	w = e.do("POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": testPassword})
	httptestutil.Expect(t, w, http.StatusOK)
	if !httptestutil.Decode[map[string]bool](t, w)["mfa_required"] {
		t.Fatal("con TOTP debe pedir código")
	}
	pending := httptestutil.SessionFrom(t, w)

	// La sesión pendiente no sirve para otra cosa.
	w = e.do("GET", "/api/v1/auth/me", nil, httptestutil.WithSession(pending))
	expectCode(t, w, http.StatusUnauthorized, "mfa_required")
	// Una sesión completa no puede usar el endpoint del código.
	w = e.do("POST", "/api/v1/auth/totp", map[string]string{"code": "000000"}, httptestutil.WithSession(session))
	expectCode(t, w, http.StatusBadRequest, "no_mfa_pending")

	w = e.do("POST", "/api/v1/auth/totp", map[string]string{"code": e.wrongCode(secret)}, httptestutil.WithSession(pending))
	expectCode(t, w, http.StatusUnauthorized, "invalid_code")
	w = e.do("POST", "/api/v1/auth/totp", map[string]string{"code": e.code(secret)}, httptestutil.WithSession(pending))
	httptestutil.Expect(t, w, http.StatusOK)
	if httptestutil.Decode[map[string]bool](t, w)["mfa_required"] {
		t.Fatal("tras el código la sesión es completa")
	}
	full := httptestutil.SessionFrom(t, w)
	if full == pending {
		t.Fatal("la sesión completa debe tener otro token")
	}
	w = e.do("GET", "/api/v1/auth/me", nil, httptestutil.WithSession(full))
	httptestutil.Expect(t, w, http.StatusOK)
	if me := httptestutil.Decode[map[string]any](t, w); me["totp_enabled"] != true {
		t.Fatalf("me=%v", me)
	}
	// La pendiente ya no existe.
	w = e.do("GET", "/api/v1/auth/me", nil, httptestutil.WithSession(pending))
	expectCode(t, w, http.StatusUnauthorized, "unauthenticated")
}

func TestRequire2FABlocksUntilConfigured(t *testing.T) {
	e := newEnv(t)
	session := e.login()
	e.exec(`INSERT INTO settings (key, value) VALUES ('require_2fa', 'true')`)

	w := e.do("GET", "/api/v1/auth/me", nil, httptestutil.WithSession(session))
	httptestutil.Expect(t, w, http.StatusOK)
	if me := httptestutil.Decode[map[string]any](t, w); me["needs_totp_setup"] != true {
		t.Fatalf("me=%v", me)
	}
	// Mientras no configure 2FA, solo puede usar los endpoints de setup.
	w = e.do("GET", "/api/v1/users", nil, httptestutil.WithSession(session))
	expectCode(t, w, http.StatusForbidden, "totp_setup_required")

	w = e.do("POST", "/api/v1/auth/totp/setup", nil, httptestutil.WithSession(session))
	httptestutil.Expect(t, w, http.StatusOK)
	secret := httptestutil.Decode[map[string]string](t, w)["secret"]
	w = e.do("POST", "/api/v1/auth/totp/enable", map[string]string{"code": e.code(secret)}, httptestutil.WithSession(session))
	httptestutil.Expect(t, w, http.StatusNoContent)
	w = e.do("GET", "/api/v1/users", nil, httptestutil.WithSession(session))
	httptestutil.Expect(t, w, http.StatusOK)
}
