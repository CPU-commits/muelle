//go:build !panel

package panel

import "net/http"

// notEmbeddedPage es lo que responde un binario sin el panel en toda ruta
// fuera de /api.
const notEmbeddedPage = `<!doctype html>
<html lang="es">
<meta charset="utf-8">
<title>muelle sin panel</title>
<h1>Este binario de muelle no trae el panel</h1>
<p>Se compiló sin <code>-tags panel</code>, así que no sirve el panel web. La API sigue en <code>/api/v1</code>.</p>
<p>La imagen de muelle trae el panel. Para probarlo en desarrollo, ver "Con el panel" en el README.</p>
`

// Embedded, en un binario compilado sin -tags panel, devuelve un handler que
// responde 404 con una página corta que lo explica, para cualquier método y
// ruta (contrato §5.5). Nunca falla.
func Embedded() (http.Handler, error) {
	return http.HandlerFunc(serveNotEmbedded), nil
}

func serveNotEmbedded(w http.ResponseWriter, r *http.Request) {
	hd := w.Header()
	hd.Set("Content-Type", "text/html; charset=utf-8")
	hd.Set("Content-Security-Policy", "default-src 'none'")
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Referrer-Policy", "same-origin")
	hd.Set("Cache-Control", cacheRevalidate)
	w.WriteHeader(http.StatusNotFound)
	if r.Method != http.MethodHead {
		w.Write([]byte(notEmbeddedPage))
	}
}
