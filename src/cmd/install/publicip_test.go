package install

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func traceServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestDetectPublicIP(t *testing.T) {
	srv := traceServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("fl=1\nh=1.1.1.1\nip=203.0.113.24\nts=1\n"))
	})
	ip, err := detectPublicIP(context.Background(), srv.Client(), srv.URL)
	if err != nil || ip != "203.0.113.24" {
		t.Fatalf("ip=%q err=%v", ip, err)
	}
}

func TestDetectPublicIPRejects(t *testing.T) {
	for name, body := range map[string]string{
		"ipv6":        "fl=1\nip=2001:db8::1\nts=1\n",
		"sin ip":      "fl=1\nts=1\n",
		"no canónica": "ip=203.000.113.24\n",
		"basura":      "ip=<script>\n",
		// La línea ip= después de 4 KiB no se lee.
		"muy largo": strings.Repeat("x="+strings.Repeat("y", 100)+"\n", 50) + "ip=203.0.113.24\n",
	} {
		t.Run(name, func(t *testing.T) {
			srv := traceServer(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) })
			if ip, err := detectPublicIP(context.Background(), srv.Client(), srv.URL); err == nil {
				t.Fatalf("ip=%q, esperaba error", ip)
			}
		})
	}

	t.Run("500", func(t *testing.T) {
		srv := traceServer(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "ip=203.0.113.24", http.StatusInternalServerError)
		})
		if ip, err := detectPublicIP(context.Background(), srv.Client(), srv.URL); err == nil {
			t.Fatalf("ip=%q, esperaba error", ip)
		}
	})

	t.Run("no responde a tiempo", func(t *testing.T) {
		release := make(chan struct{})
		srv := traceServer(t, func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-release:
			case <-r.Context().Done():
			}
		})
		defer close(release)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		start := time.Now()
		if ip, err := detectPublicIP(ctx, srv.Client(), srv.URL); err == nil {
			t.Fatalf("ip=%q, esperaba error", ip)
		}
		if d := time.Since(start); d > 2*time.Second {
			t.Fatalf("tardó %s: no respetó el plazo del ctx", d)
		}
	})
}
