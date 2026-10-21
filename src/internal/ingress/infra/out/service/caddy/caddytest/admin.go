// Package caddytest es una admin API de Caddy falsa para los tests de
// ingress: un servidor httptest en TCP que atiende POST /load, guarda cada
// configuración que recibe y puede rechazarlas, demorarlas o no responder.
// Se conecta con Dial (Options.CaddyAdminDial, caddy.NewAdmin), como el
// socket unix de muelle-caddy.
package caddytest

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"
)

// ErrDown es el error de Dial con la admin API caída (SetDown).
var ErrDown = errors.New("caddytest: la admin API no responde")

// Admin es la admin API falsa. Todos sus métodos se pueden llamar mientras
// atiende peticiones.
type Admin struct {
	srv *httptest.Server

	mu          sync.Mutex
	loads       [][]byte
	forced      []bool
	reject      string // != "": los POST /load responden 400 con este error
	down        bool
	delay       time.Duration
	inFlight    int
	maxInFlight int
}

// New arranca la admin API; se cierra con t.Cleanup.
func New(t testing.TB) *Admin {
	t.Helper()
	a := &Admin{}
	a.srv = httptest.NewServer(http.HandlerFunc(a.serve))
	t.Cleanup(a.srv.Close)
	return a
}

// Dial conecta con la admin API (para Options.CaddyAdminDial o
// caddy.NewAdmin). Con SetDown(true) falla con ErrDown.
func (a *Admin) Dial(ctx context.Context) (net.Conn, error) {
	a.mu.Lock()
	down := a.down
	a.mu.Unlock()
	if down {
		return nil, ErrDown
	}
	return (&net.Dialer{}).DialContext(ctx, "tcp", a.srv.Listener.Addr().String())
}

// Loads son los cuerpos de los POST /load recibidos, en orden, también los
// rechazados.
func (a *Admin) Loads() [][]byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([][]byte, len(a.loads))
	for i, b := range a.loads {
		out[i] = slices.Clone(b)
	}
	return out
}

// Last es el último POST /load, decodificado. Sin ninguno, el test falla.
func (a *Admin) Last(t testing.TB) map[string]any {
	t.Helper()
	loads := a.Loads()
	if len(loads) == 0 {
		t.Fatal("caddytest: no llegó ningún POST /load")
	}
	var m map[string]any
	if err := json.Unmarshal(loads[len(loads)-1], &m); err != nil {
		t.Fatalf("caddytest: el último POST /load no es JSON: %v", err)
	}
	return m
}

// Forced dice, por cada POST /load, si traía Cache-Control: must-revalidate.
func (a *Admin) Forced() []bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.forced)
}

// Reject hace que los próximos POST /load respondan 400 {"error": msg}, como
// Caddy con una configuración que no acepta, hasta Accept.
func (a *Admin) Reject(msg string) { a.mu.Lock(); a.reject = msg; a.mu.Unlock() }

// Accept vuelve a aceptar los POST /load.
func (a *Admin) Accept() { a.mu.Lock(); a.reject = ""; a.mu.Unlock() }

// SetDown hace que Dial falle (Caddy caído, o el socket que no existe).
func (a *Admin) SetDown(down bool) { a.mu.Lock(); a.down = down; a.mu.Unlock() }

// Delay demora cada POST /load antes de responder.
func (a *Admin) Delay(d time.Duration) { a.mu.Lock(); a.delay = d; a.mu.Unlock() }

// MaxInFlight es el máximo de POST /load que atendió a la vez.
func (a *Admin) MaxInFlight() int { a.mu.Lock(); defer a.mu.Unlock(); return a.maxInFlight }

func (a *Admin) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/load" {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.mu.Lock()
	a.loads = append(a.loads, body)
	a.forced = append(a.forced, r.Header.Get("Cache-Control") == "must-revalidate")
	a.inFlight++
	a.maxInFlight = max(a.maxInFlight, a.inFlight)
	delay, reject := a.delay, a.reject
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.inFlight--; a.mu.Unlock() }()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
	}
	if reject != "" {
		b, _ := json.Marshal(map[string]string{"error": reject})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write(b)
		return
	}
	w.WriteHeader(http.StatusOK)
}
