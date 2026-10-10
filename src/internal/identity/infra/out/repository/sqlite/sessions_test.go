package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/identity/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
)

func TestSessions(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	st := r.sessions
	u := r.user(t, "felipe", nil)
	now := t0

	err := st.Create(ctx, entity.Session{IDHash: "s1", UserID: u.ID, CreatedAt: now, LastSeenAt: now,
		ExpiresAt: now.Add(7 * 24 * time.Hour), UserAgent: "curl", IP: "1.2.3.4"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Create(ctx, entity.Session{IDHash: "pending", UserID: u.ID, MFAPending: true, CreatedAt: now, LastSeenAt: now,
		ExpiresAt: now.Add(5 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := st.Create(ctx, entity.Session{IDHash: "s1", UserID: u.ID, CreatedAt: now, LastSeenAt: now, ExpiresAt: now}); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("hash repetido: esperaba ErrConflict, got %v", err)
	}

	ss, err := st.Get(ctx, "s1")
	if err != nil || ss.UserID != u.ID || ss.MFAPending || ss.IP != "1.2.3.4" || ss.UserAgent != "curl" ||
		!ss.CreatedAt.Equal(now) || !ss.ExpiresAt.Equal(now.Add(7*24*time.Hour)) {
		t.Fatalf("ss=%+v err=%v", ss, err)
	}
	p, _ := st.Get(ctx, "pending")
	if !p.MFAPending {
		t.Fatal("mfa_pending no se guardó")
	}

	if err := st.Touch(ctx, "s1", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	ss, _ = st.Get(ctx, "s1")
	if !ss.LastSeenAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("last_seen=%v", ss.LastSeenAt)
	}

	// 13 h después: s1 lleva 12 h inactiva y "pending" venció.
	n, err := st.DeleteExpired(ctx, now.Add(13*time.Hour+time.Second), 12*time.Hour)
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if _, err := st.Get(ctx, "s1"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("esperaba ErrNotFound, got %v", err)
	}
	if err := st.Delete(ctx, "no-existe"); err != nil {
		t.Fatalf("borrar inexistente no debe fallar: %v", err)
	}
}

func TestDeleteExpiredKeepsLiveSessions(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	u := r.user(t, "felipe", nil)
	r.sessions.Create(ctx, entity.NewSession("vive", u.ID, false, t0, 7*24*time.Hour, "", ""))
	r.sessions.Create(ctx, entity.NewSession("pendiente", u.ID, true, t0, 5*time.Minute, "", ""))

	if n, err := r.sessions.DeleteExpired(ctx, t0.Add(4*time.Minute), 12*time.Hour); err != nil || n != 0 {
		t.Fatalf("a los 4 min ninguna venció: n=%d err=%v", n, err)
	}
	// A los 5 min vence la pendiente; la completa sigue viva.
	if n, err := r.sessions.DeleteExpired(ctx, t0.Add(5*time.Minute), 12*time.Hour); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if _, err := r.sessions.Get(ctx, "pendiente"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("la pendiente debió borrarse, got %v", err)
	}
	if _, err := r.sessions.Get(ctx, "vive"); err != nil {
		t.Fatalf("la sesión completa sigue viva: %v", err)
	}
}

// ConsumePending borra la sesión pendiente una sola vez: así dos CompleteTOTP
// a la vez no pueden canjear la misma sesión.
func TestConsumePending(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	u := r.user(t, "felipe", nil)
	r.sessions.Create(ctx, entity.NewSession("pendiente", u.ID, true, t0, 5*time.Minute, "", ""))
	r.sessions.Create(ctx, entity.NewSession("completa", u.ID, false, t0, time.Hour, "", ""))

	if err := r.sessions.ConsumePending(ctx, "completa", t0); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("una sesión completa no se consume: got %v", err)
	}
	if err := r.sessions.ConsumePending(ctx, "pendiente", t0.Add(5*time.Minute)); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("una pendiente vencida no se consume: got %v", err)
	}
	if err := r.sessions.ConsumePending(ctx, "pendiente", t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := r.sessions.ConsumePending(ctx, "pendiente", t0.Add(time.Minute)); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("la segunda vez: esperaba ErrNotFound, got %v", err)
	}
	if _, err := r.sessions.Get(ctx, "completa"); err != nil {
		t.Fatalf("la sesión completa sigue: %v", err)
	}
}

func TestDeleteSessionsByUser(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	ana := r.user(t, "ana", nil)
	beto := r.user(t, "beto", nil)
	r.sessions.Create(ctx, entity.NewSession("ana-1", ana.ID, false, t0, time.Hour, "", ""))
	r.sessions.Create(ctx, entity.NewSession("ana-2", ana.ID, true, t0, time.Minute, "", ""))
	r.sessions.Create(ctx, entity.NewSession("beto", beto.ID, false, t0, time.Hour, "", ""))

	n, err := r.sessions.DeleteByUser(ctx, ana.ID)
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	for _, h := range []string{"ana-1", "ana-2"} {
		if _, err := r.sessions.Get(ctx, h); !errors.Is(err, repository.ErrNotFound) {
			t.Fatalf("%s debió borrarse, got %v", h, err)
		}
	}
	if _, err := r.sessions.Get(ctx, "beto"); err != nil {
		t.Fatalf("la sesión de otro usuario sigue: %v", err)
	}
}

func TestDeleteUserCascadesSessions(t *testing.T) {
	r := newRepos(t)
	ctx := context.Background()
	u := r.user(t, "felipe", nil)
	r.sessions.Create(ctx, entity.Session{IDHash: "s1", UserID: u.ID, CreatedAt: t0, LastSeenAt: t0, ExpiresAt: t0.Add(time.Hour)})
	if err := r.users.Delete(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.sessions.Get(ctx, "s1"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("la sesión debió borrarse en cascada, got %v", err)
	}
}
