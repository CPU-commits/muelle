package usecases

import (
	"context"
	"testing"
	"time"

	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// newRecreatorFixture deja muelle-caddy corriendo con 8080/8443 y la
// configuración en Caddy (synced), y un Recreator con ese delay corriendo
// hasta el final del test.
func newRecreatorFixture(t *testing.T, delay time.Duration) (*lifecycleFixture, *Recreator) {
	t.Helper()
	f := newLifecycleFixture(t)
	f.enableCaddy(t, nil)
	f.docker.SetImagePresent(defCaddyImage)
	ctx := context.Background()
	if _, err := f.ensure.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.syncer.Sync(ctx, false); err != nil {
		t.Fatal(err)
	}
	if !f.state.Synced() {
		t.Fatal("el estado de partida es synced")
	}
	r := NewRecreator(f.ensure, f.syncer, f.state, delay, time.Millisecond, 4*time.Millisecond, f.slog)
	runInBackground(t, r.Run)
	return f, r
}

// changePorts guarda otro puerto HTTP: muelle-caddy se tiene que recrear.
func (f *lifecycleFixture) changePorts(t *testing.T, http int) {
	t.Helper()
	f.enableCaddy(t, func(p *shared.ProxySettings) { p.HTTPPort = http })
}

// Dos Schedule seguidos dan una sola recreación, con los ajustes de ese
// momento; mientras tanto GET /proxy dice recreating, y al final synced con
// un Load después de recrear.
func TestRecreatorDebounce(t *testing.T) {
	f, r := newRecreatorFixture(t, 20*time.Millisecond)
	loads := len(f.caddy.Loads())
	f.changePorts(t, 9080)
	r.Schedule()
	if status, _, _ := f.state.Snapshot(true); status != ProxyRecreating {
		t.Fatalf("recién agendada: %s", status)
	}
	r.Schedule()

	eventually(t, 2*time.Second, "la recreación termina", func() bool { return !f.state.Recreating() })
	if n := f.creates(); n != 2 {
		t.Fatalf("una sola recreación: CreateContainer %d veces", n)
	}
	if c := f.container(t); c.Ports[0].HostPort != 9080 {
		t.Fatalf("con los puertos nuevos: %+v", c.Ports)
	}
	if status, _, _ := f.state.Snapshot(true); status != ProxySynced {
		t.Fatalf("al final: %s", status)
	}
	if len(f.caddy.Loads()) <= loads {
		t.Fatal("falta el Load después de recrear")
	}
}

// Un Schedule durante la recreación deja recreating hasta la segunda (las
// generaciones de ProxyState).
func TestRecreatorKeepsLaterSchedule(t *testing.T) {
	f, r := newRecreatorFixture(t, 20*time.Millisecond)
	loads := len(f.caddy.Loads())
	f.caddy.Delay(100 * time.Millisecond)
	f.changePorts(t, 9080)
	r.Schedule()
	eventually(t, 2*time.Second, "la primera recreación crea el contenedor", func() bool { return f.creates() == 2 })
	r.Schedule() // llega mientras el Sync de la primera espera a Caddy

	eventually(t, 2*time.Second, "el Load de la primera", func() bool { return len(f.caddy.Loads()) == loads+1 })
	if !f.state.Recreating() {
		t.Fatal("la segunda sigue pendiente")
	}
	eventually(t, 2*time.Second, "la segunda termina", func() bool { return !f.state.Recreating() })
	if got := len(f.caddy.Loads()); got != loads+2 {
		t.Fatalf("loads: %d, esperaba %d", got, loads+2)
	}
}

// D15: con un Hold (el hook de admin corriendo), GET /proxy dice recreating
// desde ya, pero el contenedor no se recrea hasta delay después del release.
func TestRecreatorWaitsForHold(t *testing.T) {
	f, r := newRecreatorFixture(t, 20*time.Millisecond)
	f.changePorts(t, 9080) // admin guarda antes de llamar al hook
	release := r.Hold()
	if status, _, _ := f.state.Snapshot(true); status != ProxyRecreating {
		t.Fatalf("con el Hold: %s", status)
	}
	time.Sleep(100 * time.Millisecond)
	if n := f.creates(); n != 1 {
		t.Fatalf("no recrea mientras el hook no vuelve: CreateContainer %d veces", n)
	}

	at := time.Now()
	release()
	eventually(t, 2*time.Second, "recrea después del release", func() bool { return f.creates() == 2 })
	if d := time.Since(at); d < 20*time.Millisecond {
		t.Fatalf("recreó %s después del release, antes del delay", d)
	}
	eventually(t, 2*time.Second, "termina", func() bool { return !f.state.Recreating() })

	loads := len(f.caddy.Loads())
	release() // idempotente
	time.Sleep(60 * time.Millisecond)
	if f.state.Recreating() || f.creates() != 2 || len(f.caddy.Loads()) != loads {
		t.Fatalf("un segundo release no hace nada: recreating %v, CreateContainer %d, loads %d → %d",
			f.state.Recreating(), f.creates(), loads, len(f.caddy.Loads()))
	}
}

// Un Hold frena también una recreación que ya estaba agendada: una sola,
// después del release.
func TestRecreatorHoldStopsPendingSchedule(t *testing.T) {
	f, r := newRecreatorFixture(t, 50*time.Millisecond)
	f.changePorts(t, 9080)
	r.Schedule()
	release := r.Hold()
	time.Sleep(150 * time.Millisecond)
	if n := f.creates(); n != 1 {
		t.Fatalf("no recrea durante el Hold: CreateContainer %d veces", n)
	}
	release()
	eventually(t, 2*time.Second, "termina", func() bool { return !f.state.Recreating() })
	time.Sleep(80 * time.Millisecond)
	if n := f.creates(); n != 2 {
		t.Fatalf("una sola recreación: CreateContainer %d veces", n)
	}
}

// Caddy recién creado tarda en abrir la admin API: mientras no responde,
// GET /proxy sigue en recreating (no pasa por out_of_sync), y termina synced.
func TestRecreatorWaitsForCaddy(t *testing.T) {
	f, r := newRecreatorFixture(t, 20*time.Millisecond)
	f.caddy.SetDown(true)
	f.changePorts(t, 9080)
	r.Schedule()
	eventually(t, 2*time.Second, "recrea", func() bool { return f.creates() == 2 })
	time.Sleep(30 * time.Millisecond)
	if status, _, _ := f.state.Snapshot(true); status != ProxyRecreating {
		t.Fatalf("mientras Caddy no responde: %s", status)
	}
	f.caddy.SetDown(false)
	eventually(t, 2*time.Second, "synced", f.state.Synced)
}
