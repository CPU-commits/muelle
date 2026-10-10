package usecases

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/app/dto"
	"github.com/CPU-commits/muelle/src/internal/identity/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// rowsFrom devuelve las filas de auditoría nuevas desde before.
func (f *fixture) rowsFrom(t *testing.T, before int) []auditRow {
	t.Helper()
	return f.auditRows(t)[before:]
}

func expectRow(t *testing.T, r auditRow, action, target, details string, actor *int64) {
	t.Helper()
	if r.Action != action || r.Target != target || r.Details != details {
		t.Fatalf("fila=%+v, esperaba %s %s %s", r, action, target, details)
	}
	if actor == nil && (r.ActorType != "system" || r.ActorID != nil) {
		t.Fatalf("fila=%+v, esperaba actor system", r)
	}
	if actor != nil && (r.ActorType != "user" || r.ActorID == nil || *r.ActorID != *actor) {
		t.Fatalf("fila=%+v, esperaba actor user %d", r, *actor)
	}
}

func TestLoginAudit(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")

	f.login.Execute(ctx, login("felipe", "contraseña-mala!!", "1.1.1.1"))
	f.login.Execute(ctx, login("nadie", "contraseña-mala!!", "1.1.1.1"))
	rows := f.rowsFrom(t, 0)
	if len(rows) != 2 {
		t.Fatalf("audit=%+v", rows)
	}
	expectRow(t, rows[0], "auth.login_failed", "user:felipe", `{"ip":"1.1.1.1","reason":"credentials"}`, nil)
	expectRow(t, rows[1], "auth.login_failed", "user:nadie", `{"ip":"1.1.1.1","reason":"credentials"}`, nil)

	// El fallo que bloquea deja también auth.locked; los intentos rechazados
	// mientras dura el bloqueo no dejan nada.
	for range 3 {
		f.login.Execute(ctx, login("felipe", "contraseña-mala!!", "1.1.1.1"))
	}
	before := len(f.auditRows(t))
	f.login.Execute(ctx, login("felipe", "contraseña-mala!!", "1.1.1.1"))
	rows = f.rowsFrom(t, before)
	if len(rows) != 2 {
		t.Fatalf("audit=%+v", rows)
	}
	expectRow(t, rows[0], "auth.login_failed", "user:felipe", `{"ip":"1.1.1.1","reason":"credentials"}`, nil)
	expectRow(t, rows[1], "auth.locked", "user:felipe", `{"ip":"1.1.1.1","reason":"credentials"}`, nil)
	before = len(f.auditRows(t))
	for range 10 {
		f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	}
	if rows := f.rowsFrom(t, before); len(rows) != 0 {
		t.Fatalf("los intentos rechazados no se auditan: %+v", rows)
	}

	// Login correcto y logout: el actor es el usuario.
	f.advance(time.Minute)
	before = len(f.auditRows(t))
	res, err := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := f.authSession.Execute(ctx, res.Token)
	if err := f.logout.Execute(shared.WithPrincipal(ctx, p), dto.LogoutInput{SessionHash: p.SessionHash, Username: p.Username}); err != nil {
		t.Fatal(err)
	}
	rows = f.rowsFrom(t, before)
	if len(rows) != 2 {
		t.Fatalf("audit=%+v", rows)
	}
	expectRow(t, rows[0], "auth.login", "user:felipe", `{"ip":"1.1.1.1","mfa":false}`, &u.ID)
	expectRow(t, rows[1], "auth.logout", "user:felipe", `{}`, &u.ID)
	f.expectNoSecrets(t, testPassword, "contraseña-mala!!")
}

func TestLoginAuditWithTOTP(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	secret := f.enableTOTPFor(t, u.ID)
	before := len(f.auditRows(t)) // user.totp_enable

	res, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
	wrong, good := f.wrongCode(t, secret), f.code(t, secret)
	f.completeTOTP.Execute(ctx, completeTOTP(res.Token, wrong))
	if _, err := f.completeTOTP.Execute(ctx, completeTOTP(res.Token, good)); err != nil {
		t.Fatal(err)
	}
	// La contraseña correcta con 2FA pendiente todavía no es un login.
	rows := f.rowsFrom(t, before)
	if len(rows) != 2 {
		t.Fatalf("audit=%+v", rows)
	}
	expectRow(t, rows[0], "auth.login_failed", "user:felipe", `{"ip":"1.1.1.1","reason":"code"}`, nil)
	expectRow(t, rows[1], "auth.login", "user:felipe", `{"ip":"1.1.1.1","mfa":true}`, &u.ID)
	f.expectNoSecrets(t, testPassword, wrong, good, secret)
}

// Con 2FA, el 5º código malo deja auth.locked (motivo code) una sola vez.
func TestLoginAuditTOTPLockedOnlyOnTransition(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	secret := f.enableTOTPFor(t, u.ID)
	before := len(f.auditRows(t))
	for range 5 {
		res, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
		f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.wrongCode(t, secret)))
	}
	for range 5 {
		res, _ := f.login.Execute(ctx, login("felipe", testPassword, "1.1.1.1"))
		f.completeTOTP.Execute(ctx, completeTOTP(res.Token, f.code(t, secret)))
	}
	var failed, locked int
	for _, r := range f.rowsFrom(t, before) {
		switch r.Action {
		case "auth.login_failed":
			failed++
		case "auth.locked":
			locked++
			if r.Details != `{"ip":"1.1.1.1","reason":"code"}` {
				t.Fatalf("fila=%+v", r)
			}
		default:
			t.Fatalf("fila inesperada: %+v", r)
		}
	}
	if failed != 5 || locked != 1 {
		t.Fatalf("login_failed=%d locked=%d", failed, locked)
	}
}

// droppedRows devuelve los details de las filas auth.audit_dropped, y revisa
// que sean del sistema y con target auth.
func droppedRows(t *testing.T, rows []auditRow) []string {
	t.Helper()
	var out []string
	for _, r := range rows {
		if r.Action == "auth.audit_dropped" {
			expectRow(t, r, "auth.audit_dropped", "auth", r.Details, nil)
			out = append(out, r.Details)
		}
	}
	return out
}

// Los eventos que puede provocar cualquiera sin autenticarse pasan por un
// balde global: los que sobran se descartan, y se avisan a lo más una vez por
// intervalo, en el log y con una fila auth.audit_dropped con la cuenta (así
// el volumen queda acotado y el ataque se ve igual). Los eventos de un
// usuario autenticado no se limitan.
func TestUnauthenticatedAuditIsThrottled(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.user(t, "felipe")
	var logs bytes.Buffer
	la := NewLoginAudit(f.rec, service.NewTokenBucket(1, 3, f.clock), slog.New(slog.NewTextHandler(&logs, nil)), f.clock)
	uc := NewLogin(f.db, f.users, f.sessions, f.hasher, f.limiter, f.lockout, la, f.clock)

	for i := range 10 {
		uc.Execute(ctx, login(fmt.Sprintf("nadie%d", i), "x", "1.1.1.1"))
	}
	rows := f.auditRows(t)
	if len(rows) != 4 || rows[2].Action != "auth.login_failed" || rows[3].Action != "auth.audit_dropped" {
		t.Fatalf("esperaba las 3 de la ráfaga y el resumen del primer descarte: %+v", rows)
	}
	if got := droppedRows(t, rows); !slices.Equal(got, []string{`{"count":1}`}) {
		t.Fatalf("resúmenes=%v", got)
	}
	if n := strings.Count(logs.String(), "descartados"); n != 1 {
		t.Fatalf("avisos=%d, esperaba 1: %s", n, logs.String())
	}
	if !strings.Contains(logs.String(), "descartados=1") {
		t.Fatalf("el primer aviso sale con el primer descarte: %s", logs.String())
	}

	// Pasado el intervalo, el siguiente descarte avisa con la cuenta acumulada.
	f.advance(auditDropWarnEvery + time.Second) // también recarga el balde (3 tokens)
	before := len(f.auditRows(t))
	for i := range 4 {
		uc.Execute(ctx, login(fmt.Sprintf("otro%d", i), "x", "2.2.2.2"))
	}
	if n := strings.Count(logs.String(), "descartados"); n != 2 || !strings.Contains(logs.String(), "descartados=7") {
		t.Fatalf("avisos: %s", logs.String())
	}
	if rows := f.rowsFrom(t, before); len(rows) != 4 || !slices.Equal(droppedRows(t, rows), []string{`{"count":7}`}) {
		t.Fatalf("esperaba 3 fallos y el resumen de los 7 descartados: %+v", rows)
	}

	// Con el balde vacío, un login correcto se audita igual.
	before = len(f.auditRows(t))
	if _, err := uc.Execute(ctx, login("felipe", testPassword, "3.3.3.3")); err != nil {
		t.Fatal(err)
	}
	if rows := f.rowsFrom(t, before); len(rows) != 1 || rows[0].Action != "auth.login" {
		t.Fatalf("audit=%+v", rows)
	}
}

// timer es un temporizador que programó LoginAudit (la.after) en un test.
type timer struct {
	d  time.Duration
	fn func()
}

// Al terminar una avalancha, lo que quedó sin avisar se avisa al minuto
// siguiente aunque no llegue ningún evento más (un temporizador), con la
// cuenta y desde cuándo; el aviso sigue saliendo a lo más una vez por minuto.
func TestAuditDropsReportedAfterFloodEnds(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var logs bytes.Buffer
	la := NewLoginAudit(f.rec, service.NewTokenBucket(1, 1, f.clock), slog.New(slog.NewTextHandler(&logs, nil)), f.clock)
	if la.after != nil {
		t.Fatal("con un reloj manual no hay temporizador de verdad")
	}
	var timers []timer
	la.after = func(d time.Duration, fn func()) { timers = append(timers, timer{d, fn}) }
	warnings := func() int { return strings.Count(logs.String(), "descartados=") }
	start := f.now()

	la.failed(ctx, "a", "1.1.1.1", "credentials") // pasa (ráfaga de 1)
	for range 500 {                               // la avalancha, en el mismo segundo
		la.failed(ctx, "a", "1.1.1.1", "credentials")
	}
	if warnings() != 1 || !strings.Contains(logs.String(), "descartados=1 ") {
		t.Fatalf("el primer descarte se avisa enseguida, y nada más: %s", logs.String())
	}
	if len(timers) != 1 || timers[0].d != auditDropWarnEvery {
		t.Fatalf("un solo temporizador, al minuto del aviso: %+v", timers)
	}
	// Termina la avalancha y no llega nada más: al minuto se avisa el resto.
	f.advance(timers[0].d)
	timers[0].fn()
	if warnings() != 2 || !strings.Contains(logs.String(), "descartados=499 desde="+start.Format("2006-01-02T15:04:05.000Z07:00")) {
		t.Fatalf("al minuto se avisan los 499 que quedaban: %s", logs.String())
	}
	if len(timers) != 1 {
		t.Fatalf("sin nada pendiente no se programa otro: %+v", timers)
	}

	// Un aviso justo antes de que venza el temporizador: el temporizador no
	// repite el aviso, y lo que llega después sale al minuto siguiente.
	f.advance(time.Hour) // recarga el balde (1 token)
	la.failed(ctx, "a", "1.1.1.1", "credentials")
	la.failed(ctx, "a", "1.1.1.1", "credentials") // se avisa enseguida (pasó más de un minuto)
	la.failed(ctx, "a", "1.1.1.1", "credentials") // queda pendiente: temporizador
	if warnings() != 3 || len(timers) != 2 {
		t.Fatalf("avisos=%d temporizadores=%d: %s", warnings(), len(timers), logs.String())
	}
	f.advance(timers[1].d)
	la.failed(ctx, "a", "1.1.1.1", "credentials") // pasa: el minuto recargó el balde
	la.failed(ctx, "a", "1.1.1.1", "credentials") // vence el minuto: avisa los 2
	la.failed(ctx, "a", "1.1.1.1", "credentials") // otro más, pendiente (ya hay temporizador)
	if warnings() != 4 || !strings.Contains(logs.String(), "descartados=2 ") || len(timers) != 2 {
		t.Fatalf("avisos=%d temporizadores=%d: %s", warnings(), len(timers), logs.String())
	}
	timers[1].fn() // el temporizador viejo: todavía no pasó un minuto del último aviso
	if warnings() != 4 || len(timers) != 3 || timers[2].d != auditDropWarnEvery {
		t.Fatalf("no avisa antes de tiempo y se reprograma: avisos=%d %+v", warnings(), timers)
	}
	f.advance(timers[2].d)
	timers[2].fn()
	if warnings() != 5 || !strings.HasSuffix(strings.TrimSpace(logs.String()), "descartados=1 desde="+f.now().Add(-auditDropWarnEvery).Format("2006-01-02T15:04:05.000Z07:00")) {
		t.Fatalf("el último pendiente sale al minuto: %s", logs.String())
	}
	// Cada aviso deja también su fila en la auditoría.
	want := []string{`{"count":1}`, `{"count":499}`, `{"count":1}`, `{"count":2}`, `{"count":1}`}
	if got := droppedRows(t, f.auditRows(t)); !slices.Equal(got, want) {
		t.Fatalf("resúmenes=%v, esperaba %v", got, want)
	}
}

// Con el reloj del sistema, el aviso del final de la avalancha lo programa un
// temporizador de verdad (en la burbuja de synctest el tiempo es falso).
func TestAuditDropTimerWithSystemClock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var logs lockedBuffer
		rec := &memRecorder{}
		la := NewLoginAudit(rec, service.NewTokenBucket(1, 1, clock.System{}), slog.New(slog.NewTextHandler(&logs, nil)), clock.System{})
		ctx := context.Background()
		for range 11 { // 1 pasa y 10 se descartan: el primero se avisa enseguida
			la.failed(ctx, "a", "1.1.1.1", "credentials")
		}
		if n := strings.Count(logs.String(), "descartados="); n != 1 {
			t.Fatalf("avisos=%d: %s", n, logs.String())
		}
		time.Sleep(auditDropWarnEvery)
		synctest.Wait()
		if !strings.Contains(logs.String(), "descartados=9 ") {
			t.Fatalf("al minuto se avisan los 9 que quedaban: %s", logs.String())
		}
		var counts []int
		for _, e := range rec.of("auth.audit_dropped") {
			counts = append(counts, e.details.(map[string]int)["count"])
		}
		if !slices.Equal(counts, []int{1, 9}) {
			t.Fatalf("resúmenes=%v", counts)
		}
	})
}

// lockedBuffer es un bytes.Buffer que se puede escribir desde otra goroutine.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// memRecorder guarda los eventos de auditoría en memoria.
type memRecorder struct {
	mu     sync.Mutex
	events []memEvent
}

type memEvent struct {
	action, target string
	details        any
}

func (r *memRecorder) Record(_ context.Context, action, target string, details any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, memEvent{action, target, details})
}

func (r *memRecorder) of(action string) []memEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []memEvent
	for _, e := range r.events {
		if e.action == action {
			out = append(out, e)
		}
	}
	return out
}

// Los fallos del código TOTP solo los provoca quien sabe la contraseña, y el
// bloqueo por usuario ya los acota: se auditan siempre, sin pasar por el
// balde. Una avalancha de contraseñas malas que lo vacía no esconde un ataque
// al 2FA.
func TestCodeFailuresAuditedWithBucketDrained(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	u := f.user(t, "felipe")
	secret := f.enableTOTPFor(t, u.ID)
	la := NewLoginAudit(f.rec, service.NewTokenBucket(1, 3, f.clock), slog.New(slog.NewTextHandler(io.Discard, nil)), f.clock)
	loginUC := NewLogin(f.db, f.users, f.sessions, f.hasher, f.limiter, f.lockout, la, f.clock)
	complete := NewCompleteTOTP(f.db, f.users, f.sessions, f.cipher, f.totp, f.limiter, f.totpLock, f.attempts, f.replay, la, f.clock)

	// La avalancha: contraseñas malas de usuarios inventados vacían el balde.
	for i := range 10 {
		loginUC.Execute(ctx, login(fmt.Sprintf("nadie%d", i), "x", fmt.Sprintf("2001:db8:%x::1", i+1)))
	}
	before := len(f.auditRows(t))
	// El ataque al 2FA: 5 códigos malos, cada uno con su sesión pendiente y su IP.
	for i := range 5 {
		ip := fmt.Sprintf("203.0.113.%d", i+1)
		res, err := loginUC.Execute(ctx, login("felipe", testPassword, ip))
		if err != nil || !res.MFARequired {
			t.Fatalf("login %d: res=%+v err=%v", i+1, res, err)
		}
		if _, err := complete.Execute(ctx, completeFrom(res.Token, f.wrongCode(t, secret), ip)); !errors.Is(err, service.ErrInvalidCode) {
			t.Fatalf("código malo %d: got %v", i+1, err)
		}
	}
	var failed, locked int
	for _, r := range f.rowsFrom(t, before) {
		switch {
		case r.Action == "auth.login_failed" && strings.Contains(r.Details, `"reason":"code"`):
			failed++
		case r.Action == "auth.locked" && strings.Contains(r.Details, `"reason":"code"`):
			locked++
		default:
			t.Fatalf("fila inesperada: %+v", r)
		}
	}
	if failed != 5 || locked != 1 {
		t.Fatalf("con el balde vacío: login_failed(code)=%d/5 locked(code)=%d/1", failed, locked)
	}
}

// Si alguien escribe su contraseña en el campo del usuario, no queda en la
// auditoría: un nombre que no es un usuario válido se guarda como <inválido>.
func TestLoginAuditHidesInvalidUsernames(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, name := range []string{"mi contraseña secreta", "Felipe", "\xff" + strings.Repeat("ñ", 100), ""} {
		f.login.Execute(ctx, login(name, "x", "1.1.1.1"))
	}
	f.login.Execute(ctx, login("nadie", "x", "1.1.1.1"))
	rows := f.auditRows(t)
	if len(rows) != 5 {
		t.Fatalf("audit=%+v", rows)
	}
	for _, r := range rows[:4] {
		if r.Action != "auth.login_failed" || r.Target != "user:<inválido>" {
			t.Fatalf("fila=%+v", r)
		}
	}
	if rows[4].Target != "user:nadie" {
		t.Fatalf("un nombre válido que no existe se guarda tal cual: %+v", rows[4])
	}
	f.expectNoSecrets(t, "contraseña secreta")
}

// expectNoSecrets revisa que ninguna fila de auditoría contenga contraseñas,
// códigos ni secretos.
func (f *fixture) expectNoSecrets(t *testing.T, secrets ...string) {
	t.Helper()
	for _, r := range f.auditRows(t) {
		for _, s := range secrets {
			if strings.Contains(r.Target, s) || strings.Contains(r.Details, s) {
				t.Fatalf("la auditoría filtra %q: %+v", s, r)
			}
		}
	}
}
