package mcp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/CPU-commits/muelle/src/cmd/mcp/tools"
	"github.com/CPU-commits/muelle/src/internal/ingress/app/dto"
)

// Un dominio con el certificado de Cloudflare: list_domains muestra de dónde
// sale el certificado y el estado de Origin CA con sus cinco claves, con el
// error cortado a MaxErrorChars como last_error.
func TestListDomainsOriginCA(t *testing.T) {
	retry := time.Date(2026, 10, 22, 18, 0, 0, 0, time.UTC)
	exp := time.Date(2027, 1, 20, 10, 0, 0, 0, time.UTC)
	code := "permission"
	long := strings.Repeat("ñ", 400)
	ds := []dto.DomainOutput{
		{Hostname: "a.p1.cl", Status: "active", Proxied: true, TLSMode: "cloudflare", CertSource: "cloudflare_origin_ca",
			OriginCA: &dto.OriginCAOutput{Status: "active", ExpiresAt: &exp}},
		{Hostname: "b.p1.cl", Status: "active", Proxied: true, TLSMode: "cloudflare", CertSource: "letsencrypt",
			OriginCA: &dto.OriginCAOutput{Status: "failed", ErrorCode: &code, Error: &long, RetryAt: &retry}},
	}
	b, err := json.Marshal(listDomains(ds))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Domains []struct {
			CertSource string                     `json:"cert_source"`
			OriginCA   map[string]json.RawMessage `json:"origin_ca"`
		} `json:"domains"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	a, f := out.Domains[0], out.Domains[1]
	if a.CertSource != "cloudflare_origin_ca" || string(a.OriginCA["status"]) != `"active"` ||
		string(a.OriginCA["expires_at"]) != `"2027-01-20T10:00:00Z"` || string(a.OriginCA["error"]) != "null" {
		t.Errorf("activo: %s", b)
	}
	for _, k := range []string{"status", "error_code", "error", "retry_at", "expires_at"} {
		if _, ok := f.OriginCA[k]; !ok || len(f.OriginCA) != 5 {
			t.Fatalf("origin_ca sin la clave %s o con de más: %s", k, b)
		}
	}
	var e string
	if err := json.Unmarshal(f.OriginCA["error"], &e); err != nil {
		t.Fatal(err)
	}
	if f.CertSource != "letsencrypt" || string(f.OriginCA["error_code"]) != `"permission"` ||
		string(f.OriginCA["retry_at"]) != `"2026-10-22T18:00:00Z"` || utf8.RuneCountInString(e) != MaxErrorChars ||
		!strings.HasSuffix(e, tools.Ellipsis) {
		t.Errorf("falló: %s", b)
	}
	if *ds[1].OriginCA.Error != long {
		t.Error("cortar el error tocó el dominio de entrada")
	}
}
