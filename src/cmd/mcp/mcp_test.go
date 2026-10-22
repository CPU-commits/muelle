package mcp

import (
	"context"
	"fmt"
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

// Un panic en una herramienta, servido por el handler HTTP, es un internal
// en la respuesta: el SDK corre el pedido en una goroutine suya, sin el
// recover por pedido de net/http, y sin el del tools.Kit tiraría el proceso.
func TestPanicInAToolDoesNotCrash(t *testing.T) {
	k := tools.NewKit(tools.Config{Classify: func(error) (string, string, bool) { return "", "", false }, Log: slog.New(slog.DiscardHandler)})
	type in struct {
		Project string `json:"project"`
		Stage   string `json:"stage"`
		Panic   bool   `json:"panic,omitzero"`
	}
	tools.Register(k, tools.Tool{Name: "boom", Title: "Boom", Description: "Hace panic.", Kind: tools.Read},
		func(in in) *tools.Scope { return &tools.Scope{Project: in.Project, Stage: in.Stage} },
		func(_ context.Context, in in) (any, error) {
			if in.Panic {
				var m map[string]int
				m["x"]++
			}
			return map[string]string{"ok": "sí"}, nil
		})
	h := NewHandler(k)
	p := entity.Principal{Token: &entity.TokenScope{ID: 1, Name: "t", ProjectSlug: "p1"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(entity.WithPrincipal(r.Context(), p)))
	}))
	defer srv.Close()
	call := func(boom bool) string {
		t.Helper()
		body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"boom","arguments":{"project":"p1","stage":"s1","panic":%t}}}`, boom)
		req, _ := http.NewRequest("POST", srv.URL, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("MCP-Protocol-Version", "2025-11-25")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode != 200 {
			t.Fatalf("%d %s", res.StatusCode, b)
		}
		return string(b)
	}
	if got := call(true); !strings.Contains(got, `"isError":true`) || !strings.Contains(got, `\"code\":\"internal\"`) {
		t.Fatalf("con panic: %s", got)
	}
	if got := call(false); !strings.Contains(got, `{\"ok\":\"sí\"}`) || strings.Contains(got, `"isError":true`) {
		t.Fatalf("después del panic: %s", got)
	}
}
