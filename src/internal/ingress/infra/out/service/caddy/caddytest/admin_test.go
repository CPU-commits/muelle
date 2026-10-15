package caddytest

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// client habla con la admin API falsa por Dial, como lo hace caddy.Admin.
func client(a *Admin) *http.Client {
	return &http.Client{Transport: &http.Transport{
		DialContext:       func(ctx context.Context, _, _ string) (net.Conn, error) { return a.Dial(ctx) },
		DisableKeepAlives: true,
	}}
}

func post(t *testing.T, a *Admin, path, body string, force bool) (int, string) {
	t.Helper()
	req, err := http.NewRequest("POST", "http://caddy"+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if force {
		req.Header.Set("Cache-Control", "must-revalidate")
	}
	resp, err := client(a).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestAdminRecordsLoads(t *testing.T) {
	a := New(t)
	if code, _ := post(t, a, "/load", `{"admin":{"listen":"x"}}`, false); code != http.StatusOK {
		t.Fatalf("load: %d", code)
	}
	if code, _ := post(t, a, "/load", `{"apps":{}}`, true); code != http.StatusOK {
		t.Fatalf("load forzado: %d", code)
	}
	if got := a.Loads(); len(got) != 2 || string(got[0]) != `{"admin":{"listen":"x"}}` {
		t.Fatalf("Loads: %q", got)
	}
	if got := a.Forced(); len(got) != 2 || got[0] || !got[1] {
		t.Fatalf("Forced: %v", got)
	}
	if last := a.Last(t); last["apps"] == nil {
		t.Fatalf("Last: %v", last)
	}
	if code, _ := post(t, a, "/config/", `{}`, false); code != http.StatusNotFound {
		t.Fatalf("otra ruta: %d", code)
	}
	if len(a.Loads()) != 2 {
		t.Fatal("otra ruta no es un load")
	}
}

func TestAdminRejectAccept(t *testing.T) {
	a := New(t)
	a.Reject(`loading config: api_token "x"`)
	code, body := post(t, a, "/load", `{}`, false)
	if code != http.StatusBadRequest || body != `{"error":"loading config: api_token \"x\""}` {
		t.Fatalf("rechazo: %d %s", code, body)
	}
	if code, _ := post(t, a, "/load", `{}`, false); code != http.StatusBadRequest {
		t.Fatalf("sigue rechazando hasta Accept: %d", code)
	}
	a.Accept()
	if code, _ := post(t, a, "/load", `{}`, false); code != http.StatusOK {
		t.Fatalf("después de Accept: %d", code)
	}
	if len(a.Loads()) != 3 {
		t.Fatalf("Loads guarda también los rechazados: %d", len(a.Loads()))
	}
}

func TestAdminDown(t *testing.T) {
	a := New(t)
	a.SetDown(true)
	if _, err := a.Dial(context.Background()); err == nil {
		t.Fatal("Dial con la admin API caída")
	}
	a.SetDown(false)
	if code, _ := post(t, a, "/load", `{}`, false); code != http.StatusOK {
		t.Fatalf("de vuelta: %d", code)
	}
}

func TestAdminMaxInFlight(t *testing.T) {
	a := New(t)
	a.Delay(20 * time.Millisecond)
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			// Sin t.Fatal: esto corre en otra goroutine.
			resp, err := client(a).Post("http://caddy/load", "application/json", strings.NewReader(`{}`))
			if err != nil {
				t.Error(err)
				return
			}
			resp.Body.Close()
		})
	}
	wg.Wait()
	if got := a.MaxInFlight(); got < 2 {
		t.Fatalf("tres a la vez con demora: MaxInFlight = %d", got)
	}
}
