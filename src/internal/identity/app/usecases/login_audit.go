package usecases

import (
	"context"
	"strings"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/package/audit"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// maxAuditName es el largo máximo (en bytes) del nombre de usuario o la IP
// que se guardan en la auditoría: los del login pueden ser cualquier texto.
const maxAuditName = 64

// loginAudit registra los eventos de login. Nunca guarda contraseñas, códigos
// ni secretos: solo el nombre que se intentó, la IP y el motivo.
//
//   - auth.login: se abrió una sesión completa (mfa: si fue con el código TOTP).
//   - auth.login_failed: contraseña o código incorrectos (reason: credentials | code).
//   - auth.locked: un intento rechazado porque el par (usuario, IP) está bloqueado.
//   - auth.logout: se cerró la sesión.
type loginAudit struct {
	rec audit.Recorder
}

func (a loginAudit) failed(ctx context.Context, username, ip, reason string) {
	a.rec.Record(ctx, "auth.login_failed", auditUser(username), map[string]string{"ip": auditText(ip), "reason": reason})
}

func (a loginAudit) locked(ctx context.Context, username, ip string) {
	a.rec.Record(ctx, "auth.locked", auditUser(username), map[string]string{"ip": auditText(ip)})
}

// login registra la sesión nueva a nombre del usuario.
func (a loginAudit) login(ctx context.Context, u entity.User, ip string, mfa bool) {
	ctx = shared.WithPrincipal(ctx, shared.Principal{UserID: u.ID, Username: u.Username})
	a.rec.Record(ctx, "auth.login", auditUser(u.Username), map[string]any{"ip": auditText(ip), "mfa": mfa})
}

func (a loginAudit) logout(ctx context.Context, username string) {
	a.rec.Record(ctx, "auth.logout", auditUser(username), nil)
}

func auditUser(username string) string { return "user:" + auditText(username) }

// auditText deja s como UTF-8 válido (json/v2 no serializa otra cosa) y lo
// recorta a maxAuditName bytes sin cortar un carácter por la mitad.
func auditText(s string) string {
	s = strings.ToValidUTF8(s, "�")
	if len(s) <= maxAuditName {
		return s
	}
	cut := 0
	for i := range s {
		if i > maxAuditName {
			break
		}
		cut = i
	}
	return s[:cut]
}
