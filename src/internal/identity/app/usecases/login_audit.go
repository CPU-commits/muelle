package usecases

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

const (
	// maxAuditName es el largo máximo (en bytes) del nombre de usuario o la
	// IP que se guardan en la auditoría: los del login pueden ser cualquier texto.
	maxAuditName = 64
	// auditDropWarnEvery: los eventos descartados por el límite se avisan en
	// el log a lo más una vez por este intervalo, con la cuenta acumulada.
	auditDropWarnEvery = time.Minute
)

// LoginAudit registra los eventos de login. Nunca guarda contraseñas, códigos
// ni secretos: solo el nombre que se intentó (o <inválido> si no es un nombre
// de usuario válido), la IP y el motivo.
//
//   - auth.login: se abrió una sesión completa (mfa: si fue con el código TOTP).
//   - auth.login_failed: contraseña o código incorrectos (reason: credentials | code).
//   - auth.locked: un fallo dejó bloqueado el par (usuario, IP) o el 2FA del usuario.
//   - auth.logout: se cerró la sesión.
//
// Los del paso de la contraseña (auth.login_failed y auth.locked con reason
// credentials) los puede provocar cualquiera sin autenticarse, así que pasan
// por un balde global (bucket); los que sobran se descartan con un aviso en el
// log, para que muchas IPs no llenen el disco. Los del paso del código (reason
// code) se registran siempre: solo llega a ese paso quien sabe la contraseña,
// el bloqueo por usuario ya los acota (unos 33 códigos malos por día y
// usuario), y una avalancha de contraseñas malas que vacía el balde no puede
// esconder un ataque al 2FA.
type LoginAudit struct {
	rec    audit.Recorder
	bucket *service.TokenBucket
	log    *slog.Logger
	clock  clock.Clock

	mu       sync.Mutex
	dropped  int
	lastWarn time.Time
}

func NewLoginAudit(rec audit.Recorder, bucket *service.TokenBucket, log *slog.Logger, c clock.Clock) *LoginAudit {
	return &LoginAudit{rec: rec, bucket: bucket, log: log, clock: c}
}

// reasonCode es el motivo de los fallos del paso del código TOTP, al que solo
// llega quien sabe la contraseña (el de la contraseña es credentials).
const reasonCode = "code"

// failed registra una contraseña (reason credentials) o un código (reason
// code) incorrectos.
func (a *LoginAudit) failed(ctx context.Context, username, ip, reason string) {
	a.attempt(ctx, "auth.login_failed", username, ip, reason)
}

// locked se llama solo cuando un fallo deja bloqueado el par (usuario, IP) o
// el 2FA del usuario, no en cada intento rechazado mientras dura el bloqueo.
func (a *LoginAudit) locked(ctx context.Context, username, ip, reason string) {
	a.attempt(ctx, "auth.locked", username, ip, reason)
}

// attempt registra un evento de un intento fallido: los del código, siempre;
// los de la contraseña (o cualquier otro motivo), si el balde lo permite.
func (a *LoginAudit) attempt(ctx context.Context, action, username, ip, reason string) {
	details := map[string]string{"ip": auditText(ip), "reason": reason}
	if reason == reasonCode {
		a.rec.Record(ctx, action, auditUser(username), details)
		return
	}
	a.unauthenticated(ctx, action, username, details)
}

// unauthenticated registra un evento que puede provocar cualquiera, si el
// balde global lo permite.
func (a *LoginAudit) unauthenticated(ctx context.Context, action, username string, details map[string]string) {
	if !a.bucket.Allow() {
		a.drop()
		return
	}
	a.rec.Record(ctx, action, auditUser(username), details)
}

// drop cuenta un evento descartado y avisa en el log a lo más una vez por
// auditDropWarnEvery, con la cuenta acumulada desde el aviso anterior.
func (a *LoginAudit) drop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dropped++
	now := a.clock.Now()
	if !a.lastWarn.IsZero() && now.Sub(a.lastWarn) < auditDropWarnEvery {
		return
	}
	a.log.Warn("auditoría: se descartaron eventos de login sin autenticar por exceso", "descartados", a.dropped)
	a.dropped, a.lastWarn = 0, now
}

// login registra la sesión nueva a nombre del usuario.
func (a *LoginAudit) login(ctx context.Context, u entity.User, ip string, mfa bool) {
	ctx = shared.WithPrincipal(ctx, shared.Principal{UserID: u.ID, Username: u.Username})
	a.rec.Record(ctx, "auth.login", auditUser(u.Username), map[string]any{"ip": auditText(ip), "mfa": mfa})
}

func (a *LoginAudit) logout(ctx context.Context, username string) {
	a.rec.Record(ctx, "auth.logout", auditUser(username), nil)
}

// invalidUsername reemplaza en la auditoría un nombre que no es un usuario
// válido: suele ser una contraseña escrita en el campo equivocado.
const invalidUsername = "<inválido>"

func auditUser(username string) string {
	if validate.Username(username) != nil {
		return "user:" + invalidUsername
	}
	return "user:" + auditText(username)
}

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
