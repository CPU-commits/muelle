// Package panel sirve el build del panel (web/, SvelteKit con adapter-static):
// el archivo del build o el 200.html, siempre con la CSP que escribió el
// build en csp-header.txt (contrato §5.5). cmd/http lo usa para toda ruta
// fuera de /api.
//
// El build se embebe solo con -tags panel (embed_panel.go); sin el tag,
// Embedded devuelve una página que dice que el binario no trae el panel
// (embed_none.go).
package panel

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
)

const (
	shellFile = "200.html"       // el shell de la SPA: lo resuelve el router del panel
	cspFile   = "csp-header.txt" // el header Content-Security-Policy exacto (svelte.config.js)

	// immutablePrefix son los archivos con hash en el nombre: nunca cambian.
	immutablePrefix = "_app/immutable/"

	cacheImmutable  = "public, max-age=31536000, immutable"
	cacheRevalidate = "no-cache"
)

type handler struct {
	fsys  fs.FS
	csp   string
	shell []byte
	etags map[string]string // los archivos fuera de _app/immutable/ (contrato §5.5)
}

// New arma el handler sobre un build (la raíz tiene 200.html y
// csp-header.txt). Falla si falta alguno, o si la CSP está vacía o no es una
// sola línea: muelle no arranca con un panel que el navegador bloquearía.
//
// Calcula al arrancar el ETag de cada archivo fuera de _app/immutable/ (son
// pocos y chicos); los de _app/immutable/ no se leen: con un año de caché no
// lo necesitan, y leerlos dejaría todo el embed en memoria.
func New(fsys fs.FS) (http.Handler, error) {
	shell, err := fs.ReadFile(fsys, shellFile)
	if err != nil {
		return nil, fmt.Errorf("el build no trae %s: %w", shellFile, err)
	}
	raw, err := fs.ReadFile(fsys, cspFile)
	if err != nil {
		return nil, fmt.Errorf("el build no trae %s: %w", cspFile, err)
	}
	csp, err := parseCSP(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", cspFile, err)
	}
	etags, err := hashFiles(fsys)
	if err != nil {
		return nil, fmt.Errorf("leyendo el build: %w", err)
	}
	return &handler{fsys: fsys, csp: csp, shell: shell, etags: etags}, nil
}

// parseCSP devuelve la política sin los espacios de los bordes (el build
// termina el archivo con \n). Tiene que ser una sola línea de texto: un \r o
// un \n en el medio partiría el header.
func parseCSP(raw []byte) (string, error) {
	csp := strings.TrimSpace(string(raw))
	if csp == "" {
		return "", errors.New("la CSP está vacía")
	}
	for _, c := range []byte(csp) {
		if (c < 0x20 && c != '\t') || c == 0x7f {
			return "", errors.New("la CSP tiene que ser una sola línea de texto, sin caracteres de control")
		}
	}
	return csp, nil
}

// hashFiles devuelve el ETag fuerte (los primeros 16 bytes del sha256, en
// hex) de cada archivo del build fuera de _app/immutable/.
func hashFiles(fsys fs.FS) (map[string]string, error) {
	etags := map[string]string{}
	err := fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && name+"/" == immutablePrefix:
			return fs.SkipDir
		case !d.Type().IsRegular() || name == cspFile:
			return nil
		}
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		etags[name] = `"` + hex.EncodeToString(sum[:16]) + `"`
		return nil
	})
	return etags, err
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hd := w.Header()
	hd.Set("Content-Security-Policy", h.csp)
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Referrer-Policy", "same-origin")
	hd.Set("Cache-Control", cacheRevalidate)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		hd.Set("Allow", "GET, HEAD")
		utils.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "método no permitido")
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/") // "" es la raíz
	if f, ok := h.open(name); ok {
		defer f.Close()
		h.serveFile(w, r, name, f)
		return
	}
	if name == "_app" || strings.HasPrefix(name, "_app/") {
		// Un chunk viejo nunca recibe HTML: con nosniff, el navegador lo
		// rechazaría como script con un error más confuso que un 404.
		http.Error(w, "no existe", http.StatusNotFound)
		return
	}
	hd.Set("Content-Type", contentType(shellFile))
	hd.Set("ETag", h.etags[shellFile])
	http.ServeContent(w, r, shellFile, time.Time{}, bytes.NewReader(h.shell))
}

// open abre el archivo del build que corresponde a name, si es un archivo que
// se sirve: la raíz y csp-header.txt no, y un directorio cuenta como que no
// existe (el router del panel resuelve esa ruta).
func (h *handler) open(name string) (fs.File, bool) {
	if name == "" || name == cspFile || !fs.ValidPath(name) {
		return nil, false
	}
	f, err := h.fsys.Open(name)
	if err != nil {
		return nil, false
	}
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		f.Close()
		return nil, false
	}
	return f, true
}

// serveFile responde f con el Content-Type de la tabla y la caché que le
// toca: un año para _app/immutable/, y no-cache con ETag para el resto.
// http.ServeContent atiende HEAD, Range y las condiciones (If-None-Match…);
// sin fecha, porque el embed no tiene.
func (h *handler) serveFile(w http.ResponseWriter, r *http.Request, name string, f fs.File) {
	hd := w.Header()
	hd.Set("Content-Type", contentType(name))
	if strings.HasPrefix(name, immutablePrefix) {
		hd.Set("Cache-Control", cacheImmutable)
	} else if etag, ok := h.etags[name]; ok {
		hd.Set("ETag", etag)
	}
	content, ok := f.(io.ReadSeeker)
	if !ok { // los archivos de embed y de fstest lo son; por si otro fs.FS no
		b, err := io.ReadAll(f)
		if err != nil {
			http.Error(w, "no se pudo leer el archivo", http.StatusInternalServerError)
			return
		}
		content = bytes.NewReader(b)
	}
	http.ServeContent(w, r, name, time.Time{}, content)
}

// contentTypes es la tabla de tipos del panel. No usa mime.TypeByExtension:
// Go completa su tabla con los archivos del sistema (/etc/mime.types), y con
// nosniff un tipo equivocado (un .js que no es JavaScript) rompe el panel.
var contentTypes = map[string]string{
	".html":        "text/html; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".json":        "application/json",
	".map":         "application/json",
	".webmanifest": "application/manifest+json",
	".txt":         "text/plain; charset=utf-8",
	".xml":         "application/xml",
	".svg":         "image/svg+xml",
	".png":         "image/png",
	".jpg":         "image/jpeg",
	".jpeg":        "image/jpeg",
	".gif":         "image/gif",
	".webp":        "image/webp",
	".avif":        "image/avif",
	".ico":         "image/x-icon",
	".woff":        "font/woff",
	".woff2":       "font/woff2",
	".ttf":         "font/ttf",
	".otf":         "font/otf",
	".wasm":        "application/wasm",
}

// contentType es el Content-Type de un archivo del build según su extensión
// (sin distinguir mayúsculas), o application/octet-stream si no está en la
// tabla.
func contentType(name string) string {
	if t, ok := contentTypes[strings.ToLower(path.Ext(name))]; ok {
		return t
	}
	return "application/octet-stream"
}
