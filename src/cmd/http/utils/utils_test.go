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
	k.Handle("OPTIONS /api/v1/projects/{p}", AccessToken, echo)
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

// cookies agrega varias cookies muelle_session, como haría un subdominio que
// planta la suya (cookie tossing).
func cookies(vs ...string) reqOpt {
	return func(r *http.Request) {
		for _, v := range vs {
			r.AddCookie(&http.Cookie{Name: SessionCookie, Value: v})
		}
	}
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
		// HEAD es lectura y OPTIONS es escritura (write := método distinto de GET y HEAD).
		{"bearer de stage, HEAD de proyecto", "HEAD", "/api/v1/projects/auditfy", bearer("mk_stage"), 200, ""},
		{"bearer de proyecto, HEAD de otro proyecto", "HEAD", "/api/v1/projects/otro", bearer("mk_project"), 403, "out_of_scope"},
		{"bearer de stage, OPTIONS de proyecto", "OPTIONS", "/api/v1/projects/auditfy", bearer("mk_stage"), 403, "out_of_scope"},
		{"bearer de proyecto, OPTIONS de proyecto", "OPTIONS", "/api/v1/projects/auditfy", bearer("mk_project"), 200, ""},
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
		{"dos cookies de sesión", "GET", "/session", cookies("full", "pending"), 400, "ambiguous_session"},
		{"dos cookies de sesión iguales", "GET", "/session", cookies("full", "full"), 400, "ambiguous_session"},
		{"dos cookies de sesión en endpoint de token", "GET", "/api/v1/projects/auditfy", cookies("full", "setup"), 400, "ambiguous_session"},
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

// El alcance de un token se verifica con los comodines {p}, {s} y {a}: una
// ruta AccessToken que no los tiene en su lugar dejaría pasar a un token de un
// stage a otro, así que el Kit la rechaza al registrarla.
func TestHandleTokenPatternGuard(t *testing.T) {
	ok := func(http.ResponseWriter, *http.Request) {}
	register := func(pattern string, a Access) (msg string) {
		defer func() {
			if r := recover(); r != nil {
				msg = fmt.Sprint(r)
			}
		}()
		k := NewKit(http.NewServeMux(), fakeAuth{}, true, discard())
		k.Handle(pattern, a, ok)
		return ""
	}

	const prefix = "no empieza con /api/v1/projects/{p}"
	bad := map[string]string{
		// Sin /api/v1/projects/{p} al principio.
		"GET /api/v1/projects":                "no empieza con",
		"GET /api/v1/tokens/{id}":             prefix,
		"GET /api/v1/projects/{project}":      prefix,
		"GET /api/v1/projects/{p...}":         prefix,
		"GET /api/v1/projects/{x}/env/{p}":    prefix,
		"GET /api/v2/projects/{p}":            prefix,
		"GET example.com/api/v1/projects/{x}": prefix,
		// Un literal de colección seguido de otra cosa que su comodín.
		"GET /api/v1/projects/{p}/stages/x/env":            `/stages/ seguido de "x" en vez de {s}`,
		"GET /api/v1/projects/{p}/stages/{stage}/env":      `/stages/ seguido de "{stage}" en vez de {s}`,
		"GET /api/v1/projects/{p}/stages/{x}/env/{s}":      `/stages/ seguido de "{x}" en vez de {s}`,
		"GET /api/v1/projects/{p}/st%61ges/{x}/env":        `/stages/ seguido de "{x}" en vez de {s}`,
		"GET /api/v1/projects/{p}/stages/%7Bs%7D/env":      `/stages/ seguido de "%7Bs%7D" en vez de {s}`,
		"GET /api/v1/projects/{p}/stages/{s}/apps/x":       `/apps/ seguido de "x" en vez de {a}`,
		"GET /api/v1/projects/{p}/stages/{s}/apps/{app}/x": `/apps/ seguido de "{app}" en vez de {a}`,
		// Comodines que no son de alcance (incluidos los de varios segmentos,
		// que se tragarían un /stages/<otro>).
		"GET /api/v1/projects/{p}/stage/{x}/env":                 "comodín {x}; solo se permiten {p}, {s} y {a}",
		"POST /api/v1/projects/{p}/stages/{s}/promote/{dst}":     "comodín {dst}",
		"GET /api/v1/projects/{p}/{rest...}":                     "comodín {rest...}",
		"GET /api/v1/projects/{p}/stages/{s}/{x...}":             "comodín {x...}",
		"GET /api/v1/projects/{p}/stages/{s}/apps/{a}/{path...}": "comodín {path...}",
		// Después de {a} se permite un comodín de un segmento (…/deploys/{id}),
		// pero no uno de varios ni antes de {a}.
		"GET /api/v1/projects/{p}/stages/{s}/apps/{a}/deploys/{id...}": "comodín {id...}",
		"GET /api/v1/projects/{p}/stages/{s}/deploys/{id}":             "comodín {id}",
		"GET /api/v1/projects/{p}/stages/{s}/apps/{a}/stages/{id}":     `/stages/ seguido de "{id}" en vez de {s}`,
		"GET /api/v1/projects/{p}/env/{s}":                             "{s} fuera de su lugar: va justo después de /stages/",
		"GET /api/v1/projects/{p}/stages/{s}/env/{a}":                  "{a} fuera de su lugar: va justo después de /apps/",
		"GET /api/v1/projects/{p}/stages/{s}/apps/{a}/stages/{s}":      "repite el comodín {s}",
		"GET /api/v1/projects/{p}/apps/{a}":                            "{a} sin {s}",
		// Abarcan cualquier ruta debajo.
		"GET /api/v1/projects/{p}/":        `termina en "/"`,
		"/api/v1/projects/{p}/stages/{s}/": `termina en "/"`,
	}
	for pattern, want := range bad {
		msg := register(pattern, AccessToken)
		if msg == "" {
			t.Errorf("%q: Handle con AccessToken debió hacer panic", pattern)
			continue
		}
		if !strings.Contains(msg, pattern) || !strings.Contains(msg, want) || !strings.Contains(msg, "AccessToken") {
			t.Errorf("%q: el panic %q debe nombrar la ruta, AccessToken y %q", pattern, msg, want)
		}
	}

	good := []string{
		// Las rutas AccessToken de catalog.
		"GET /api/v1/projects/{p}",
		"GET /api/v1/projects/{p}/stages",
		"GET /api/v1/projects/{p}/stages/{s}",
		"GET /api/v1/projects/{p}/stages/{s}/apps",
		"GET /api/v1/projects/{p}/stages/{s}/apps/{a}",
		// Las de secrets.
		"GET /api/v1/projects/{p}/stages/{s}/env",
		"PUT /api/v1/projects/{p}/stages/{s}/env",
		"PATCH /api/v1/projects/{p}/stages/{s}/env",
		"GET /api/v1/projects/{p}/stages/{s}/apps/{a}/env",
		"PUT /api/v1/projects/{p}/stages/{s}/apps/{a}/env",
		"PATCH /api/v1/projects/{p}/stages/{s}/apps/{a}/env",
		// Las de deploy (Fase 2): un comodín de un segmento después de {a}.
		"GET /api/v1/projects/{p}/stages/{s}/apps/{a}/deploys/{id}",
		"GET /api/v1/projects/{p}/stages/{s}/apps/{a}/deploys/{id}/log",
		"GET /api/v1/projects/{p}/stages/{s}/apps/{a}/deploys/{id}/log/stream",
		// Sin método, con host, con espacios y tabs, escapes y {$}, como los acepta el mux.
		"/api/v1/projects/{p}/env",
		"GET example.com/api/v1/projects/{p}",
		"GET\t /api/v1/projects/{p}",
		"GET /api/v1/pr%6Fjects/{p}/st%61ges/{s}/env",
		"GET /api/v1/projects/{p}/{$}",
		"GET /api/v1/projects/{p}/stages/{$}",
	}
	for _, pattern := range good {
		if msg := register(pattern, AccessToken); msg != "" {
			t.Errorf("%q: panic inesperado: %s", pattern, msg)
		}
	}
	// Las rutas que no aceptan tokens no necesitan comodines.
	for _, a := range []Access{AccessSession, AccessSetup, AccessMFAPending} {
		if msg := register("GET /api/v1/projects/", a); msg != "" {
			t.Errorf("Access %d: panic inesperado: %s", a, msg)
		}
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
		"null":              `null`,
		"null con espacios": " \n null ",
		"arreglo vacío":     `[]`,
		"string":            `"x"`,
		"número":            `1`,
		"booleano":          `true`,
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
		{`null`, "objeto"},
		{`[]`, "objeto"},
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
	r := httptest.NewRequest("DELETE", "/api/v1/projects/auditfy", nil)

	w := httptest.NewRecorder()
	k.Fail(w, r, validate.Errorf("slug", "no vale"))
	if e := decodeError(t, w); w.Code != 422 || e.Error.Code != "invalid" || e.Error.Message != "slug: no vale" {
		t.Fatalf("validación: %d %+v", w.Code, e)
	}

	w = httptest.NewRecorder()
	k.Fail(w, r, fmt.Errorf("borrando: %w", errGone))
	if e := decodeError(t, w); w.Code != http.StatusGone || e.Error.Code != "gone" || e.Error.Message != "ya no está" {
		t.Fatalf("registrado: %d %+v", w.Code, e)
	}
	if logs.Len() != 0 {
		t.Fatalf("un error registrado no va al log: %q", logs.String())
	}

	w = httptest.NewRecorder()
	k.Fail(w, r, errors.New("detalle interno: /data/muelle.db está bloqueada"))
	e := decodeError(t, w)
	if w.Code != 500 || e.Error.Code != "internal" || strings.Contains(e.Error.Message, "bloqueada") {
		t.Fatalf("desconocido: %d %+v", w.Code, e)
	}
	for _, want := range []string{"bloqueada", "method=DELETE", "path=/api/v1/projects/auditfy"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("el log del error interno debe tener %q: %q", want, logs.String())
		}
	}
}

// El link de una invitación lleva el token en la ruta: el log de un error
// interno no lo muestra.
func TestFailLogOmitsPathToken(t *testing.T) {
	var logs bytes.Buffer
	mux := http.NewServeMux()
	k := NewKit(mux, fakeAuth{}, true, slog.New(slog.NewTextHandler(&logs, nil)))
	k.HandlePublic("POST /api/v1/auth/invite/{token}", func(w http.ResponseWriter, r *http.Request) {
		k.Fail(w, r, errors.New("falló la base"))
	})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/auth/invite/secreto-de-la-invitacion", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", w.Code)
	}
	if strings.Contains(logs.String(), "secreto-de-la-invitacion") || !strings.Contains(logs.String(), "path=/api/v1/auth/invite/{token}") {
		t.Fatalf("log=%q", logs.String())
	}
}

// Un 429 dice cuándo reintentar (RateLimitRetryAfter, fijo); los demás errores no.
func TestFailRetryAfter(t *testing.T) {
	k := NewKit(http.NewServeMux(), fakeAuth{}, true, discard())
	errLimited, errLocked := errors.New("límite"), errors.New("bloqueado")
	k.RegisterError(errLimited, http.StatusTooManyRequests, "rate_limited", "espera")
	k.RegisterError(errLocked, http.StatusLocked, "locked", "bloqueado")
	r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)

	w := httptest.NewRecorder()
	k.Fail(w, r, fmt.Errorf("login: %w", errLimited))
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "60" {
		t.Fatalf("429: status %d, Retry-After %q", w.Code, w.Header().Get("Retry-After"))
	}
	for _, err := range []error{errLocked, errors.New("interno")} {
		w = httptest.NewRecorder()
		k.Fail(w, r, err)
		if got := w.Header().Get("Retry-After"); got != "" {
			t.Errorf("%v: Retry-After %q, esperaba ninguno", err, got)
		}
	}
}

func TestFailLaterRegistrationWins(t *testing.T) {
	k := NewKit(http.NewServeMux(), fakeAuth{}, true, discard())
	base := errors.New("base")
	specific := fmt.Errorf("específico: %w", base)
	k.RegisterError(base, http.StatusNotFound, "not_found", "no existe")
	k.RegisterError(specific, http.StatusBadRequest, "specific", "específico")

	r := httptest.NewRequest("GET", "/api/v1/x", nil)
	w := httptest.NewRecorder()
	k.Fail(w, r, fmt.Errorf("x: %w", specific))
	if e := decodeError(t, w); w.Code != http.StatusBadRequest || e.Error.Code != "specific" {
		t.Fatalf("el error más específico (registrado después) debe ganar: %d %+v", w.Code, e)
	}
	w = httptest.NewRecorder()
	k.Fail(w, r, base)
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

// Un string con UTF-8 inválido (ej. un nombre que vino de Docker) no tumba la
// respuesta: los bytes inválidos salen como U+FFFD.
func TestWriteJSONInvalidUTF8(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusOK, map[string]string{"name": "a\xffb"})
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["name"] != "a\uFFFDb" {
		t.Fatalf("name=%q, esperaba %q", got["name"], "a\uFFFDb")
	}
}

// Lo que escribe el Kit registra sus fallos en el logger del Kit; WriteJSON,
// que no recibe el Kit, en slog.Default() (main lo configura).
func TestWriteJSONLogger(t *testing.T) {
	unmarshalable := map[string]any{"f": func() {}}

	var kitLogs bytes.Buffer
	writeJSON(slog.New(slog.NewTextHandler(&kitLogs, nil)), httptest.NewRecorder(), http.StatusOK, unmarshalable)
	if !strings.Contains(kitLogs.String(), "respuesta JSON") {
		t.Fatalf("el error debe ir al logger del Kit: %q", kitLogs.String())
	}

	var defaultLogs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&defaultLogs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	WriteJSON(httptest.NewRecorder(), http.StatusOK, unmarshalable)
	if !strings.Contains(defaultLogs.String(), "respuesta JSON") {
		t.Fatalf("el error debe ir a slog.Default(): %q", defaultLogs.String())
	}
}

func TestClientIP(t *testing.T) {
	cases := []struct {
		name, remote, realIP, want string
	}{
		{"sin header", "10.0.0.5:51234", "", "10.0.0.5"},
		{"sin header, IPv6", "[2001:db8::1]:443", "", "2001:db8::1"},
		{"RemoteAddr sin puerto", "sin-puerto", "", "sin-puerto"},
		// Caddy llega desde la red interna (loopback o privada) y pone X-Real-IP.
		{"header desde 10.x", "10.0.0.5:51234", " 203.0.113.7 ", "203.0.113.7"},
		{"header desde 127.0.0.1", "127.0.0.1:51234", "203.0.113.7", "203.0.113.7"},
		{"header desde 192.168.x", "192.168.1.2:51234", "203.0.113.7", "203.0.113.7"},
		{"header desde ::1", "[::1]:51234", "203.0.113.7", "203.0.113.7"},
		{"header IPv6 se canoniza", "127.0.0.1:1", "2001:0db8:0:0::1", "2001:db8::1"},
		// Desde una IP pública el header es del cliente: se ignora.
		{"header falsificado desde IP pública", "198.51.100.9:51234", "10.1.2.3", "198.51.100.9"},
		{"header falsificado desde IPv6 pública", "[2001:db8::9]:1", "127.0.0.1", "2001:db8::9"},
		// Un header que no es una IP se ignora aunque venga de la red interna.
		{"header basura", "10.0.0.5:51234", "<script>", "10.0.0.5"},
		{"header con dos IPs", "10.0.0.5:51234", "203.0.113.7, 10.0.0.1", "10.0.0.5"},
		{"header con zona", "10.0.0.5:51234", "fe80::1%eth0", "10.0.0.5"},
		{"header desde RemoteAddr inválido", "sin-puerto", "203.0.113.7", "sin-puerto"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		if c.realIP != "" {
			r.Header.Set("X-Real-IP", c.realIP)
		}
		if ip := ClientIP(r); ip != c.want {
			t.Errorf("%s: ClientIP = %q, esperaba %q", c.name, ip, c.want)
		}
	}
}

func TestBearerToken(t *testing.T) {
	cases := map[string]struct {
		token string
		ok    bool
	}{
		"Bearer mk_abc": {"mk_abc", true},
		"bearer x":      {"x", true},
		"BEARER x":      {"x", true},
		"Bearer ":       {"", false},
		"bearer ":       {"", false},
		"Bearer":        {"", false},
		"Bearerx":       {"", false},
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
