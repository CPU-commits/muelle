// Package http es el servidor de la API /api/v1: arma el ServeMux y el Kit
// que usan los módulos, pone los headers de seguridad y revisa el Origin.
package http

import (
	"context"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
)

type Config struct {
	// PanelOrigin es MUELLE_PANEL_ORIGIN, ej. "https://panel.tudominio.cl",
	// en forma canónica (settings lo valida), o "" si no está (install
	// escribe MUELLE_PANEL_DOMAIN): entonces solo pasa el de ProxyOrigin.
	PanelOrigin   string
	SecureCookies bool
	Auth          utils.Authenticator
	Logger        *slog.Logger
	// ProxyOrigin es el origen en que muelle-caddy sirve el panel según los
	// ajustes (entity.ProxySettings.PanelOrigin), o "" sin dominio del panel.
	// Se pregunta en cada petición (admin lo tiene en caché), así sigue al
	// puerto HTTPS del proxy: después de cambiarlo (spec §16) pasa el origen
	// nuevo, y el viejo ya no. Se acepta además de PanelOrigin, y se consulta
	// solo cuando el Origin no es PanelOrigin. Opcional si hay PanelOrigin.
	ProxyOrigin func(ctx context.Context) string
	// Panel, opcional, sirve el panel (cmd/http/panel) en toda ruta fuera de
	// /api que no tiene una ruta registrada (contrato §5.5). Con nil, esas
	// rutas responden 404 no_route, como las de /api.
	Panel http.Handler
}

type Server struct {
	mux         *http.ServeMux
	kit         *utils.Kit
	origin      string
	proxyOrigin func(ctx context.Context) string
	panel       http.Handler
}

// NewServer arma el mux y el Kit, y registra los errores comunes:
// repository.ErrNotFound 404 not_found, ErrConflict 409 conflict, ErrNotEmpty 409 not_empty,
// entity.ErrUnauthenticated 401 unauthenticated, entity.ErrForbidden 403 forbidden,
// entity.ErrDockerUnavailable 503 docker_unavailable, entity.ErrNotImplemented 501 not_implemented,
// entity.ErrCloudflareUnavailable 502 cloudflare_unavailable.
// Hace panic si falta Auth, o PanelOrigin y ProxyOrigin a la vez: sin ellos
// ningún endpoint autenticado ni ninguna escritura del panel podría funcionar.
func NewServer(c Config) *Server {
	if c.Auth == nil {
		panic("http: NewServer sin Config.Auth")
	}
	if c.PanelOrigin == "" && c.ProxyOrigin == nil {
		panic("http: NewServer sin Config.PanelOrigin ni Config.ProxyOrigin")
	}
	log := c.Logger
	if log == nil {
		log = slog.Default()
	}
	mux := http.NewServeMux()
	k := utils.NewKit(mux, c.Auth, c.SecureCookies, log)
	k.RegisterError(repository.ErrNotFound, http.StatusNotFound, "not_found", "no existe")
	k.RegisterError(repository.ErrConflict, http.StatusConflict, "conflict", "ya existe")
	k.RegisterError(repository.ErrNotEmpty, http.StatusConflict, "not_empty", "primero hay que borrar lo que contiene")
	k.RegisterError(entity.ErrUnauthenticated, http.StatusUnauthorized, "unauthenticated", "inicia sesión")
	k.RegisterError(entity.ErrForbidden, http.StatusForbidden, "forbidden", "no tienes permiso para esta operación")
	k.RegisterError(entity.ErrDockerUnavailable, http.StatusServiceUnavailable, "docker_unavailable", "Docker no responde; reintenta en unos segundos")
	k.RegisterError(entity.ErrNotImplemented, http.StatusNotImplemented, "not_implemented", "todavía no está implementado en esta versión de muelle")
	k.RegisterError(entity.ErrCloudflareUnavailable, http.StatusBadGateway, "cloudflare_unavailable", "Cloudflare no responde; reintenta en unos minutos")
	return &Server{mux: mux, kit: k, origin: c.PanelOrigin, proxyOrigin: c.ProxyOrigin, panel: c.Panel}
}

// Kit es lo que reciben los módulos en RegisterRoutes.
func (s *Server) Kit() *utils.Kit { return s.kit }

// ServeHTTP pone X-Content-Type-Options: nosniff y, en /api/, Cache-Control: no-store;
// rechaza con 403 bad_origin las peticiones que modifican algo, sin Bearer y
// con un Origin que no es PanelOrigin ni el de ProxyOrigin.
// Una petición que no coincide con ninguna ruta responde en JSON, en vez del
// texto plano del ServeMux: 405 method_not_allowed (con el header Allow) si la
// ruta existe con otro método, y 404 no_route si no. no_route es distinto de
// not_found (un recurso que no existe en una ruta que sí existe): el panel lo
// usa para saber que un endpoint todavía no está en el backend.
// Fuera de /api, en vez de no_route responde Config.Panel, si está.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Cache-Control", "no-store")
	}
	if !s.originAllowed(r) {
		utils.WriteError(w, http.StatusForbidden, "bad_origin", "origen no permitido")
		return
	}
	if h, pattern := s.mux.Handler(r); pattern == "" {
		s.notMatched(w, r, h)
		return
	}
	s.mux.ServeHTTP(w, r)
}

// notMatched responde en JSON lo que el mux respondería en texto plano. Corre
// el handler del mux contra un statusRecorder solo para saber si es un 405 y
// con qué métodos.
func (s *Server) notMatched(w http.ResponseWriter, r *http.Request, h http.Handler) {
	rec := &statusRecorder{header: http.Header{}}
	h.ServeHTTP(rec, r)
	if rec.status == http.StatusMethodNotAllowed {
		if allow := rec.header.Get("Allow"); allow != "" {
			w.Header().Set("Allow", allow)
		}
		utils.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "método no permitido")
		return
	}
	if s.panel != nil && !isAPIPath(r.URL.Path) {
		s.panel.ServeHTTP(w, r)
		return
	}
	utils.WriteError(w, http.StatusNotFound, "no_route", "la ruta no existe")
}

// isAPIPath dice si p, ya limpio, es /api o algo debajo: esas rutas sin una
// ruta registrada son no_route, nunca el panel.
func isAPIPath(p string) bool {
	c := path.Clean(p)
	return c == "/api" || strings.HasPrefix(c, "/api/")
}

// originAllowed protege contra CSRF: toda petición que modifica algo y no
// trae un token Bearer tiene que venir del origen exacto del panel: el de
// ProxyOrigin (https://<panel_domain>, en el puerto HTTPS de los ajustes, y
// en ese host solo muelle-caddy sirve el panel) o PanelOrigin, que se
// configura a mano. Un navegador no puede agregar Authorization a una
// petición de otro origen sin CORS, y muelle no lo habilita.
func (s *Server) originAllowed(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	if _, ok := utils.BearerToken(r); ok {
		return true
	}
	// Sin Origin no pasa ninguna, aunque PanelOrigin o ProxyOrigin sean "".
	origin := r.Header.Get("Origin")
	switch {
	case origin == "":
		return false
	case origin == s.origin:
		return true
	case s.proxyOrigin == nil:
		return false
	}
	return origin == s.proxyOrigin(r.Context())
}

// statusRecorder es un http.ResponseWriter que guarda el status y los headers
// y descarta el body.
type statusRecorder struct {
	header http.Header
	status int
}

func (r *statusRecorder) Header() http.Header { return r.header }

func (r *statusRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.WriteHeader(http.StatusOK)
	return len(b), nil
}
