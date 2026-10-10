package utils

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// ClientIP devuelve la IP del cliente. Confía en X-Real-IP, que pone Caddy
// (el único que llega a muelle en producción, §8 del spec), solo si la
// conexión viene de loopback o de una red privada y el header es una IP sin
// zona; la devuelve en forma canónica. Si no, usa la IP de RemoteAddr: desde
// una IP pública el header lo escribe el propio cliente.
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if remote, err := netip.ParseAddr(host); err == nil && (remote.IsLoopback() || remote.IsPrivate()) {
		ip, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("X-Real-IP")))
		if err == nil && ip.Zone() == "" {
			return ip.Unmap().String()
		}
	}
	return host
}

// BearerToken devuelve el token de "Authorization: Bearer <token>", si hay
// uno. El esquema no distingue mayúsculas (RFC 9110 §11.1).
func BearerToken(r *http.Request) (string, bool) {
	const scheme = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(scheme) || !strings.EqualFold(h[:len(scheme)], scheme) {
		return "", false
	}
	return h[len(scheme):], true
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
