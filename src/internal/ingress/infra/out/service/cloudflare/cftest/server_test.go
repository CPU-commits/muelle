package cftest

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
)

type reply struct {
	status int
	body   string
	env    struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
		Result     jsontext.Value `json:"result"`
		ResultInfo struct {
			Page       int `json:"page"`
			TotalPages int `json:"total_pages"`
		} `json:"result_info"`
	}
}

func call(t *testing.T, s *Server, token, method, path, body string) reply {
	t.Helper()
	return callCtx(t, context.Background(), s, token, method, path, body)
}

func callCtx(t *testing.T, ctx context.Context, s *Server, token, method, path, body string) reply {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, method, s.URL()+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return reply{status: -1, body: err.Error()}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	r := reply{status: resp.StatusCode, body: string(b)}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		if err := json.Unmarshal(b, &r.env); err != nil {
			t.Fatalf("%s %s: JSON ilegible %q: %v", method, path, b, err)
		}
	}
	return r
}

func TestTokens(t *testing.T) {
	s := New(t)
	s.AddToken("activo")
	s.AddAccountToken("cuenta")
	s.SetTokenStatus("viejo", "expired")
	s.AddZone("z1", "x.cl")

	r := call(t, s, "activo", "GET", "/user/tokens/verify", "")
	if r.status != 200 || !r.env.Success || !strings.Contains(string(r.env.Result), `"status":"active"`) {
		t.Fatalf("verify activo: %d %s", r.status, r.body)
	}
	for _, tok := range []string{"", "otro", "cuenta"} {
		r := call(t, s, tok, "GET", "/user/tokens/verify", "")
		if r.status != 401 || r.env.Success || len(r.env.Errors) != 1 || r.env.Errors[0].Code != 1000 || r.env.Errors[0].Message != "Invalid API Token" {
			t.Errorf("verify %q: %d %s", tok, r.status, r.body)
		}
	}
	r = call(t, s, "viejo", "GET", "/user/tokens/verify", "")
	if r.status != 200 || !strings.Contains(string(r.env.Result), `"status":"expired"`) {
		t.Fatalf("verify vencido: %d %s", r.status, r.body)
	}
	// El token de una cuenta lista zonas; uno vencido o desconocido, no.
	if r := call(t, s, "cuenta", "GET", "/zones", ""); r.status != 200 {
		t.Fatalf("zonas con token de cuenta: %d %s", r.status, r.body)
	}
	for _, tok := range []string{"viejo", "otro"} {
		if r := call(t, s, tok, "GET", "/zones", ""); r.status != 401 {
			t.Errorf("zonas con %q: %d %s", tok, r.status, r.body)
		}
	}
}

func TestZonesPages(t *testing.T) {
	s := New(t)
	s.AddToken("t")
	s.AddZone("z2", "b.cl")
	s.AddZone("z1", "a.cl")
	s.AddZone("z3", "c.cl")
	r := call(t, s, "t", "GET", "/zones?page=2&per_page=2", "")
	if r.status != 200 || r.env.ResultInfo.Page != 2 || r.env.ResultInfo.TotalPages != 2 {
		t.Fatalf("página 2: %d %s", r.status, r.body)
	}
	var zones []service.Zone
	if err := json.Unmarshal(r.env.Result, &zones, json.MatchCaseInsensitiveNames(true)); err != nil {
		t.Fatal(err)
	}
	if len(zones) != 1 || zones[0].ID != "z3" || zones[0].Name != "c.cl" || zones[0].Status != "active" {
		t.Fatalf("página 2: %+v", zones)
	}
	// per_page tiene un máximo de 50, como en Cloudflare.
	if r := call(t, s, "t", "GET", "/zones?per_page=500", ""); !strings.Contains(r.body, `"per_page":50`) {
		t.Fatalf("per_page=500: %s", r.body)
	}
}

func TestRecordsCRUD(t *testing.T) {
	s := New(t)
	s.AddToken("t")
	s.AddZone("z1", "x.cl")
	ajeno := s.PutRecord("z1", service.Record{Type: "TXT", Name: "a.x.cl", Content: "v=spf1"})
	s.PutRecord("z1", service.Record{ID: "fijo", Type: "A", Name: "b.x.cl", Content: "203.0.113.9"})

	r := call(t, s, "t", "POST", "/zones/z1/dns_records",
		`{"type":"A","name":"a.x.cl","content":"203.0.113.24","ttl":1,"proxied":true,"comment":"muelle"}`)
	if r.status != 200 || !r.env.Success {
		t.Fatalf("POST: %d %s", r.status, r.body)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(r.env.Result, &created); err != nil || created.ID == "" {
		t.Fatalf("POST sin id: %s %v", r.env.Result, err)
	}
	if c := s.Comment("z1", created.ID); c != "muelle" {
		t.Fatalf("comentario: %q", c)
	}
	if c := s.Comment("z1", ajeno); c != "" {
		t.Fatalf("PutRecord no pone comentario: %q", c)
	}

	// Filtra por nombre exacto (sin importar mayúsculas) y por tipo.
	r = call(t, s, "t", "GET", "/zones/z1/dns_records?name.exact=A.X.CL", "")
	if r.status != 200 || strings.Count(string(r.env.Result), `"id"`) != 2 || strings.Contains(r.body, "b.x.cl") {
		t.Fatalf("por nombre: %s", r.body)
	}
	r = call(t, s, "t", "GET", "/zones/z1/dns_records?name.exact=a.x.cl&type=TXT", "")
	if strings.Count(string(r.env.Result), `"id"`) != 1 || !strings.Contains(r.body, "v=spf1") {
		t.Fatalf("por nombre y tipo: %s", r.body)
	}

	// PATCH cambia solo lo que viene, también el tipo.
	if r := call(t, s, "t", "PATCH", "/zones/z1/dns_records/fijo", `{"content":"203.0.113.10"}`); r.status != 200 {
		t.Fatalf("PATCH: %d %s", r.status, r.body)
	}
	if r := call(t, s, "t", "PATCH", "/zones/z1/dns_records/fijo", `{"type":"AAAA","content":"2001:db8::10"}`); r.status != 200 {
		t.Fatalf("PATCH tipo: %d %s", r.status, r.body)
	}
	want := []service.Record{
		{ID: created.ID, Type: "A", Name: "a.x.cl", Content: "203.0.113.24", Proxied: true},
		{ID: ajeno, Type: "TXT", Name: "a.x.cl", Content: "v=spf1"},
		{ID: "fijo", Type: "AAAA", Name: "b.x.cl", Content: "2001:db8::10"},
	}
	if got := s.Records("z1"); !reflect.DeepEqual(got, want) {
		t.Fatalf("registros:\n%+v\nesperaba\n%+v", got, want)
	}
	if r := call(t, s, "t", "GET", "/zones/z1/dns_records/fijo", ""); r.status != 200 || !strings.Contains(r.body, "2001:db8::10") {
		t.Fatalf("GET: %d %s", r.status, r.body)
	}
	if r := call(t, s, "t", "DELETE", "/zones/z1/dns_records/fijo", ""); r.status != 200 {
		t.Fatalf("DELETE: %d %s", r.status, r.body)
	}
	for _, m := range []string{"GET", "PATCH", "DELETE"} {
		if r := call(t, s, "t", m, "/zones/z1/dns_records/fijo", `{}`); r.status != 404 || r.env.Success || len(r.env.Errors) == 0 {
			t.Errorf("%s de un registro borrado: %d %s", m, r.status, r.body)
		}
	}
	if r := call(t, s, "t", "GET", "/zones/nada/dns_records", ""); r.status != 404 || len(r.env.Errors) == 0 {
		t.Fatalf("zona que no existe: %d %s", r.status, r.body)
	}
}

// Un A no convive con un CNAME del mismo nombre, ni dos registros idénticos.
func TestRecordConflicts(t *testing.T) {
	s := New(t)
	s.AddToken("t")
	s.AddZone("z1", "x.cl")
	s.PutRecord("z1", service.Record{Type: "CNAME", Name: "c.x.cl", Content: "otro.cl"})
	s.PutRecord("z1", service.Record{Type: "A", Name: "a.x.cl", Content: "203.0.113.1"})
	for body, code := range map[string]int{
		`{"type":"A","name":"c.x.cl","content":"203.0.113.24"}`: 81053,
		`{"type":"CNAME","name":"a.x.cl","content":"otro.cl"}`:  81053,
		`{"type":"A","name":"a.x.cl","content":"203.0.113.1"}`:  81058,
	} {
		r := call(t, s, "t", "POST", "/zones/z1/dns_records", body)
		if r.status != 400 || len(r.env.Errors) != 1 || r.env.Errors[0].Code != code {
			t.Errorf("%s: %d %s", body, r.status, r.body)
		}
	}
	// Otro A del mismo nombre sí (Cloudflare acepta varios A).
	if r := call(t, s, "t", "POST", "/zones/z1/dns_records", `{"type":"A","name":"a.x.cl","content":"203.0.113.2"}`); r.status != 200 {
		t.Fatalf("segundo A: %d %s", r.status, r.body)
	}
}

func TestFailuresAndCalls(t *testing.T) {
	s := New(t)
	s.AddToken("t")
	s.AddZone("z1", "x.cl")

	s.FailNext("GET", "/zones?", 429, 10000, "Rate limited")
	if r := call(t, s, "t", "GET", "/zones/z1/dns_records", ""); r.status != 200 {
		t.Fatalf("otra ruta no falla: %d", r.status)
	}
	if r := call(t, s, "t", "GET", "/zones?page=1", ""); r.status != 429 || r.env.Errors[0].Message != "Rate limited" {
		t.Fatalf("FailNext: %d %s", r.status, r.body)
	}
	if r := call(t, s, "t", "GET", "/zones?page=1", ""); r.status != 200 {
		t.Fatalf("FailNext es una sola vez: %d", r.status)
	}
	s.FailNext("POST", "/dns_records", 200, 9999, "algo")
	if r := call(t, s, "t", "POST", "/zones/z1/dns_records", `{}`); r.status != 200 || r.env.Success || r.env.Errors[0].Code != 9999 {
		t.Fatalf("FailNext con 200: %d %s", r.status, r.body)
	}

	s.SetDown(true)
	if r := call(t, s, "t", "GET", "/user/tokens/verify", ""); r.status != 503 || strings.HasPrefix(strings.TrimSpace(r.body), "{") {
		t.Fatalf("SetDown: %d %s", r.status, r.body)
	}
	s.SetDown(false)

	s.Hang(time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	r := callCtx(t, ctx, s, "t", "GET", "/zones", "")
	if r.status != -1 || !errors.Is(ctx.Err(), context.DeadlineExceeded) || time.Since(start) > 900*time.Millisecond {
		t.Fatalf("Hang: %d %s en %v", r.status, r.body, time.Since(start))
	}
	s.Hang(0)

	want := []string{
		"GET /zones/z1/dns_records",
		"GET /zones?page=1",
		"GET /zones?page=1",
		"POST /zones/z1/dns_records",
		"GET /user/tokens/verify",
		"GET /zones",
	}
	if got := s.Calls(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Calls:\n%q\nesperaba\n%q", got, want)
	}
}
