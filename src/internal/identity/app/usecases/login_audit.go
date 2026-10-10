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
	// auditDropWarnEvery: los eventos descartados por el límite se avisan a lo
	// más una vez por este intervalo, con la cuenta acumulada.
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
//   - auth.audit_dropped: se descartaron eventos por el límite (count: cuántos).
//
// Los del paso de la contraseña (auth.login_failed y auth.locked con reason
// credentials) los puede provocar cualquiera sin autenticarse, así que pasan
// por un balde global (bucket); los que sobran se descartan, para que muchas
// IPs no llenen el disco. Los descartes se avisan a lo más una vez por
// auditDropWarnEvery, en el log y con una fila auth.audit_dropped con la
// cuenta (así el volumen queda acotado y el ataque se ve igual): el primero
// enseguida, y lo que se acumula después, al vencer el intervalo, aunque la
// avalancha haya terminado y no llegue ningún evento más. Los del paso del
// código (reason
// code) se registran siempre: solo llega a ese paso quien sabe la contraseña,
// el bloqueo por usuario ya los acota (unos 33 códigos malos por día y
// usuario), y una avalancha de contraseñas malas que vacía el balde no puede
// esconder un ataque al 2FA.
type LoginAudit struct {
	rec    audit.Recorder
	bucket *service.TokenBucket
	log    *slog.Logger
	clock  clock.Clock

	// after programa fn para dentro de d: el aviso de lo que quedó al
	// terminar una avalancha. Es time.AfterFunc con el reloj del sistema; con
	// otro reloj (un test) es nil, y lo pendiente se avisa con el próximo
	// descarte.
	after func(d time.Duration, fn func())

	mu        sync.Mutex
	dropped   int       // descartados todavía sin avisar
	since     time.Time // cuándo fue el primero de ellos
	lastWarn  time.Time
	scheduled bool // hay un aviso programado con after
}

func NewLoginAudit(rec audit.Recorder, bucket *service.TokenBucket, log *slog.Logger, c clock.Clock) *LoginAudit {
	a := &LoginAudit{rec: rec, bucket: bucket, log: log, clock: c}
	if _, ok := c.(clock.System); ok {
		a.after = func(d time.Duration, fn func()) { time.AfterFunc(d, fn) }
	}
	return a
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

// drop cuenta un evento descartado. Si pasó auditDropWarnEvery desde el
// último aviso (o nunca hubo uno), avisa ya; si no, programa el aviso para
// cuando venza el intervalo.
func (a *LoginAudit) drop() {
	a.mu.Lock()
	now := a.clock.Now()
	if a.dropped == 0 {
		a.since = now
	}
	a.dropped++
	n, since, due := a.takeDue(now)
	if !due {
		a.schedule(now)
	}
	a.mu.Unlock()
	if due {
		a.warn(n, since)
	}
}

// flush es el aviso programado: avisa lo pendiente, o se reprograma si otro
// aviso salió hace menos de auditDropWarnEvery.
func (a *LoginAudit) flush() {
	a.mu.Lock()
	a.scheduled = false
	now := a.clock.Now()
	n, since, due := a.takeDue(now)
	if !due {
		a.schedule(now)
	}
	a.mu.Unlock()
	if due {
		a.warn(n, since)
	}
}

// takeDue saca los descartes pendientes si toca avisarlos: hay alguno y pasó
// auditDropWarnEvery desde el último aviso. Se llama con a.mu tomado.
func (a *LoginAudit) takeDue(now time.Time) (n int, since time.Time, due bool) {
	if a.dropped == 0 {
		return 0, time.Time{}, false
	}
	if !a.lastWarn.IsZero() && now.Sub(a.lastWarn) < auditDropWarnEvery {
		return 0, time.Time{}, false
	}
	n, since = a.dropped, a.since
	a.dropped, a.lastWarn = 0, now
	return n, since, true
}

// schedule programa el aviso de lo pendiente para cuando venza el intervalo,
// si no hay uno programado ya. Se llama con a.mu tomado.
func (a *LoginAudit) schedule(now time.Time) {
	if a.dropped == 0 || a.scheduled || a.after == nil {
		return
	}
	a.scheduled = true
	a.after(a.lastWarn.Add(auditDropWarnEvery).Sub(now), a.flush)
}

// warn avisa n eventos descartados desde since: en el log y con una fila de
// auditoría del sistema (fuera de cualquier transacción y sin el Principal
// del pedido que la disparó).
func (a *LoginAudit) warn(n int, since time.Time) {
	a.log.Warn("auditoría: se descartaron eventos de login sin autenticar por exceso", "descartados", n, "desde", since)
	a.rec.Record(context.Background(), "auth.audit_dropped", "auth", map[string]int{"count": n})
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
