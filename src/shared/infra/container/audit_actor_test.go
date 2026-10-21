package container

import (
	"net/http"
	"testing"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare/cftest"
)

// actor_name de GET /audit con el contenedor completo (contrato §4.2): cada
// módulo que audita (identity, catalog, secrets, admin e ingress) lo recibe
// del Principal de la petición, con sesiones y tokens de verdad, y el
// evento lo conserva después de borrar el usuario y el token.
func TestAuditActorNames(t *testing.T) {
	e := newIngressContainer(t, cftest.New(t))
	admin := e.setup()
	ana := e.invite(admin, "ana")

	const P = "/api/v1/projects/auditfy"
	e.create(admin, "/api/v1/projects", map[string]string{"slug": "auditfy", "name": "Auditfy"})
	e.create(admin, P+"/stages", map[string]string{"slug": "staging"})
	e.create(admin, P+"/stages/staging/apps", map[string]any{"slug": "api", "kind": "service", "spec": map[string]any{"image": "x:1", "port": 80}})
	e.create(admin, P+"/stages/staging/apps/api/domains", map[string]any{"hostname": "api.auditfy.cl", "target_service": "api", "target_port": 80})
	httptestutil.Expect(t, e.do("PATCH", "/api/v1/settings", map[string]any{"require_2fa": false}, admin), http.StatusOK)

	tokenID, token := e.newToken(admin, "auditfy", "")
	httptestutil.Expect(t, e.bearer(token, "PUT", P+"/stages/staging/env", map[string]any{"vars": map[string]string{"A": "1"}}), http.StatusNoContent)
	httptestutil.Expect(t, e.do("PATCH", P+"/stages/staging/apps/api/env", map[string]any{"set": map[string]string{"B": "2"}}, ana), http.StatusNoContent)

	// Un login fallido no tiene autor: es del sistema.
	httptestutil.Expect(t, e.do("POST", "/api/v1/auth/login", map[string]string{"username": "ana", "password": "no-es-la-contraseña"}), http.StatusUnauthorized)

	// Borrados el token y el usuario, sus eventos siguen con su nombre.
	httptestutil.Expect(t, e.do("DELETE", "/api/v1/tokens/"+tokenID, nil, admin), http.StatusNoContent)
	httptestutil.Expect(t, e.do("DELETE", "/api/v1/users/ana", nil, admin), http.StatusNoContent)

	w := e.do("GET", "/api/v1/audit?limit=500", nil, admin)
	httptestutil.Expect(t, w, http.StatusOK)
	events := httptestutil.Decode[[]map[string]any](t, w)

	want := map[[2]string]string{
		{"user.create", "user:admin"}:              "admin",
		{"invite.create", "user:admin"}:            "admin",
		{"user.create", "user:ana"}:                "ana",
		{"project.create", "project:auditfy"}:      "admin",
		{"stage.create", "stage:auditfy/staging"}:  "admin",
		{"app.create", "app:auditfy/staging/api"}:  "admin",
		{"domain.create", "domain:api.auditfy.cl"}: "admin",
		{"settings.update", "settings"}:            "admin",
		{"token.create", "token:" + tokenID}:       "admin",
		{"env.replace", "stage:auditfy/staging"}:   "ci",
		{"env.patch", "app:auditfy/staging/api"}:   "ana",
		{"auth.login_failed", "user:ana"}:          "system",
		{"token.delete", "token:" + tokenID}:       "admin",
		{"user.delete", "user:ana"}:                "admin",
	}
	byType := map[string]string{"env.replace": "token", "auth.login_failed": "system"}
	seen := map[[2]string]bool{}
	for _, ev := range events {
		action, _ := ev["action"].(string)
		target, _ := ev["target"].(string)
		name, ok := ev["actor_name"].(string)
		if !ok || name == "" {
			t.Errorf("%s %s: actor_name=%#v, debe ser un string con el nombre", action, target, ev["actor_name"])
			continue
		}
		key := [2]string{action, target}
		exp, listed := want[key]
		if !listed {
			continue // otros eventos (ej. del arranque): basta con que traigan nombre
		}
		seen[key] = true
		typ := "user"
		if v, ok := byType[action]; ok {
			typ = v
		}
		if name != exp || ev["actor_type"] != typ {
			t.Errorf("%s %s: actor %v/%q, esperaba %s/%q", action, target, ev["actor_type"], name, typ, exp)
		}
	}
	for key := range want {
		if !seen[key] {
			t.Errorf("falta el evento %s %s en GET /audit", key[0], key[1])
		}
	}
}
