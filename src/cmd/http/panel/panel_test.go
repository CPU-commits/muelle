package panel

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

const (
	testShell = "<!doctype html><title>muelle</title>"
	testCSP   = "default-src 'self'; script-src 'self' 'sha256-abc='; frame-ancestors 'none'"
)

// testBuild es un build de SvelteKit (adapter-static) chico: el shell, la
// CSP con el \n final que escribe svelte.config.js, archivos con hash en
// _app/immutable/ y los que no lo tienen.
func testBuild() fstest.MapFS {
	return fstest.MapFS{
		"200.html":                              {Data: []byte(testShell)},
		"csp-header.txt":                        {Data: []byte(testCSP + "\n")},
		"_app/immutable/entry/start.Bx1.js":     {Data: []byte("export const start = 1\n")},
		"_app/immutable/assets/geist.Cq2.woff2": {Data: []byte("wOF2fuente")},
		"_app/immutable/assets/app.D3.css":      {Data: []byte("body{margin:0}\n")},
		"_app/version.json":                     {Data: []byte(`{"version":"1"}`)},
		"favicon.svg":                           {Data: []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"/>")},
		"robots.txt":                            {Data: []byte("User-agent: *\nDisallow: /\n")},
		"assets/x.png":                          {Data: []byte("\x89PNG")},
	}
}

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	h, err := New(testBuild())
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func serve(h http.Handler, method, target string, header ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	for i := 0; i+1 < len(header); i += 2 {
		r.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestNewValidatesBuild(t *testing.T) {
	cases := []struct {
		name string
		edit func(fstest.MapFS)
		want string // lo que tiene que nombrar el error
	}{
		{"sin 200.html", func(b fstest.MapFS) { delete(b, "200.html") }, "200.html"},
		{"sin csp-header.txt", func(b fstest.MapFS) { delete(b, "csp-header.txt") }, "csp-header.txt"},
		{"CSP vacía", func(b fstest.MapFS) { b["csp-header.txt"] = &fstest.MapFile{Data: []byte(" \n")} }, "csp-header.txt"},
		{"CSP en dos líneas", func(b fstest.MapFS) {
			b["csp-header.txt"] = &fstest.MapFile{Data: []byte("default-src 'self'\nframe-ancestors 'none'\n")}
		}, "csp-header.txt"},
		{"CSP con \\r", func(b fstest.MapFS) {
			b["csp-header.txt"] = &fstest.MapFile{Data: []byte("default-src 'self'\r; frame-ancestors 'none'")}
		}, "csp-header.txt"},
		{"CSP con un carácter de control", func(b fstest.MapFS) {
			b["csp-header.txt"] = &fstest.MapFile{Data: []byte("default-src 'self'\x00")}
		}, "csp-header.txt"},
	}
	for _, c := range cases {
		b := testBuild()
		c.edit(b)
		h, err := New(b)
		if err == nil {
			t.Errorf("%s: New no falló (%v)", c.name, h)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: el error no nombra %s: %v", c.name, c.want, err)
		}
	}

	// La CSP del build (con el \n final) sirve, y el header sale sin él.
	w := serve(newTestHandler(t), "GET", "/")
	if got := w.Header().Get("Content-Security-Policy"); got != testCSP {
		t.Fatalf("Content-Security-Policy=%q, esperaba %q", got, testCSP)
	}
}

func TestServesBuildFiles(t *testing.T) {
	const immutable = "public, max-age=31536000, immutable"
	b := testBuild()
	cases := []struct {
		path, file, ctype, cache string
	}{
		{"/_app/immutable/entry/start.Bx1.js", "_app/immutable/entry/start.Bx1.js", "text/javascript; charset=utf-8", immutable},
		{"/_app/immutable/assets/geist.Cq2.woff2", "_app/immutable/assets/geist.Cq2.woff2", "font/woff2", immutable},
		{"/_app/immutable/assets/app.D3.css", "_app/immutable/assets/app.D3.css", "text/css; charset=utf-8", immutable},
		{"/_app/version.json", "_app/version.json", "application/json", "no-cache"},
		{"/favicon.svg", "favicon.svg", "image/svg+xml", "no-cache"},
		{"/robots.txt", "robots.txt", "text/plain; charset=utf-8", "no-cache"},
		{"/assets/x.png", "assets/x.png", "image/png", "no-cache"},
	}
	h := newTestHandler(t)
	for _, c := range cases {
		w := serve(h, "GET", c.path)
		if w.Code != http.StatusOK {
			t.Errorf("%s: %d %s", c.path, w.Code, w.Body.String())
			continue
		}
		if got := w.Header().Get("Content-Type"); got != c.ctype {
			t.Errorf("%s: Content-Type=%q, esperaba %q", c.path, got, c.ctype)
		}
		if got := w.Header().Get("Cache-Control"); got != c.cache {
			t.Errorf("%s: Cache-Control=%q, esperaba %q", c.path, got, c.cache)
		}
		if got, want := w.Body.String(), string(b[c.file].Data); got != want {
			t.Errorf("%s: body=%q, esperaba %q", c.path, got, want)
		}
		if w.Header().Get("Last-Modified") != "" {
			t.Errorf("%s: Last-Modified=%q (el embed no tiene fechas)", c.path, w.Header().Get("Last-Modified"))
		}
	}
}

// Toda ruta que no es un archivo del build es del router del panel: el
// 200.html, sin caché. El path se limpia antes de buscarlo, y un directorio
// cuenta como que no existe.
func TestFallbackToShell(t *testing.T) {
	h := newTestHandler(t)
	paths := []string{
		"/",
		"/login",
		"/projects/a/stages/b/apps/c/deploys",
		"/setup?token=x",
		"/csp-header.txt", // no se sirve
		"/_app/immutable/../../login",
		"/%2e%2e/%2e%2e/etc/passwd",
		"/assets",
		"/assets/",
		"/200.html",
	}
	for _, p := range paths {
		w := serve(h, "GET", p)
		if w.Code != http.StatusOK {
			t.Errorf("%s: %d %s", p, w.Code, w.Body.String())
			continue
		}
		if got := w.Body.String(); got != testShell {
			t.Errorf("%s: body=%q, esperaba el 200.html", p, got)
		}
		if got := w.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
			t.Errorf("%s: Content-Type=%q", p, got)
		}
		if got := w.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: Cache-Control=%q", p, got)
		}
	}
}

// Un chunk que no existe (un panel viejo abierto después de actualizar
// muelle) nunca recibe el HTML: con nosniff, el navegador lo rechazaría como
// script y el error sería más confuso.
func TestAppMissingIs404(t *testing.T) {
	h := newTestHandler(t)
	for _, p := range []string{"/_app/immutable/entry/viejo.js", "/_app/nada", "/_app", "/_app/immutable/", "/_app/immutable"} {
		w := serve(h, "GET", p)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: %d, esperaba 404", p, w.Code)
			continue
		}
		if ct := w.Header().Get("Content-Type"); strings.HasPrefix(ct, "text/html") {
			t.Errorf("%s: Content-Type=%q", p, ct)
		}
		if strings.Contains(w.Body.String(), testShell) {
			t.Errorf("%s: el 404 trae el 200.html", p)
		}
		if got := w.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: Cache-Control=%q, esperaba no-cache", p, got)
		}
	}
}

func TestSecurityHeadersEverywhere(t *testing.T) {
	h := newTestHandler(t)
	cases := []struct {
		method, path string
		status       int
	}{
		{"GET", "/favicon.svg", http.StatusOK},
		{"GET", "/_app/immutable/entry/start.Bx1.js", http.StatusOK},
		{"GET", "/login", http.StatusOK},
		{"GET", "/_app/nada.js", http.StatusNotFound},
		{"POST", "/login", http.StatusMethodNotAllowed},
	}
	for _, c := range cases {
		w := serve(h, c.method, c.path)
		if w.Code != c.status {
			t.Errorf("%s %s: %d, esperaba %d", c.method, c.path, w.Code, c.status)
		}
		want := map[string]string{
			"Content-Security-Policy": testCSP,
			"X-Content-Type-Options":  "nosniff",
			"Referrer-Policy":         "same-origin",
		}
		for k, v := range want {
			if got := w.Header().Get(k); got != v {
				t.Errorf("%s %s: %s=%q, esperaba %q", c.method, c.path, k, got, v)
			}
		}
	}
}

func TestOnlyGetAndHead(t *testing.T) {
	h := newTestHandler(t)
	for _, m := range []string{"POST", "PUT", "DELETE", "PATCH", "OPTIONS"} {
		w := serve(h, m, "/")
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /: %d", m, w.Code)
			continue
		}
		if got := w.Header().Get("Allow"); got != "GET, HEAD" {
			t.Errorf("%s /: Allow=%q", m, got)
		}
		if got := w.Header().Get("Content-Type"); got != "application/json" {
			t.Errorf("%s /: Content-Type=%q", m, got)
		}
		if got := strings.TrimSpace(w.Body.String()); got != `{"error":{"code":"method_not_allowed","message":"método no permitido"}}` {
			t.Errorf("%s /: body=%s", m, got)
		}
	}

	w := serve(h, "HEAD", "/")
	if w.Code != http.StatusOK || w.Body.Len() != 0 {
		t.Fatalf("HEAD /: %d, %d bytes", w.Code, w.Body.Len())
	}
	if got := w.Header().Get("Content-Length"); got != strconv.Itoa(len(testShell)) {
		t.Fatalf("HEAD /: Content-Length=%q, esperaba %d", got, len(testShell))
	}
	if got := w.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("HEAD /: Content-Type=%q", got)
	}

	// Range funciona (http.ServeContent).
	w = serve(h, "GET", "/robots.txt", "Range", "bytes=0-9")
	if w.Code != http.StatusPartialContent || w.Body.String() != "User-agent" {
		t.Fatalf("Range: %d %q", w.Code, w.Body.String())
	}
}

func TestETagRevalidates(t *testing.T) {
	h := newTestHandler(t)

	w := serve(h, "GET", "/")
	etag := w.Header().Get("ETag")
	if !strings.HasPrefix(etag, `"`) || !strings.HasSuffix(etag, `"`) || len(etag) != 34 {
		t.Fatalf("GET /: ETag=%q, esperaba un ETag fuerte de 32 hex", etag)
	}
	// El fallback y el propio 200.html son el mismo contenido: el mismo ETag.
	if got := serve(h, "GET", "/login").Header().Get("ETag"); got != etag {
		t.Fatalf("GET /login: ETag=%q, esperaba %q", got, etag)
	}
	w = serve(h, "GET", "/projects", "If-None-Match", etag)
	if w.Code != http.StatusNotModified || w.Body.Len() != 0 {
		t.Fatalf("If-None-Match: %d %q", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Security-Policy") != testCSP || w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("304 sin los headers: %v", w.Header())
	}
	if w := serve(h, "GET", "/", "If-None-Match", `"otro"`); w.Code != http.StatusOK {
		t.Fatalf("If-None-Match distinto: %d", w.Code)
	}

	fav := serve(h, "GET", "/favicon.svg").Header().Get("ETag")
	if fav == "" || fav == etag {
		t.Fatalf("favicon.svg: ETag=%q", fav)
	}
	if w := serve(h, "GET", "/favicon.svg", "If-None-Match", fav); w.Code != http.StatusNotModified {
		t.Fatalf("favicon.svg con If-None-Match: %d", w.Code)
	}
	if got := serve(h, "GET", "/_app/version.json").Header().Get("ETag"); got == "" {
		t.Fatal("_app/version.json sin ETag")
	}
	if got := serve(h, "GET", "/_app/immutable/entry/start.Bx1.js").Header().Get("ETag"); got != "" {
		t.Fatalf("un archivo de _app/immutable/ con ETag=%q", got)
	}
}

func TestContentType(t *testing.T) {
	cases := map[string]string{
		"200.html":                 "text/html; charset=utf-8",
		"a/start.js":               "text/javascript; charset=utf-8",
		"worker.mjs":               "text/javascript; charset=utf-8",
		"app.css":                  "text/css; charset=utf-8",
		"_app/version.json":        "application/json",
		"manifest.webmanifest":     "application/manifest+json",
		"start.js.map":             "application/json",
		"favicon.svg":              "image/svg+xml",
		"logo.png":                 "image/png",
		"foto.jpg":                 "image/jpeg",
		"foto.jpeg":                "image/jpeg",
		"foto.webp":                "image/webp",
		"favicon.ico":              "image/x-icon",
		"geist.woff":               "font/woff",
		"geist.woff2":              "font/woff2",
		"robots.txt":               "text/plain; charset=utf-8",
		"wasm/modulo.wasm":         "application/wasm",
		"X.PNG":                    "image/png",
		"_app/immutable/x.Bq.JS":   "text/javascript; charset=utf-8",
		"archivo.desconocido":      "application/octet-stream",
		"sin-extension":            "application/octet-stream",
		"carpeta.js/sin-extension": "application/octet-stream",
	}
	for name, want := range cases {
		if got := contentType(name); got != want {
			t.Errorf("contentType(%q)=%q, esperaba %q", name, got, want)
		}
	}
}
