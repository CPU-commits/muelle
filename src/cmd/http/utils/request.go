package utils

import (
	"net"
	"net/http"
	"strings"
	"time"
)

// ClientIP usa X-Real-IP, que pone Caddy (el único que llega a muelle en
// producción, §8 del spec). Sin ese header usa la IP de RemoteAddr.
func ClientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// BearerToken devuelve el token de "Authorization: Bearer <token>", si hay uno.
func BearerToken(r *http.Request) (string, bool) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return "", false
	}
	return token, true
}

// SetSessionCookie pone la cookie de sesión: HttpOnly, Secure según la
// configuración, SameSite=Strict y Path=/.
func (k *Kit) SetSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: token, Path: "/", Expires: expires,
		HttpOnly: true, Secure: k.secureCookies, SameSite: http.SameSiteStrictMode,
	})
}

// ClearSessionCookie le pide al navegador que borre la cookie de sesión.
func (k *Kit) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: k.secureCookies, SameSite: http.SameSiteStrictMode,
	})
}
