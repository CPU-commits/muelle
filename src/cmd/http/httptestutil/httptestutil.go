// Package httptestutil prueba los endpoints de un módulo solo: arma el
// servidor real de cmd/http, registra las rutas del módulo y hace peticiones.
package httptestutil

import (
	"bytes"
	"context"
	jsonv2 "encoding/json/v2"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	httpserver "github.com/CPU-commits/muelle/src/cmd/http"
	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

const PanelOrigin = "https://panel.test"

// FakeAuth mapea cookies y tokens a Principals fijos. Lo que no está devuelve entity.ErrUnauthenticated.
type FakeAuth struct {
	Sessions map[string]entity.Principal
	Tokens   map[string]entity.Principal
}

var _ utils.Authenticator = FakeAuth{}

func (f FakeAuth) AuthenticateSession(ctx context.Context, token string) (entity.Principal, error) {
	if p, ok := f.Sessions[token]; ok {
		return p, nil
	}
	return entity.Principal{}, entity.ErrUnauthenticated
}

func (f FakeAuth) AuthenticateToken(ctx context.Context, bearer string) (entity.Principal, error) {
	if p, ok := f.Tokens[bearer]; ok {
		return p, nil
	}
	return entity.Principal{}, entity.ErrUnauthenticated
}

// Client hace peticiones al servidor de prueba sin abrir un puerto.
type Client struct {
	t testing.TB
	h http.Handler
}

// New arma un Server (cmd/http) con PanelOrigin y cookies seguras, llama a
// register con su Kit y devuelve un cliente. Los errores internos (500) se
// registran en la salida del test.
func New(t testing.TB, auth utils.Authenticator, register func(k *utils.Kit)) *Client {
	t.Helper()
	s := httpserver.NewServer(httpserver.Config{
		PanelOrigin:   PanelOrigin,
		SecureCookies: true,
		Auth:          auth,
		Logger:        slog.New(slog.NewTextHandler(t.Output(), nil)),
	})
	register(s.Kit())
	return &Client{t: t, h: s}
}

// Opt modifica la petición de prueba.
type Opt func(*http.Request)

// WithSession agrega la cookie muelle_session.
func WithSession(token string) Opt {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: utils.SessionCookie, Value: token}) }
}

func WithBearer(token string) Opt {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
}

// WithOrigin reemplaza el Origin por defecto ("" lo quita).
func WithOrigin(origin string) Opt {
	return func(r *http.Request) {
		if origin == "" {
			r.Header.Del("Origin")
		} else {
			r.Header.Set("Origin", origin)
		}
	}
}

// Do envía la petición con Origin: PanelOrigin por defecto; body se serializa a JSON si no es nil.
// Un body string o []byte se manda tal cual, para probar JSON inválido.
func (c *Client) Do(method, path string, body any, opts ...Opt) *httptest.ResponseRecorder {
	c.t.Helper()
	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		reader = bytes.NewBufferString(b)
	case []byte:
		reader = bytes.NewReader(b)
	default:
		raw, err := jsonv2.Marshal(b, jsonv2.Deterministic(true))
		if err != nil {
			c.t.Fatalf("serializando el body de %s %s: %v", method, path, err)
		}
		reader = bytes.NewReader(raw)
	}
	r := httptest.NewRequest(method, path, reader)
	r.Header.Set("Origin", PanelOrigin)
	if reader != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	for _, o := range opts {
		o(r)
	}
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, r)
	return w
}

// Decode lee el body de la respuesta como T; si no puede, falla mostrando el body.
func Decode[T any](t testing.TB, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := jsonv2.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("JSON inválido (%d): %s", w.Code, w.Body.String())
	}
	return v
}

// ErrorCode devuelve el código de una respuesta {"error": {"code", "message"}}.
func ErrorCode(t testing.TB, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := jsonv2.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error == nil {
		t.Fatalf("la respuesta no es un error (%d): %s", w.Code, w.Body.String())
		return ""
	}
	return body.Error.Code
}

// Expect falla mostrando el body si el status no es el esperado.
func Expect(t testing.TB, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status %d, esperaba %d: %s", w.Code, status, w.Body.String())
	}
}

// SessionFrom devuelve el valor de la cookie muelle_session de la respuesta.
func SessionFrom(t testing.TB, w *httptest.ResponseRecorder) string {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == utils.SessionCookie && c.Value != "" {
			return c.Value
		}
	}
	t.Fatalf("la respuesta no trae cookie de sesión: %v", w.Result().Header)
	return ""
}
