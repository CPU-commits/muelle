// Package http es el servidor de la API /api/v1: arma el ServeMux y el Kit
// que usan los módulos, pone los headers de seguridad y revisa el Origin.
package http

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
)

type Config struct {
	PanelOrigin   string // ej. "https://panel.tudominio.cl", en forma canónica (settings lo valida)
	SecureCookies bool
	Auth          utils.Authenticator
	Logger        *slog.Logger
}

type Server struct {
	mux    *http.ServeMux
	kit    *utils.Kit
	origin string
}

// NewServer arma el mux y el Kit, y registra los errores comunes:
// repository.ErrNotFound 404 not_found, ErrConflict 409 conflict, ErrNotEmpty 409 not_empty,
// entity.ErrUnauthenticated 401 unauthenticated, entity.ErrForbidden 403 forbidden.
// Hace panic si falta Auth o PanelOrigin: sin ellos ningún endpoint
// autenticado ni ninguna escritura del panel podría funcionar.
func NewServer(c Config) *Server {
	if c.Auth == nil {
		panic("http: NewServer sin Config.Auth")
	}
	if c.PanelOrigin == "" {
		panic("http: NewServer sin Config.PanelOrigin")
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
	return &Server{mux: mux, kit: k, origin: c.PanelOrigin}
}

// Kit es lo que reciben los módulos en RegisterRoutes.
func (s *Server) Kit() *utils.Kit { return s.kit }

// ServeHTTP pone X-Content-Type-Options: nosniff y, en /api/, Cache-Control: no-store;
// rechaza con 403 bad_origin las peticiones que modifican algo, sin Bearer y con un Origin distinto de PanelOrigin.
// Una petición que no coincide con ninguna ruta responde en JSON, en vez del
// texto plano del ServeMux: 405 method_not_allowed (con el header Allow) si la
// ruta existe con otro método, y 404 not_found si no.
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
// el handler del mux contra un recorder solo para saber si es un 405 y con
// qué métodos.
func (s *Server) notMatched(w http.ResponseWriter, r *http.Request, h http.Handler) {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code == http.StatusMethodNotAllowed {
		if allow := rec.Header().Get("Allow"); allow != "" {
			w.Header().Set("Allow", allow)
		}
		utils.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "método no permitido")
		return
	}
	utils.WriteError(w, http.StatusNotFound, "not_found", "no existe")
}

// originAllowed protege contra CSRF: toda petición que modifica algo y no
// trae un token Bearer tiene que venir del origen exacto del panel. Un
// navegador no puede agregar Authorization a una petición de otro origen sin
// CORS, y muelle no lo habilita.
func (s *Server) originAllowed(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	if _, ok := utils.BearerToken(r); ok {
		return true
	}
	// Con PanelOrigin vacío (mal configurado) no pasa ninguna.
	return s.origin != "" && r.Header.Get("Origin") == s.origin
}
