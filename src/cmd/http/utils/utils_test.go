package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// fakeAuth mapea cookies y tokens a Principals fijos.
type fakeAuth struct {
	sessions map[string]entity.Principal
	tokens   map[string]entity.Principal
}

var (
	errBadSession = errors.New("sesión inválida")
	errBadToken   = errors.New("token inválido")
)

func (f fakeAuth) AuthenticateSession(_ context.Context, token string) (entity.Principal, error) {
	if p, ok := f.sessions[token]; ok {
		return p, nil
	}
	return entity.Principal{}, errBadSession
}

func (f fakeAuth) AuthenticateToken(_ context.Context, bearer string) (entity.Principal, error) {
	if p, ok := f.tokens[bearer]; ok {
		return p, nil
	}
	return entity.Principal{}, errBadToken
}

func decodeError(t *testing.T, w *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var b errorBody
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatalf("respuesta no es un error JSON (%d): %s", w.Code, w.Body.String())
	}
	return b
}

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newTestKit() (*Kit, *http.ServeMux) {
	auth := fakeAuth{
		sessions: map[string]entity.Principal{
			"full":    {UserID: 1, Username: "ana", SessionHash: "h-full"},
			"pending": {UserID: 1, Username: "ana", SessionHash: "h-pending", MFAPending: true},
			"setup":   {UserID: 2, Username: "beto", SessionHash: "h-setup", NeedsTOTPSetup: true},
		},
		tokens: map[string]entity.Principal{
			"mk_project": {Token: &entity.TokenScope{ID: 7, Name: "ci", ProjectSlug: "auditfy"}},
			"mk_stage":   {Token: &entity.TokenScope{ID: 8, Name: "ci-staging", ProjectSlug: "auditfy", StageSlug: "staging"}},
			"mk_broken":  {UserID: 1, Username: "ana"}, // AuthenticateToken sin TokenScope: un bug de identity
		},
	}
	mux := http.NewServeMux()
	k := NewKit(mux, auth, true, discard())
	k.RegisterError(errBadSession, http.StatusUnauthorized, "bad_session", "la sesión no vale")
	k.RegisterError(errBadToken, http.StatusUnauthorized, "bad_token", "el token no vale")

	// echo responde quién hizo la petición.
	echo := func(w http.ResponseWriter, r *http.Request) {
		p, ok := entity.PrincipalFrom(r.Context())
		if !ok {
			WriteJSON(w, http.StatusOK, map[string]any{"principal": false})
			return
		}
		var token int64
		if p.Token != nil {
			token = p.Token.ID
		}
		WriteJSON(w, http.StatusOK, map[string]any{"principal": true, "user": p.Username, "token": token})
	}
	k.Handle("GET /session", AccessSession, echo)
	k.Handle("GET /setup", AccessSetup, echo)
	k.Handle("POST /mfa", AccessMFAPending, echo)
	k.Handle("GET /api/v1/projects/{p}", AccessToken, echo)
	k.Handle("GET /api/v1/projects/{p}/stages/{s}/env", AccessToken, echo)
	k.Handle("PUT /api/v1/projects/{p}/stages/{s}/env", AccessToken, echo)
	k.Handle("DELETE /api/v1/projects/{p}", AccessToken, echo)
	k.HandlePublic("POST /public", echo)
	return k, mux
}

type reqOpt func(*http.Request)

func cookie(v string) reqOpt {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: SessionCookie, Value: v}) }
}

func bearer(v string) reqOpt {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+v) }
}

func serve(h http.Handler, method, path string, opts ...reqOpt) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	for _, o := range opts {
		o(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestHandleAccess(t *testing.T) {
	_, mux := newTestKit()
	cases := []struct {
		name   string
		method string
		path   string
		opt    reqOpt
		status int
		code   string // vacío: el handler respondió
	}{
		// Tokens.
		{"bearer en endpoint de sesión", "GET", "/session", bearer("mk_project"), 403, "token_not_allowed"},
		{"bearer en endpoint de setup", "GET", "/setup", bearer("mk_project"), 403, "token_not_allowed"},
		{"bearer en endpoint de MFA", "POST", "/mfa", bearer("mk_project"), 403, "token_not_allowed"},
		{"bearer de otro proyecto", "GET", "/api/v1/projects/otro", bearer("mk_project"), 403, "out_of_scope"},
		{"bearer de proyecto, leer", "GET", "/api/v1/projects/auditfy", bearer("mk_project"), 200, ""},
		{"bearer de proyecto, escribir stage", "PUT", "/api/v1/projects/auditfy/stages/production/env", bearer("mk_project"), 200, ""},
		{"bearer de stage, su stage", "PUT", "/api/v1/projects/auditfy/stages/staging/env", bearer("mk_stage"), 200, ""},
		{"bearer de stage, otro stage", "GET", "/api/v1/projects/auditfy/stages/production/env", bearer("mk_stage"), 403, "out_of_scope"},
		{"bearer de stage, leer proyecto", "GET", "/api/v1/projects/auditfy", bearer("mk_stage"), 200, ""},
		{"bearer de stage, borrar proyecto", "DELETE", "/api/v1/projects/auditfy", bearer("mk_stage"), 403, "out_of_scope"},
		{"bearer desconocido", "GET", "/api/v1/projects/auditfy", bearer("mk_nope"), 401, "bad_token"},
		{"bearer sin TokenScope", "GET", "/api/v1/projects/auditfy", bearer("mk_broken"), 500, "internal"},
		// Sesiones.
		{"sin cookie", "GET", "/session", nil, 401, "unauthenticated"},
		{"cookie vacía", "GET", "/session", cookie(""), 401, "unauthenticated"},
		{"cookie desconocida", "GET", "/session", cookie("nope"), 401, "bad_session"},
		{"sesión completa", "GET", "/session", cookie("full"), 200, ""},
		{"sesión completa en endpoint de token", "DELETE", "/api/v1/projects/otro", cookie("full"), 200, ""},
		{"sesión pendiente de MFA", "GET", "/session", cookie("pending"), 401, "mfa_required"},
		{"sesión pendiente de MFA en setup", "GET", "/setup", cookie("pending"), 401, "mfa_required"},
		{"sesión pendiente de MFA en MFA", "POST", "/mfa", cookie("pending"), 200, ""},
		{"sesión completa en MFA", "POST", "/mfa", cookie("full"), 400, "no_mfa_pending"},
		{"falta configurar TOTP", "GET", "/session", cookie("setup"), 403, "totp_setup_required"},
		{"falta configurar TOTP, endpoint de token", "GET", "/api/v1/projects/auditfy", cookie("setup"), 403, "totp_setup_required"},
		{"falta configurar TOTP, setup", "GET", "/setup", cookie("setup"), 200, ""},
		{"sesión completa en setup", "GET", "/setup", cookie("full"), 200, ""},
		// Públicos.
		{"público sin nada", "POST", "/public", nil, 200, ""},
	}
	for _, c := range cases {
		var opts []reqOpt
		if c.opt != nil {
			opts = append(opts, c.opt)
		}
		w := serve(mux, c.method, c.path, opts...)
		if w.Code != c.status {
			t.Errorf("%s: status %d, esperaba %d: %s", c.name, w.Code, c.status, w.Body.String())
			continue
		}
		if c.code != "" {
			if got := decodeError(t, w).Error.Code; got != c.code {
				t.Errorf("%s: code %q, esperaba %q", c.name, got, c.code)
			}
		}
	}
}

func TestHandlePutsPrincipalInContext(t *testing.T) {
	_, mux := newTestKit()
	read := func(w *httptest.ResponseRecorder) map[string]any {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	if m := read(serve(mux, "GET", "/session", cookie("full"))); m["principal"] != true || m["user"] != "ana" {
		t.Fatalf("sesión: %v", m)
	}
	if m := read(serve(mux, "GET", "/api/v1/projects/auditfy", bearer("mk_stage"))); m["principal"] != true || m["token"] != float64(8) {
		t.Fatalf("token: %v", m)
	}
	if m := read(serve(mux, "POST", "/public", cookie("full"))); m["principal"] != false {
		t.Fatalf("un endpoint público no autentica: %v", m)
	}
}

type decodeTarget struct {
	A int    `json:"a"`
	B string `json:"b"`
}

func decodeInto(body string) (*httptest.ResponseRecorder, bool, decodeTarget) {
	var dst decodeTarget
	r := httptest.NewRequest("POST", "/", strings.NewReader(body))
	w := httptest.NewRecorder()
	ok := DecodeJSON(w, r, &dst)
	return w, ok, dst
}

func TestDecodeJSONRejects(t *testing.T) {
	cases := map[string]string{
		"campo desconocido": `{"a":1,"c":2}`,
		"clave duplicada":   `{"a":1,"a":2}`,
		"UTF-8 inválido":    "{\"b\":\"\xff\"}",
		"datos después":     `{"a":1} x`,
		"dos valores":       `{"a":1} {"a":2}`,
		"vacío":             ``,
		"tipo incorrecto":   `{"a":"uno"}`,
		"no es objeto":      `[1]`,
		"más de 1 MiB":      `{"b":"` + strings.Repeat("x", MaxBodyBytes) + `"}`,
	}
	for name, body := range cases {
		w, ok, _ := decodeInto(body)
		if ok {
			t.Errorf("%s: DecodeJSON debió fallar", name)
			continue
		}
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d", name, w.Code)
			continue
		}
		e := decodeError(t, w)
		if e.Error.Code != "invalid_json" {
			t.Errorf("%s: code %q", name, e.Error.Code)
		}
		// Sin detalles internos de Go: ni el paquete, ni tipos, ni mensajes en inglés.
		for _, leak := range []string{"json:", "jsontext", "Go ", "struct", "int", "unknown", "invalid", "http:"} {
			if strings.Contains(e.Error.Message, leak) {
				t.Errorf("%s: el mensaje %q filtra %q", name, e.Error.Message, leak)
			}
		}
		if !strings.HasPrefix(e.Error.Message, "JSON inválido") && !strings.HasPrefix(e.Error.Message, "el cuerpo") {
			t.Errorf("%s: mensaje %q", name, e.Error.Message)
		}
	}
}

func TestDecodeJSONMessages(t *testing.T) {
	cases := []struct{ body, want string }{
		{`{"a":1,"c":2}`, `"c"`},
		{`{"a":1,"a":2}`, `"a"`},
		{`{"a":"uno"}`, `"a"`},
		{`{"b":"` + strings.Repeat("x", MaxBodyBytes) + `"}`, "1 MiB"},
	}
	for _, c := range cases {
		w, _, _ := decodeInto(c.body)
		if msg := decodeError(t, w).Error.Message; !strings.Contains(msg, c.want) {
			t.Errorf("mensaje %q, esperaba que nombrara %s", msg, c.want)
		}
	}
}

func TestDecodeJSONAccepts(t *testing.T) {
	w, ok, dst := decodeInto(` {"a": 3, "b": "ñandú"} ` + "\n")
	if !ok || dst.A != 3 || dst.B != "ñandú" {
		t.Fatalf("ok=%v dst=%+v body=%s", ok, dst, w.Body.String())
	}
	if w.Body.Len() != 0 {
		t.Fatalf("no debe escribir nada si funciona: %s", w.Body.String())
	}
}

func TestFail(t *testing.T) {
	var logs bytes.Buffer
	k := NewKit(http.NewServeMux(), fakeAuth{}, true, slog.New(slog.NewTextHandler(&logs, nil)))
	errGone := errors.New("se fue")
	k.RegisterError(errGone, http.StatusGone, "gone", "ya no está")

	w := httptest.NewRecorder()
	k.Fail(w, validate.Errorf("slug", "no vale"))
	if e := decodeError(t, w); w.Code != 422 || e.Error.Code != "invalid" || e.Error.Message != "slug: no vale" {
		t.Fatalf("validación: %d %+v", w.Code, e)
	}

	w = httptest.NewRecorder()
	k.Fail(w, fmt.Errorf("borrando: %w", errGone))
	if e := decodeError(t, w); w.Code != http.StatusGone || e.Error.Code != "gone" || e.Error.Message != "ya no está" {
		t.Fatalf("registrado: %d %+v", w.Code, e)
	}

	w = httptest.NewRecorder()
	k.Fail(w, errors.New("detalle interno: /data/muelle.db está bloqueada"))
	e := decodeError(t, w)
	if w.Code != 500 || e.Error.Code != "internal" || strings.Contains(e.Error.Message, "bloqueada") {
		t.Fatalf("desconocido: %d %+v", w.Code, e)
	}
	if !strings.Contains(logs.String(), "bloqueada") {
		t.Fatalf("el error interno debe quedar en el log: %q", logs.String())
	}
}

func TestFailLaterRegistrationWins(t *testing.T) {
	k := NewKit(http.NewServeMux(), fakeAuth{}, true, discard())
	base := errors.New("base")
	specific := fmt.Errorf("específico: %w", base)
	k.RegisterError(base, http.StatusNotFound, "not_found", "no existe")
	k.RegisterError(specific, http.StatusBadRequest, "specific", "específico")

	w := httptest.NewRecorder()
	k.Fail(w, fmt.Errorf("x: %w", specific))
	if e := decodeError(t, w); w.Code != http.StatusBadRequest || e.Error.Code != "specific" {
		t.Fatalf("el error más específico (registrado después) debe ganar: %d %+v", w.Code, e)
	}
	w = httptest.NewRecorder()
	k.Fail(w, base)
	if w.Code != http.StatusNotFound {
		t.Fatalf("base: %d", w.Code)
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusCreated, map[string]any{"b": []string(nil), "a": 1})
	if w.Code != http.StatusCreated || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status=%d headers=%v", w.Code, w.Header())
	}
	if got := strings.TrimSpace(w.Body.String()); got != `{"a":1,"b":[]}` {
		t.Fatalf("body=%s", got)
	}

	w = httptest.NewRecorder()
	WriteError(w, http.StatusConflict, "conflict", "ya existe")
	if e := decodeError(t, w); w.Code != http.StatusConflict || e.Error.Code != "conflict" || e.Error.Message != "ya existe" {
		t.Fatalf("WriteError: %d %+v", w.Code, e)
	}

	// Un valor que no se puede serializar da 500 en vez de una respuesta a medias.
	w = httptest.NewRecorder()
	WriteJSON(w, http.StatusOK, map[string]any{"f": func() {}})
	if e := decodeError(t, w); w.Code != 500 || e.Error.Code != "internal" {
		t.Fatalf("no serializable: %d %+v", w.Code, e)
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.5:51234"
	if ip := ClientIP(r); ip != "10.0.0.5" {
		t.Fatalf("RemoteAddr: %q", ip)
	}
	r.Header.Set("X-Real-IP", " 203.0.113.7 ")
	if ip := ClientIP(r); ip != "203.0.113.7" {
		t.Fatalf("X-Real-IP: %q", ip)
	}
	r = httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "sin-puerto"
	if ip := ClientIP(r); ip != "sin-puerto" {
		t.Fatalf("RemoteAddr sin puerto: %q", ip)
	}
}

func TestBearerToken(t *testing.T) {
	cases := map[string]struct {
		token string
		ok    bool
	}{
		"Bearer mk_abc": {"mk_abc", true},
		"Bearer ":       {"", false},
		"Basic abc":     {"", false},
		"":              {"", false},
	}
	for header, want := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		if header != "" {
			r.Header.Set("Authorization", header)
		}
		if token, ok := BearerToken(r); token != want.token || ok != want.ok {
			t.Errorf("%q: (%q, %v), esperaba (%q, %v)", header, token, ok, want.token, want.ok)
		}
	}
}

func TestSessionCookies(t *testing.T) {
	expires := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	for _, secure := range []bool{true, false} {
		k := NewKit(http.NewServeMux(), fakeAuth{}, secure, discard())

		w := httptest.NewRecorder()
		k.SetSessionCookie(w, "tok", expires)
		c := w.Result().Cookies()[0]
		if c.Name != SessionCookie || c.Value != "tok" || !c.HttpOnly || c.Secure != secure ||
			c.SameSite != http.SameSiteStrictMode || c.Path != "/" || !c.Expires.Equal(expires) {
			t.Fatalf("secure=%v: cookie=%+v", secure, c)
		}

		w = httptest.NewRecorder()
		k.ClearSessionCookie(w)
		c = w.Result().Cookies()[0]
		if c.Name != SessionCookie || c.Value != "" || c.MaxAge >= 0 || !c.HttpOnly || c.Secure != secure ||
			c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
			t.Fatalf("secure=%v: cookie borrada=%+v", secure, c)
		}
	}
}
