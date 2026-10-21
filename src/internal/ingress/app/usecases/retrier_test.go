package usecases

import (
	"context"
	"testing"
	"time"
)

// eventually repite check cada 2 ms hasta que da true o pasa timeout.
func eventually(t *testing.T, timeout time.Duration, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatalf("%s: no pasó en %s", what, timeout)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// runInBackground corre run hasta el final del test, y al final cancela y
// espera a que vuelva.
func runInBackground(t *testing.T, run func(ctx context.Context)) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("Run no volvió al cancelar el ctx")
		}
	})
}

func TestNextBackoff(t *testing.T) {
	const minD, maxD = time.Second, 60 * time.Second
	cases := []struct{ in, want time.Duration }{
		{0, time.Second}, {time.Second, 2 * time.Second}, {2 * time.Second, 4 * time.Second},
		{16 * time.Second, 32 * time.Second}, {32 * time.Second, 60 * time.Second}, {60 * time.Second, 60 * time.Second},
	}
	for _, c := range cases {
		if got := nextBackoff(c.in, minD, maxD); got != c.want {
			t.Errorf("nextBackoff(%s) = %s, esperaba %s", c.in, got, c.want)
		}
	}
}

// Un Sync que falla despierta al Retrier, que reintenta con backoff hasta
// que Caddy acepta (D14).
func TestRetrierRecovers(t *testing.T) {
	f := newSyncFixture(t)
	f.enable(t, nil)
	var r *Retrier
	r = NewRetrier(func(ctx context.Context) error { return f.syncer.Sync(ctx, false) }, time.Millisecond, 4*time.Millisecond)
	f.syncer.onFail = r.Kick
	f.caddy.Reject("todavía no")
	if err := f.syncer.Sync(context.Background(), false); err == nil {
		t.Fatal("debió fallar")
	}
	runInBackground(t, r.Run)

	eventually(t, 2*time.Second, "reintentos mientras rechaza", func() bool { return len(f.caddy.Loads()) >= 4 })
	f.caddy.Accept()
	eventually(t, 2*time.Second, "synced", f.state.Synced)
	n := len(f.caddy.Loads())
	time.Sleep(30 * time.Millisecond)
	if got := len(f.caddy.Loads()); got != n {
		t.Fatalf("después de funcionar, deja de reintentar: %d → %d loads", n, got)
	}
}

// Kick no bloquea, aunque nadie corra Run.
func TestRetrierKickDoesNotBlock(t *testing.T) {
	r := NewRetrier(func(context.Context) error { return nil }, time.Millisecond, time.Millisecond)
	done := make(chan struct{})
	go func() {
		for range 10 {
			r.Kick()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Kick bloqueó")
	}
}
