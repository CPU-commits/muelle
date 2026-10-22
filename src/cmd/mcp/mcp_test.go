package mcp

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/CPU-commits/muelle/src/cmd/mcp/tools"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// Claude Code corta las instrucciones del servidor a los 4 096 caracteres.
func TestInstructionsFit(t *testing.T) {
	if n := utf8.RuneCountInString(Instructions); n >= tools.MaxDescription || n < 500 {
		t.Fatalf("las instrucciones tienen %d caracteres", n)
	}
}

// Los plazos que pone el handler no quedan en la conexión: un plazo viejo
// cortaría la respuesta siguiente de la misma conexión keep-alive (Caddy las
// reusa). net/http los quita al terminar cada pedido; el test lo fija.
func TestDeadlinesDoNotOutliveTheRequest(t *testing.T) {
	k := tools.NewKit(tools.Config{Classify: func(error) (string, string, bool) { return "", "", false }, Log: slog.New(slog.DiscardHandler)})
	h := NewHandler(k)
	h.deadline = 100 * time.Millisecond
	p := entity.Principal{Token: &entity.TokenScope{ID: 1, Name: "t", ProjectSlug: "p1"}}
	mux := http.NewServeMux()
	mux.Handle("POST /mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(entity.WithPrincipal(r.Context(), p)))
	}))
	mux.HandleFunc("GET /otro", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("x", 1<<20)))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client := &http.Client{Transport: &http.Transport{MaxConnsPerHost: 1}}
	defer client.CloseIdleConnections()

	req, _ := http.NewRequest("POST", srv.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-11-25")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("tools/list: %d %v", res.StatusCode, res.Header)
	}
	time.Sleep(300 * time.Millisecond)
	var reused bool
	req2, _ := http.NewRequest("GET", srv.URL+"/otro", nil)
	req2 = req2.WithContext(httptrace.WithClientTrace(req2.Context(), &httptrace.ClientTrace{
		GotConn: func(i httptrace.GotConnInfo) { reused = i.Reused },
	}))
	res, err = client.Do(req2)
	if err != nil {
		t.Fatalf("el pedido siguiente en la misma conexión: %v", err)
	}
	n, err := io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if err != nil || n != 1<<20 || !reused {
		t.Fatalf("el pedido siguiente leyó %d bytes (misma conexión: %v): %v", n, reused, err)
	}
}
