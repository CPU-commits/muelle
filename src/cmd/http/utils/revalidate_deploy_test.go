package utils

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// Revalidate repite las comprobaciones de Handle con la credencial con que se
// abrió un stream (contrato §0.3), sin escribir la respuesta.
func TestRevalidate(t *testing.T) {
	auth := fakeAuth{
		sessions: map[string]entity.Principal{
			"full":    {UserID: 1, Username: "ana", SessionHash: "h-full"},
			"gone":    {UserID: 1, Username: "ana", SessionHash: "h-gone"},
			"pending": {UserID: 1, Username: "ana", SessionHash: "h-pending", MFAPending: true},
			"setup":   {UserID: 2, Username: "beto", SessionHash: "h-setup", NeedsTOTPSetup: true},
		},
		tokens: map[string]entity.Principal{
			"mk_project": {Token: &entity.TokenScope{ID: 7, Name: "ci", ProjectSlug: "auditfy"}},
			"mk_stage":   {Token: &entity.TokenScope{ID: 8, Name: "ci-staging", ProjectSlug: "auditfy", StageSlug: "staging"}},
			"mk_revoked": {Token: &entity.TokenScope{ID: 9, Name: "viejo", ProjectSlug: "auditfy"}},
			"mk_broken":  {UserID: 1, Username: "ana"},
		},
	}
	k := NewKit(http.NewServeMux(), auth, true, discard())
	// La sesión "gone" se cerró y el token "mk_revoked" se revocó después de abrir el stream.
	delete(auth.sessions, "gone")
	delete(auth.tokens, "mk_revoked")

	req := func(p, s string, opts ...reqOpt) *http.Request {
		r := httptest.NewRequest("GET", "/api/v1/projects/"+p+"/stages/"+s+"/apps/api/logs", nil)
		r.SetPathValue("p", p)
		r.SetPathValue("s", s)
		r.SetPathValue("a", "api")
		for _, o := range opts {
			o(r)
		}
		return r
	}
	for _, c := range []struct {
		name   string
		r      *http.Request
		access Access
		want   error // nil: sigue valiendo
	}{
		{"sesión vigente", req("auditfy", "staging", cookie("full")), AccessToken, nil},
		{"sesión vigente, ruta de sesión", req("auditfy", "staging", cookie("full")), AccessSession, nil},
		{"la sesión ya no existe", req("auditfy", "staging", cookie("gone")), AccessToken, errBadSession},
		{"sin credencial", req("auditfy", "staging"), AccessToken, entity.ErrUnauthenticated},
		{"cookie vacía", req("auditfy", "staging", cookie("")), AccessToken, entity.ErrUnauthenticated},
		{"dos cookies", req("auditfy", "staging", cookies("full", "full")), AccessToken, entity.ErrUnauthenticated},
		{"sesión con el 2FA a medias", req("auditfy", "staging", cookie("pending")), AccessToken, entity.ErrUnauthenticated},
		{"sesión que debe configurar TOTP", req("auditfy", "staging", cookie("setup")), AccessToken, entity.ErrForbidden},
		{"token del proyecto", req("auditfy", "production", bearer("mk_project")), AccessToken, nil},
		{"token de su stage", req("auditfy", "staging", bearer("mk_stage")), AccessToken, nil},
		{"token revocado", req("auditfy", "staging", bearer("mk_revoked")), AccessToken, errBadToken},
		{"token de otro proyecto", req("otro", "staging", bearer("mk_project")), AccessToken, entity.ErrForbidden},
		{"token de otro stage", req("auditfy", "production", bearer("mk_stage")), AccessToken, entity.ErrForbidden},
		{"token en una ruta de sesión", req("auditfy", "staging", bearer("mk_project")), AccessSession, entity.ErrForbidden},
		{"token sin TokenScope", req("auditfy", "staging", bearer("mk_broken")), AccessToken, errTokenWithoutScope},
	} {
		err := k.Revalidate(c.r, c.access)
		switch {
		case c.want == nil && err != nil:
			t.Errorf("%s: %v, esperaba nil", c.name, err)
		case c.want != nil && !errors.Is(err, c.want):
			t.Errorf("%s: %v, esperaba %v", c.name, err, c.want)
		}
	}
}

// El alcance de un token se revisa como lectura: uno de un stage sigue
// leyendo las rutas del proyecto, aunque el stream se abriera con otro método.
func TestRevalidateTokenScopeAsRead(t *testing.T) {
	k, _ := newTestKit()
	r := httptest.NewRequest("POST", "/api/v1/projects/auditfy", nil)
	r.SetPathValue("p", "auditfy")
	bearer("mk_stage")(r)
	if err := k.Revalidate(r, AccessToken); err != nil {
		t.Fatalf("token de stage leyendo el proyecto: %v", err)
	}
}
