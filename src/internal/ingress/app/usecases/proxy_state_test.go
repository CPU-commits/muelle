package usecases

import (
	"testing"
	"time"
)

func TestProxyStateInitial(t *testing.T) {
	s := NewProxyState()
	status, msg, at := s.Snapshot(true)
	if status != ProxyOutOfSync || msg != NotSyncedYet || at != nil {
		t.Fatalf("al arrancar: %s %q %v", status, msg, at)
	}
	if s.Synced() || s.Recreating() {
		t.Fatal("al arrancar no está sincronizado ni recreando")
	}
}

func TestProxyStatePrecedence(t *testing.T) {
	s := NewProxyState()
	at := time.Date(2026, 10, 10, 14, 20, 41, 0, time.UTC)
	s.LoadFailed("caddy dijo que no")
	s.ContainerFailed("los puertos 80 o 443 del host ya están en uso")

	// disabled gana todo, sin error.
	if status, msg, _ := s.Snapshot(false); status != ProxyDisabled || msg != "" {
		t.Fatalf("disabled: %s %q", status, msg)
	}
	// El error del contenedor gana sobre el de /load.
	if status, msg, _ := s.Snapshot(true); status != ProxyOutOfSync || msg != "los puertos 80 o 443 del host ya están en uso" {
		t.Fatalf("contenedor: %s %q", status, msg)
	}
	// Una recreación en curso gana sobre los errores, y no muestra ninguno.
	g := s.BeginRecreate()
	if status, msg, _ := s.Snapshot(true); status != ProxyRecreating || msg != "" {
		t.Fatalf("recreando: %s %q", status, msg)
	}
	if s.Synced() {
		t.Fatal("recreando no está sincronizado")
	}
	s.EndRecreate(g)
	s.ContainerOK()
	if status, msg, _ := s.Snapshot(true); status != ProxyOutOfSync || msg != "caddy dijo que no" {
		t.Fatalf("/load: %s %q", status, msg)
	}
	s.Loaded(at)
	status, msg, got := s.Snapshot(true)
	if status != ProxySynced || msg != "" || got == nil || !got.Equal(at) || !s.Synced() {
		t.Fatalf("sincronizado: %s %q %v", status, msg, got)
	}
	// syncedAt es una copia: cambiarla no cambia el estado.
	*got = got.Add(time.Hour)
	if _, _, again := s.Snapshot(true); !again.Equal(at) {
		t.Fatalf("Snapshot devolvió el puntero interno: %v", again)
	}
	// Un error después de un Load conserva syncedAt (el último exitoso).
	s.LoadFailed("otra vez no")
	if status, _, again := s.Snapshot(true); status != ProxyOutOfSync || again == nil || !again.Equal(at) {
		t.Fatalf("después de fallar: %s %v", status, again)
	}
}

// Un cambio que llega durante una recreación no se pierde: hasta que termina
// la última, sigue recreating (D15).
func TestProxyStateRecreateGenerations(t *testing.T) {
	s := NewProxyState()
	s.Loaded(time.Now())
	g1 := s.BeginRecreate()
	g2 := s.BeginRecreate()
	s.EndRecreate(g1)
	if !s.Recreating() {
		t.Fatal("terminó g1, pero g2 sigue pendiente")
	}
	s.EndRecreate(g2)
	if s.Recreating() || !s.Synced() {
		t.Fatal("terminó la última")
	}
	// Terminar una vieja después de una nueva no la reabre.
	s.EndRecreate(g1)
	if s.Recreating() {
		t.Fatal("EndRecreate de una generación vieja")
	}
}
