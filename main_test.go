package main

import (
	"bytes"
	"encoding/base64"
	jsonv2 "encoding/json/v2"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
)

func TestNewHTTPServer(t *testing.T) {
	h := http.NotFoundHandler()
	srv := newHTTPServer(h, slog.New(slog.DiscardHandler))

	if srv.ReadHeaderTimeout != 10*time.Second {
		t.Errorf("ReadHeaderTimeout=%s", srv.ReadHeaderTimeout)
	}
	// Caddy no limita la lectura y reenvía el cuerpo a medida que llega: sin
	// ReadTimeout, un cliente lento ocupa la conexión para siempre.
	if srv.ReadTimeout != 30*time.Second {
		t.Errorf("ReadTimeout=%s", srv.ReadTimeout)
	}
	if srv.IdleTimeout != 3*time.Minute {
		t.Errorf("IdleTimeout=%s", srv.IdleTimeout)
	}
	// Sin WriteTimeout global: los streams SSE (Fase 2) ponen su propio plazo.
	if srv.WriteTimeout != 0 {
		t.Errorf("WriteTimeout=%s, esperaba 0", srv.WriteTimeout)
	}
	if srv.MaxHeaderBytes != 64<<10 {
		t.Errorf("MaxHeaderBytes=%d", srv.MaxHeaderBytes)
	}
	if srv.ErrorLog == nil {
		t.Error("falta ErrorLog")
	}
	if srv.Handler == nil {
		t.Error("falta Handler")
	}
}

func TestRunUsage(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
	}{
		{nil, 2},
		{[]string{"otro"}, 2},
		{[]string{"serve", "extra"}, 2},
		{[]string{"genkey", "extra"}, 2},
		{[]string{"genkey", "-h"}, 2},
		{[]string{"-h"}, 0},
		{[]string{"--help"}, 0},
		{[]string{"help"}, 0},
	} {
		var stdout, stderr bytes.Buffer
		code := run(tc.args, &stdout, &stderr)
		if code != tc.code {
			t.Errorf("%q: código %d, esperaba %d", tc.args, code, tc.code)
		}
		// La ayuda pedida va a stdout; la de un error, a stderr.
		out, other := &stderr, &stdout
		if tc.code == 0 {
			out, other = &stdout, &stderr
		}
		if !strings.Contains(out.String(), usage) {
			t.Errorf("%q: falta el uso en la salida: %q", tc.args, out.String())
		}
		if other.Len() != 0 {
			t.Errorf("%q: salida inesperada: %q", tc.args, other.String())
		}
	}
}

func TestRunGenkey(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"genkey"}, &stdout, &stderr); code != 0 {
		t.Fatalf("código %d: %s", code, stderr.String())
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(stdout.String()))
	if err != nil || len(key) != crypto.KeySize {
		t.Fatalf("clave=%q err=%v", stdout.String(), err)
	}
}

// serveEnv configura un muelle de prueba en un directorio temporal y
// devuelve su DataDir. key es el contenido del archivo de la clave maestra
// ("" para que el archivo no exista).
func serveEnv(t *testing.T, listen, key string) string {
	t.Helper()
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key")
	if key != "" {
		if err := os.WriteFile(keyFile, []byte(key), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	dataDir := filepath.Join(dir, "data")
	t.Setenv("MUELLE_LISTEN", listen)
	t.Setenv("MUELLE_DATA_DIR", dataDir)
	t.Setenv("MUELLE_MASTER_KEY_FILE", keyFile)
	t.Setenv("MUELLE_PANEL_ORIGIN", "http://127.0.0.1:8080")
	t.Setenv("MUELLE_SECURE_COOKIES", "false")
	t.Setenv("GOMEMLIMIT", "")
	// Nunca el Docker real de la máquina: un socket que no existe.
	t.Setenv("DOCKER_HOST", "unix://"+filepath.Join(t.TempDir(), "sin-docker.sock"))

	// serve cambia el logger por defecto y el límite de memoria del proceso.
	prevLog := slog.Default()
	prevLimit := debug.SetMemoryLimit(-1)
	t.Cleanup(func() {
		slog.SetDefault(prevLog)
		debug.SetMemoryLimit(prevLimit)
	})
	return dataDir
}

// lastLogLine decodifica la última línea JSON de out.
func lastLogLine(t *testing.T, out string) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var m map[string]any
	if err := jsonv2.Unmarshal([]byte(lines[len(lines)-1]), &m); err != nil {
		t.Fatalf("la última línea no es JSON: %q (%v)", out, err)
	}
	return m
}

// Sin la clave maestra, serve termina con un error en JSON (el mismo formato
// que el resto del log) que dice qué variable revisar y cómo crear una clave.
func TestRunServeMissingKey(t *testing.T) {
	dataDir := serveEnv(t, "127.0.0.1:0", "")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"serve"}, &stdout, &stderr); code != 1 {
		t.Fatalf("código %d, esperaba 1", code)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr=%q: el error va en el log JSON", stderr.String())
	}
	m := lastLogLine(t, stdout.String())
	errMsg, _ := m["err"].(string)
	if m["level"] != "ERROR" || !strings.Contains(errMsg, "MUELLE_MASTER_KEY_FILE") || !strings.Contains(errMsg, "muelle genkey") {
		t.Fatalf("log=%v", m)
	}
	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Fatalf("sin la clave no se debió crear DataDir: %v", err)
	}
}

// Con el puerto ocupado (por ejemplo, otra instancia), serve falla antes de
// leer la clave o tocar DataDir: no crea, no migra ni abre la base.
func TestRunServeBusyPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	dataDir := serveEnv(t, ln.Addr().String(), crypto.GenerateKey()+"\n")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"serve"}, &stdout, &stderr); code != 1 {
		t.Fatalf("código %d, esperaba 1", code)
	}
	if m := lastLogLine(t, stdout.String()); !strings.Contains(m["err"].(string), "address already in use") {
		t.Fatalf("log=%v", m)
	}
	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Fatalf("con el puerto ocupado no se debió crear DataDir: %v", err)
	}
}

// Sin GOMEMLIMIT, serve pone un límite blando de 64 MiB; con GOMEMLIMIT
// definido respeta el del entorno, que el runtime ya aplicó al arrancar.
func TestRunServeMemoryLimit(t *testing.T) {
	const before = 1 << 40 // un valor que serve nunca pone
	for _, tc := range []struct {
		env  string
		want int64
	}{
		{"", 64 << 20},
		{"1GiB", before},
	} {
		t.Run("GOMEMLIMIT="+tc.env, func(t *testing.T) {
			serveEnv(t, "127.0.0.1:0", "")
			t.Setenv("GOMEMLIMIT", tc.env)
			debug.SetMemoryLimit(before)
			var stdout, stderr bytes.Buffer
			run([]string{"serve"}, &stdout, &stderr) // falla por la clave, después de poner el límite
			if got := debug.SetMemoryLimit(-1); got != tc.want {
				t.Errorf("límite %d, esperaba %d", got, tc.want)
			}
		})
	}
}

// La línea del link de setup lleva event=setup_url y la url completa: install
// (Fase 5) la busca en los logs del servicio para imprimirla (§3.3 paso 8).
func TestLogSetupURL(t *testing.T) {
	var out bytes.Buffer
	logSetupURL(slog.New(slog.NewJSONHandler(&out, nil)), "https://panel.example.com", "abc123")
	m := lastLogLine(t, out.String())
	if m["level"] != "INFO" || m["event"] != "setup_url" || m["url"] != "https://panel.example.com/setup?token=abc123" {
		t.Fatalf("log=%v", m)
	}
}
