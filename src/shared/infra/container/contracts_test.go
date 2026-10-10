package container

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// Contratos entre módulos con el servidor real. Los módulos no se importan
// entre sí (§4.1 del spec): cada uno lee o escribe las tablas del otro desde
// un adaptador propio, así que el acuerdo entre ambos (una clave, un formato,
// una columna, una cascada) solo se prueba con los cuatro armados juntos.

// queryInt ejecuta una consulta que devuelve un entero.
func (a api) queryInt(query string, args ...any) int64 {
	a.t.Helper()
	var n int64
	ctx := context.Background()
	if err := a.c.DB.Conn(ctx).QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		a.t.Fatalf("%s: %v", query, err)
	}
	return n
}

func (a api) exec(query string, args ...any) {
	a.t.Helper()
	ctx := context.Background()
	if _, err := a.c.DB.Conn(ctx).ExecContext(ctx, query, args...); err != nil {
		a.t.Fatalf("%s: %v", query, err)
	}
}

// create hace un POST con la sesión s y espera 201.
func (a api) create(s httptestutil.Opt, path string, body any) map[string]any {
	a.t.Helper()
	w := a.do("POST", path, body, s)
	httptestutil.Expect(a.t, w, http.StatusCreated)
	return httptestutil.Decode[map[string]any](a.t, w)
}

// enableTOTP configura y activa el 2FA de la sesión s con identity, como el
// panel: setup, y enable con el código actual.
func (a api) enableTOTP(s httptestutil.Opt) {
	a.t.Helper()
	w := a.do("POST", "/api/v1/auth/totp/setup", nil, s)
	httptestutil.Expect(a.t, w, http.StatusOK)
	secret := httptestutil.Decode[map[string]string](a.t, w)["secret"]
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		a.t.Fatal(err)
	}
	httptestutil.Expect(a.t, a.do("POST", "/api/v1/auth/totp/enable", map[string]string{"code": code}, s), http.StatusNoContent)
}

// invite crea una invitación con la sesión s, la acepta con username y
// devuelve la sesión del usuario nuevo.
func (a api) invite(s httptestutil.Opt, username string) httptestutil.Opt {
	a.t.Helper()
	url, _ := a.create(s, "/api/v1/invites", nil)["url"].(string)
	token := url[strings.LastIndex(url, "/")+1:]
	w := a.do("POST", "/api/v1/auth/invite/"+token, map[string]string{"username": username, "password": testPassword})
	httptestutil.Expect(a.t, w, http.StatusCreated)
	return httptestutil.WithSession(httptestutil.SessionFrom(a.t, w))
}

// login abre una sesión de username (sin 2FA) y la devuelve.
func (a api) login(username string) httptestutil.Opt {
	a.t.Helper()
	w := a.do("POST", "/api/v1/auth/login", map[string]string{"username": username, "password": testPassword})
	httptestutil.Expect(a.t, w, http.StatusOK)
	if httptestutil.Decode[map[string]any](a.t, w)["mfa_required"] != false {
		a.t.Fatalf("login de %s pide 2FA: %s", username, w.Body.String())
	}
	return httptestutil.WithSession(httptestutil.SessionFrom(a.t, w))
}

// setRequire2FA cambia require_2fa con la sesión s (admin).
func (a api) setRequire2FA(s httptestutil.Opt, on bool) {
	a.t.Helper()
	httptestutil.Expect(a.t, a.do("PATCH", "/api/v1/settings", map[string]any{"require_2fa": on}, s), http.StatusOK)
}

// needsTOTPSetup lee needs_totp_setup de GET /auth/me con la sesión s.
func (a api) needsTOTPSetup(s httptestutil.Opt) bool {
	a.t.Helper()
	w := a.do("GET", "/api/v1/auth/me", nil, s)
	httptestutil.Expect(a.t, w, http.StatusOK)
	return httptestutil.Decode[map[string]any](a.t, w)["needs_totp_setup"] == true
}

// require_2fa lo escribe admin y lo lee identity en cada petición
// (shared.SettingRequire2FA, con shared.FormatBool y shared.ParseBool).
func TestContractRequire2FA(t *testing.T) {
	e := api{t: t, c: newTestContainer(t)}
	admin := e.setup()
	e.enableTOTP(admin) // sin su propio 2FA, admin no deja activarlo
	bob := e.invite(admin, "bob")
	if e.needsTOTPSetup(bob) {
		t.Fatal("sin require_2fa, bob no debería tener que configurar 2FA")
	}

	e.setRequire2FA(admin, true)
	stored := func() string {
		t.Helper()
		var v []byte
		ctx := context.Background()
		if err := e.c.DB.Conn(ctx).QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, shared.SettingRequire2FA).Scan(&v); err != nil {
			t.Fatalf("admin no guardó %s: %v", shared.SettingRequire2FA, err)
		}
		return string(v)
	}
	if got := stored(); got != shared.FormatBool(true) {
		t.Fatalf("admin guardó %s=%q, esperaba %q", shared.SettingRequire2FA, got, shared.FormatBool(true))
	}
	if !e.needsTOTPSetup(bob) {
		t.Fatal("con require_2fa, bob (sin TOTP) tiene que configurar 2FA")
	}
	expect(t, "bob sin TOTP con require_2fa", e.do("GET", "/api/v1/projects", nil, bob), http.StatusForbidden, "totp_setup_required")
	if e.needsTOTPSetup(admin) {
		t.Fatal("admin tiene TOTP: no tiene que configurarlo")
	}

	e.setRequire2FA(admin, false)
	if got := stored(); got != shared.FormatBool(false) {
		t.Fatalf("admin guardó %s=%q, esperaba %q", shared.SettingRequire2FA, got, shared.FormatBool(false))
	}
	if e.needsTOTPSetup(bob) {
		t.Fatal("sin require_2fa, bob ya no tiene que configurar 2FA")
	}
	expect(t, "bob sin TOTP ni require_2fa", e.do("GET", "/api/v1/projects", nil, bob), http.StatusOK, "")

	// Con require_2fa otra vez, configurarlo lo libera.
	e.setRequire2FA(admin, true)
	e.enableTOTP(bob)
	if e.needsTOTPSetup(bob) {
		t.Fatal("bob ya activó su TOTP")
	}
	expect(t, "bob con TOTP y require_2fa", e.do("GET", "/api/v1/projects", nil, bob), http.StatusOK, "")
}

// Un cambio de variables en secrets marca "cambios pendientes" en catalog
// (apps.config_changed_at): el del stage, en todas sus apps y solo en ellas;
// el de una app, solo en esa app.
func TestContractEnvChangeMarksPendingChanges(t *testing.T) {
	e := api{t: t, c: newTestContainer(t)}
	s := e.setup()
	const P = "/api/v1/projects/p"
	e.create(s, "/api/v1/projects", map[string]string{"slug": "p", "name": "P"})
	app := func(slug string) map[string]any {
		return map[string]any{"slug": slug, "kind": "service", "spec": map[string]any{"image": "x", "port": 80}}
	}
	for _, st := range []string{"st", "otro"} {
		e.create(s, P+"/stages", map[string]string{"slug": st})
	}
	apps := map[string]string{ // app → stage
		"a1": "st", "a2": "st", "a3": "otro",
	}
	for slug, st := range apps {
		e.create(s, P+"/stages/"+st+"/apps", app(slug))
	}

	// deployed deja todas las apps como recién desplegadas (sin cambios
	// pendientes), lo que en la Fase 2 hace un deploy.
	deployed := func() {
		t.Helper()
		e.exec(`UPDATE apps SET config_changed_at = 1000, last_deployed_at = 1000`)
	}
	expectPending := func(step string, want map[string]bool) {
		t.Helper()
		for slug, st := range apps {
			w := e.do("GET", P+"/stages/"+st+"/apps/"+slug, nil, s)
			httptestutil.Expect(t, w, http.StatusOK)
			if got := httptestutil.Decode[map[string]any](t, w)["pending_changes"]; got != want[slug] {
				t.Errorf("%s: pending_changes de %s=%v, esperaba %v", step, slug, got, want[slug])
			}
		}
	}

	deployed()
	expectPending("recién desplegadas", nil)

	httptestutil.Expect(t, e.do("PATCH", P+"/stages/st/env", map[string]any{"set": map[string]string{"K": "v"}}, s), http.StatusNoContent)
	expectPending("PATCH del env del stage st", map[string]bool{"a1": true, "a2": true})

	deployed()
	httptestutil.Expect(t, e.do("PUT", P+"/stages/st/env", map[string]any{"vars": map[string]string{"K": "w"}}, s), http.StatusNoContent)
	expectPending("PUT del env del stage st", map[string]bool{"a1": true, "a2": true})

	deployed()
	httptestutil.Expect(t, e.do("PUT", P+"/stages/st/apps/a2/env", map[string]any{"vars": map[string]string{"K": "v"}}, s), http.StatusNoContent)
	expectPending("PUT del env de a2", map[string]bool{"a2": true})
}

// Borrar un stage (catalog) revoca los tokens de ese stage (identity) y
// borra sus variables (secrets), por las cascadas de la base. Un stage nuevo
// con el mismo slug no hereda ni unos ni otras.
func TestContractDeleteStage(t *testing.T) {
	e := api{t: t, c: newTestContainer(t)}
	s := e.setup()
	const P = "/api/v1/projects/p"
	e.create(s, "/api/v1/projects", map[string]string{"slug": "p", "name": "P"})
	e.create(s, P+"/stages", map[string]string{"slug": "tmp"})
	oldID := e.queryInt(`SELECT id FROM stages WHERE slug = 'tmp'`)
	stageTokID, stageTok := e.newToken(s, "p", "tmp")
	_, projectTok := e.newToken(s, "p", "")
	httptestutil.Expect(t, e.do("PUT", P+"/stages/tmp/env", map[string]any{"vars": map[string]string{"A": "1"}}, s), http.StatusNoContent)
	expect(t, "token del stage antes de borrarlo", e.bearer(stageTok, "GET", P+"/stages/tmp/env", nil), http.StatusOK, "")

	httptestutil.Expect(t, e.do("DELETE", P+"/stages/tmp", nil, s), http.StatusNoContent)
	if n := e.queryInt(`SELECT COUNT(*) FROM env_vars WHERE stage_id = ?`, oldID); n != 0 {
		t.Errorf("quedaron %d variables del stage borrado", n)
	}
	if n := e.queryInt(`SELECT COUNT(*) FROM api_tokens WHERE stage_id = ?`, oldID); n != 0 {
		t.Errorf("quedaron %d tokens del stage borrado", n)
	}

	e.create(s, P+"/stages", map[string]string{"slug": "tmp"})
	if newID := e.queryInt(`SELECT id FROM stages WHERE slug = 'tmp'`); newID == oldID {
		t.Fatalf("el stage nuevo reutilizó el id %d (AUTOINCREMENT, §12)", oldID)
	}
	expect(t, "token del stage borrado, en el stage nuevo", e.bearer(stageTok, "GET", P+"/stages/tmp/env", nil), http.StatusUnauthorized, "unauthenticated")
	w := e.do("GET", "/api/v1/tokens", nil, s)
	httptestutil.Expect(t, w, http.StatusOK)
	for _, tk := range httptestutil.Decode[[]map[string]any](t, w) {
		if id, _ := tk["id"].(float64); fmt.Sprint(int64(id)) == stageTokID {
			t.Errorf("GET /tokens sigue listando el token del stage borrado: %v", tk)
		}
	}
	// El token del proyecto sigue sirviendo, y el stage nuevo empieza sin variables.
	w = e.bearer(projectTok, "GET", P+"/stages/tmp/env", nil)
	httptestutil.Expect(t, w, http.StatusOK)
	if vars := httptestutil.Decode[map[string][]any](t, w)["vars"]; len(vars) != 0 {
		t.Errorf("el stage nuevo heredó variables: %v", vars)
	}
}

// Borrar un usuario borra sus sesiones (cascada de users a sessions).
func TestContractDeleteUserRemovesSessions(t *testing.T) {
	e := api{t: t, c: newTestContainer(t)}
	admin := e.setup()
	bob := e.invite(admin, "bob")
	bob2 := e.login("bob")
	bobID := e.queryInt(`SELECT id FROM users WHERE username = 'bob'`)
	if n := e.queryInt(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, bobID); n != 2 {
		t.Fatalf("bob tiene %d sesiones, esperaba 2", n)
	}

	httptestutil.Expect(t, e.do("DELETE", "/api/v1/users/bob", nil, admin), http.StatusNoContent)
	if n := e.queryInt(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, bobID); n != 0 {
		t.Errorf("quedaron %d sesiones de bob", n)
	}
	for _, s := range []httptestutil.Opt{bob, bob2} {
		expect(t, "sesión de bob borrado", e.do("GET", "/api/v1/auth/me", nil, s), http.StatusUnauthorized, "unauthenticated")
	}
	expect(t, "la sesión de admin sigue", e.do("GET", "/api/v1/auth/me", nil, admin), http.StatusOK, "")
}

// admin decide si alguien puede activar require_2fa con
// users.totp_secret IS NOT NULL; identity lo llena al activar el 2FA (no al
// empezar a configurarlo) y lo borra al reiniciarlo.
func TestContractAdminTOTPStatus(t *testing.T) {
	e := api{t: t, c: newTestContainer(t)}
	admin := e.setup()
	tryRequire := func(name string, s httptestutil.Opt, status int, code string) {
		t.Helper()
		expect(t, name, e.do("PATCH", "/api/v1/settings", map[string]any{"require_2fa": true}, s), status, code)
		if status == http.StatusOK {
			e.setRequire2FA(s, false) // lo deja como estaba
		}
	}

	tryRequire("sin TOTP", admin, http.StatusConflict, "totp_required_first")
	httptestutil.Expect(t, e.do("POST", "/api/v1/auth/totp/setup", nil, admin), http.StatusOK)
	tryRequire("con el TOTP pendiente de confirmar", admin, http.StatusConflict, "totp_required_first")

	bob := e.invite(admin, "bob")
	e.enableTOTP(bob)
	tryRequire("con TOTP activo", bob, http.StatusOK, "")

	// admin le reinicia el 2FA a bob: sus sesiones se cierran y, con una
	// nueva, ya no puede activar require_2fa.
	httptestutil.Expect(t, e.do("DELETE", "/api/v1/users/bob/totp", nil, admin), http.StatusNoContent)
	if n := e.queryInt(`SELECT COUNT(*) FROM users WHERE username = 'bob' AND totp_secret IS NULL`); n != 1 {
		t.Fatal("el reinicio no borró users.totp_secret de bob")
	}
	tryRequire("después del reinicio de su 2FA", e.login("bob"), http.StatusConflict, "totp_required_first")
}
