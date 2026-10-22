package caddy

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
)

var update = flag.Bool("update", false, "reescribe testdata/*.golden.json con la salida de Render")

const testToken = "cf-token-1234567890abcdef"

func testPanel(tls entity.TLSMode, cidrs ...string) *service.PanelRoute {
	if cidrs == nil {
		cidrs = []string{}
	}
	return &service.PanelRoute{Hostname: "panel.auditfy.cl", Upstream: "muelle_muelle:8080", AllowedCIDRs: cidrs, TLS: tls}
}

// threeRoutes son dos dominios cloudflare y uno http, por hostname.
func threeRoutes() []service.Route {
	return []service.Route{
		{Hostname: "api.auditfy.cl", Upstream: "auditfy_staging_api_api:3000", TLS: entity.TLSCloudflare},
		{Hostname: "grafana.ejemplo.com", Upstream: "auditfy_staging_infra_grafana:3000", TLS: entity.TLSHTTP},
		{Hostname: "web.auditfy.cl", Upstream: "auditfy_production_web_web:8080", TLS: entity.TLSCloudflare},
	}
}

// originPEM es un certificado de Origin CA de prueba para host: Render no
// lo lee, así que basta con la forma de un PEM.
func originPEM(host string) *service.OriginPEM {
	label := strings.ReplaceAll(host, ".", "")
	return &service.OriginPEM{
		CertPEM: "-----BEGIN CERTIFICATE-----\nMIIBcert" + label + "\n-----END CERTIFICATE-----\n",
		KeyPEM:  "-----BEGIN PRIVATE KEY-----\nMIGHkey" + label + "SecretoDeLaClave\n-----END PRIVATE KEY-----\n",
	}
}

// withOrigin es routes con un certificado de Origin CA en los hosts.
func withOrigin(routes []service.Route, hosts ...string) []service.Route {
	for i := range routes {
		if slices.Contains(hosts, routes[i].Hostname) {
			routes[i].OriginCA = originPEM(routes[i].Hostname)
		}
	}
	return routes
}

// originPanel es el panel con su certificado de Origin CA.
func originPanel(tls entity.TLSMode, cidrs ...string) *service.PanelRoute {
	p := testPanel(tls, cidrs...)
	p.OriginCA = originPEM(p.Hostname)
	return p
}

// goldenCases son las configuraciones de testdata/<caso>.golden.json.
func goldenCases() map[string]service.ProxyConfig {
	return map[string]service.ProxyConfig{
		// Sin token, sin IPs del panel y con email: el panel es un subroute
		// con solo el reverse_proxy, y hay una política http con email.
		"panel_only": {Panel: testPanel(entity.TLSHTTP), Routes: []service.Route{}, ACMEEmail: "ops@auditfy.cl"},
		// Con token y la zona del panel, IPs del panel y tres dominios.
		"full": {
			Panel:  testPanel(entity.TLSCloudflare, "190.5.6.0/24", "2001:db8::/32"),
			Routes: threeRoutes(), ACMEEmail: "ops@auditfy.cl", CloudflareToken: testToken,
		},
		// Sin token, los dominios cloudflare van todos a la política http.
		"no_token": {Panel: testPanel(entity.TLSCloudflare), Routes: threeRoutes(), ACMEEmail: "ops@auditfy.cl"},
		// Sin email, el emisor no lo lleva.
		"no_email": {Panel: testPanel(entity.TLSCloudflare), Routes: threeRoutes(), CloudflareToken: testToken},
		// La CA interna de Caddy (integración): sin ACME ni desafío DNS.
		"internal": {
			Panel:  testPanel(entity.TLSHTTP, "203.0.113.0/24"),
			Routes: threeRoutes(), ACMEEmail: "ops@auditfy.cl", CloudflareToken: testToken, InternalCA: true,
		},
		// Origin CA (§7.2 del diseño de Origin CA): el panel y api con su
		// certificado, web en DNS only (DNS-01) y grafana fuera de
		// Cloudflare (HTTP-01).
		"origin_ca": {
			Panel:  originPanel(entity.TLSCloudflare, "190.5.6.0/24", "2001:db8::/32"),
			Routes: withOrigin(threeRoutes(), "api.auditfy.cl"), ACMEEmail: "ops@auditfy.cl", CloudflareToken: testToken,
		},
		// Todos con Origin CA: no va automation.
		"origin_ca_all": {
			Panel: originPanel(entity.TLSCloudflare),
			Routes: withOrigin([]service.Route{
				{Hostname: "api.auditfy.cl", Upstream: "auditfy_staging_api_api:3000", TLS: entity.TLSCloudflare},
				{Hostname: "web.auditfy.cl", Upstream: "auditfy_production_web_web:8080", TLS: entity.TLSCloudflare},
			}, "api.auditfy.cl", "web.auditfy.cl"),
			ACMEEmail: "ops@auditfy.cl", CloudflareToken: testToken,
		},
		// Lo que manda la integración: Origin CA para un host y la CA
		// interna para los demás.
		"origin_ca_internal": {
			Panel:  testPanel(entity.TLSHTTP),
			Routes: withOrigin(threeRoutes(), "api.auditfy.cl"), ACMEEmail: "ops@auditfy.cl", CloudflareToken: testToken, InternalCA: true,
		},
	}
}

func render(t *testing.T, cfg service.ProxyConfig) []byte {
	t.Helper()
	b, err := Render(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRenderGolden(t *testing.T) {
	for name, cfg := range goldenCases() {
		t.Run(name, func(t *testing.T) {
			var got bytes.Buffer
			if err := json.Indent(&got, render(t, cfg), "", "  "); err != nil {
				t.Fatalf("Render no dio JSON: %v", err)
			}
			got.WriteByte('\n')
			path := filepath.Join("testdata", name+".golden.json")
			if *update {
				if err := os.WriteFile(path, got.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (genéralo con go test ./… -run Golden -update)", err)
			}
			if !bytes.Equal(got.Bytes(), want) {
				t.Errorf("%s cambió; si es a propósito, -update. Salida:\n%s", path, got.String())
			}
		})
	}
}

// decode es la salida de Render como mapa, para mirar sus partes.
func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// path baja por claves de mapas y posiciones de arreglos.
func path(t *testing.T, v any, keys ...any) any {
	t.Helper()
	for _, k := range keys {
		switch k := k.(type) {
		case string:
			m, ok := v.(map[string]any)
			if !ok {
				t.Fatalf("%v: no es un objeto en %q", keys, k)
			}
			v = m[k]
		case int:
			a, ok := v.([]any)
			if !ok || k >= len(a) {
				t.Fatalf("%v: no es un arreglo con %d elementos", keys, k+1)
			}
			v = a[k]
		}
	}
	return v
}

var placeholderRe = regexp.MustCompile(`\{[^}]*\}`)

// jsonStrings junta todos los textos del JSON decodificado, claves y valores: los
// placeholders de Caddy van dentro de un texto, nunca en la sintaxis del JSON.
func jsonStrings(v any, out *[]string) {
	switch v := v.(type) {
	case string:
		*out = append(*out, v)
	case []any:
		for _, e := range v {
			jsonStrings(e, out)
		}
	case map[string]any:
		for k, e := range v {
			*out = append(*out, k)
			jsonStrings(e, out)
		}
	}
}

func TestRenderInvariants(t *testing.T) {
	for name, cfg := range goldenCases() {
		t.Run(name, func(t *testing.T) {
			b := render(t, cfg)
			m := decode(t, b)
			if got := path(t, m, "admin", "listen"); got != naming.CaddyAdminListen {
				t.Errorf("admin.listen = %v", got)
			}
			// Ningún atajo del Caddyfile: el único placeholder es el nombre completo.
			var texts []string
			jsonStrings(m, &texts)
			found := 0
			for _, s := range texts {
				for _, p := range placeholderRe.FindAllString(s, -1) {
					found++
					if p != "{http.vars.client_ip}" {
						t.Errorf("placeholder %q en %q", p, s)
					}
				}
			}
			if found != 1 {
				t.Errorf("%d placeholders, esperaba solo el de X-Real-IP", found)
			}
			srv := path(t, m, "apps", "http", "servers", "muelle")
			if got := path(t, srv, "listen"); !reflect.DeepEqual(got, []any{":443"}) {
				t.Errorf("listen = %v", got)
			}
			var ranges []string
			for _, r := range path(t, srv, "trusted_proxies", "ranges").([]any) {
				ranges = append(ranges, r.(string))
			}
			if path(t, srv, "trusted_proxies", "source") != "static" || !reflect.DeepEqual(ranges, CloudflareRanges) {
				t.Errorf("trusted_proxies = %v", path(t, srv, "trusted_proxies"))
			}
			if got := path(t, srv, "trusted_proxies_strict"); got != float64(1) {
				t.Errorf("trusted_proxies_strict = %v", got)
			}
			if got := path(t, srv, "client_ip_headers"); !reflect.DeepEqual(got, []any{"CF-Connecting-IP", "X-Forwarded-For"}) {
				t.Errorf("client_ip_headers = %v", got)
			}
			routes := path(t, srv, "routes").([]any)
			if len(routes) != 1+len(cfg.Routes) {
				t.Errorf("%d rutas, esperaba el panel y %d", len(routes), len(cfg.Routes))
			}
			for i, r := range routes {
				if path(t, r, "terminal") != true {
					t.Errorf("la ruta %d no es terminal", i)
				}
			}
			// El panel va primero, como subroute, con X-Real-IP.
			if path(t, routes[0], "match", 0, "host", 0) != "panel.auditfy.cl" || path(t, routes[0], "handle", 0, "handler") != "subroute" {
				t.Errorf("la primera ruta no es la del panel: %v", routes[0])
			}
			var policies []any
			tlsApp, _ := path(t, m, "apps").(map[string]any)["tls"].(map[string]any)
			if auto, ok := tlsApp["automation"].(map[string]any); ok {
				policies = auto["policies"].([]any)
			}
			// Un host de Origin CA (skip_certificates) tiene su load_pem y no
			// está en ninguna política.
			skipped := map[string]bool{}
			if auto, ok := srv.(map[string]any)["automatic_https"]; ok {
				for _, h := range path(t, auto, "skip_certificates").([]any) {
					skipped[h.(string)] = true
				}
			}
			loaded := 0
			if certs, ok := tlsApp["certificates"]; ok {
				for _, p := range path(t, certs, "load_pem").([]any) {
					loaded++
					if host := strings.TrimPrefix(path(t, p, "tags", 1).(string), "muelle_origin_ca:"); !skipped[host] {
						t.Errorf("load_pem de %s sin skip_certificates", host)
					}
				}
			}
			if loaded != len(skipped) || (loaded > 0) != (tlsApp["disable_ocsp_stapling"] == true) {
				t.Errorf("%d load_pem, %d en skip_certificates, disable_ocsp_stapling %v", loaded, len(skipped), tlsApp["disable_ocsp_stapling"])
			}
			cloudflare := 0
			for _, p := range policies {
				if len(path(t, p, "subjects").([]any)) == 0 {
					t.Errorf("política sin subjects: %v", p)
				}
				for _, h := range path(t, p, "subjects").([]any) {
					if skipped[h.(string)] {
						t.Errorf("%s tiene Origin CA y está en una política", h)
					}
				}
				if path(t, p, "issuers", 0, "challenges") != nil {
					cloudflare++
					if path(t, p, "issuers", 0, "challenges", "dns", "provider", "api_token") != testToken {
						t.Errorf("el token no está en api_token: %v", p)
					}
				}
			}
			if n := strings.Count(string(b), testToken); n != cloudflare {
				t.Errorf("el token aparece %d veces, con %d políticas cloudflare", n, cloudflare)
			}
		})
	}
}

func TestRenderPanelRoute(t *testing.T) {
	cfg := goldenCases()["full"]
	panel := path(t, decode(t, render(t, cfg)), "apps", "http", "servers", "muelle", "routes", 0, "handle", 0, "routes")
	// Con IPs del panel: primero el 403 para las que no están (client_ip, no remote_ip).
	if got := path(t, panel, 0, "match", 0, "not", 0, "client_ip", "ranges"); !reflect.DeepEqual(got, []any{"190.5.6.0/24", "2001:db8::/32"}) {
		t.Errorf("client_ip = %v", got)
	}
	if path(t, panel, 0, "handle", 0, "handler") != "static_response" || path(t, panel, 0, "handle", 0, "status_code") != float64(403) {
		t.Errorf("403: %v", path(t, panel, 0))
	}
	rp := path(t, panel, 1, "handle", 0)
	if path(t, rp, "handler") != "reverse_proxy" || path(t, rp, "upstreams", 0, "dial") != "muelle_muelle:8080" {
		t.Errorf("reverse_proxy: %v", rp)
	}
	if got := path(t, rp, "headers", "request", "set", "X-Real-IP"); !reflect.DeepEqual(got, []any{"{http.vars.client_ip}"}) {
		t.Errorf("X-Real-IP = %v", got)
	}

	// Sin IPs del panel, el subroute solo tiene el reverse_proxy.
	panel = path(t, decode(t, render(t, goldenCases()["panel_only"])), "apps", "http", "servers", "muelle", "routes", 0, "handle", 0, "routes")
	if len(panel.([]any)) != 1 || path(t, panel, 0, "handle", 0, "handler") != "reverse_proxy" {
		t.Errorf("sin IPs: %v", panel)
	}
}

func TestRenderTLSPolicies(t *testing.T) {
	subjects := func(p any) []any { return path(t, p, "subjects").([]any) }
	m := decode(t, render(t, goldenCases()["full"]))
	policies := path(t, m, "apps", "tls", "automation", "policies").([]any)
	if len(policies) != 2 {
		t.Fatalf("políticas: %v", policies)
	}
	if got := subjects(policies[0]); !reflect.DeepEqual(got, []any{"api.auditfy.cl", "panel.auditfy.cl", "web.auditfy.cl"}) {
		t.Errorf("cloudflare: %v", got)
	}
	dns := path(t, policies[0], "issuers", 0, "challenges", "dns")
	if path(t, dns, "provider", "name") != "cloudflare" || !reflect.DeepEqual(path(t, dns, "resolvers"), []any{"1.1.1.1:53", "1.0.0.1:53"}) {
		t.Errorf("desafío DNS: %v", dns)
	}
	if got := subjects(policies[1]); !reflect.DeepEqual(got, []any{"grafana.ejemplo.com"}) {
		t.Errorf("http: %v", got)
	}
	for _, p := range policies {
		if path(t, p, "issuers", 0, "module") != "acme" || path(t, p, "issuers", 0, "email") != "ops@auditfy.cl" {
			t.Errorf("emisor: %v", p)
		}
	}
	if path(t, m, "apps", "pki") != nil {
		t.Error("sin InternalCA no va pki")
	}

	// Sin token, todo en una política http, sin desafío DNS.
	policies = path(t, decode(t, render(t, goldenCases()["no_token"])), "apps", "tls", "automation", "policies").([]any)
	if len(policies) != 1 || len(subjects(policies[0])) != 4 || path(t, policies[0], "issuers", 0, "challenges") != nil {
		t.Errorf("sin token: %v", policies)
	}

	if b := render(t, goldenCases()["no_email"]); bytes.Contains(b, []byte(`"email"`)) {
		t.Errorf("sin email no va la clave: %s", b)
	}

	m = decode(t, render(t, goldenCases()["internal"]))
	for _, p := range path(t, m, "apps", "tls", "automation", "policies").([]any) {
		if got := path(t, p, "issuers"); !reflect.DeepEqual(got, []any{map[string]any{"module": "internal"}}) {
			t.Errorf("InternalCA: emisores %v", got)
		}
	}
	if got := path(t, m, "apps", "pki", "certificate_authorities", "local", "install_trust"); got != false {
		t.Errorf("install_trust = %v", got)
	}

	// Sin panel ni dominios no hay políticas, y no va apps.tls.
	if m := decode(t, render(t, service.ProxyConfig{Routes: []service.Route{}})); path(t, m, "apps", "tls") != nil {
		t.Errorf("sin políticas: %v", path(t, m, "apps"))
	}
}

// Origin CA (§7.2 del diseño de Origin CA): cada certificado en
// apps.tls.certificates.load_pem, por hostname y con sus tags; el host en
// automatic_https.skip_certificates, ordenado, y en ninguna política;
// disable_ocsp_stapling solo si hay alguno. ACME sigue igual para los demás.
func TestRenderOriginCA(t *testing.T) {
	m := decode(t, render(t, goldenCases()["origin_ca"]))
	pems := path(t, m, "apps", "tls", "certificates", "load_pem").([]any)
	if len(pems) != 2 {
		t.Fatalf("load_pem: %v", pems)
	}
	for i, host := range []string{"api.auditfy.cl", "panel.auditfy.cl"} {
		want := originPEM(host)
		if path(t, pems[i], "certificate") != want.CertPEM || path(t, pems[i], "key") != want.KeyPEM {
			t.Errorf("load_pem[%d]: %v", i, pems[i])
		}
		if got := path(t, pems[i], "tags"); !reflect.DeepEqual(got, []any{"muelle_origin_ca", "muelle_origin_ca:" + host}) {
			t.Errorf("tags de %s: %v", host, got)
		}
	}
	srv := path(t, m, "apps", "http", "servers", "muelle")
	if got := path(t, srv, "automatic_https", "skip_certificates"); !reflect.DeepEqual(got, []any{"api.auditfy.cl", "panel.auditfy.cl"}) {
		t.Errorf("skip_certificates: %v", got)
	}
	if path(t, m, "apps", "tls", "disable_ocsp_stapling") != true {
		t.Error("disable_ocsp_stapling")
	}
	policies := path(t, m, "apps", "tls", "automation", "policies").([]any)
	if len(policies) != 2 || !reflect.DeepEqual(path(t, policies[0], "subjects"), []any{"web.auditfy.cl"}) ||
		!reflect.DeepEqual(path(t, policies[1], "subjects"), []any{"grafana.ejemplo.com"}) {
		t.Errorf("políticas: %v", policies)
	}
	// Las rutas siguen siendo las mismas, con los mismos hosts.
	if routes := path(t, srv, "routes").([]any); len(routes) != 4 || path(t, routes[1], "match", 0, "host", 0) != "api.auditfy.cl" {
		t.Errorf("rutas: %v", routes)
	}

	// Todos con Origin CA: certificates sin automation.
	m = decode(t, render(t, goldenCases()["origin_ca_all"]))
	if path(t, m, "apps", "tls", "automation") != nil || len(path(t, m, "apps", "tls", "certificates", "load_pem").([]any)) != 3 {
		t.Errorf("todos con Origin CA: %v", path(t, m, "apps", "tls"))
	}
	// Sin ninguno, nada de esto (los golden de antes no cambian).
	for _, name := range []string{"full", "no_token", "internal", "panel_only", "no_email"} {
		b := render(t, goldenCases()[name])
		for _, k := range []string{"load_pem", "skip_certificates", "disable_ocsp_stapling", "automatic_https", "certificates"} {
			if bytes.Contains(b, []byte(`"`+k+`"`)) {
				t.Errorf("%s trae %s", name, k)
			}
		}
	}
}

// D16: Caddy expande placeholders en el email del emisor y en api_token: con
// { o } no se arma nada.
func TestRenderRejectsPlaceholders(t *testing.T) {
	base := goldenCases()["full"]
	for name, mod := range map[string]func(*service.ProxyConfig){
		"email":        func(c *service.ProxyConfig) { c.ACMEEmail = "a{x}@b.cl" },
		"token":        func(c *service.ProxyConfig) { c.CloudflareToken = "{file./etc/passwd}" },
		"token cierre": func(c *service.ProxyConfig) { c.CloudflareToken = "cf-tok}en-1234567890" },
		// Un PEM de Origin CA no los trae (es base64), pero Caddy también
		// los expandiría (load_pem pasa por ReplaceKnown).
		"certificado": func(c *service.ProxyConfig) {
			c.Routes = withOrigin(threeRoutes(), "api.auditfy.cl")
			c.Routes[0].OriginCA.CertPEM = "-----BEGIN CERTIFICATE-----\n{env.X}\n-----END CERTIFICATE-----\n"
		},
		"clave": func(c *service.ProxyConfig) {
			c.Panel = originPanel(entity.TLSCloudflare)
			c.Panel.OriginCA.KeyPEM = "{file./etc/shadow}"
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := base
			mod(&cfg)
			b, err := Render(cfg)
			if !errors.Is(err, service.ErrPlaceholderInSettings) || b != nil {
				t.Fatalf("Render: %q %v", b, err)
			}
			if strings.Contains(err.Error(), cfg.CloudflareToken) {
				t.Errorf("el error trae el token: %v", err)
			}
		})
	}
}

func TestRenderDeterministic(t *testing.T) {
	cfg := goldenCases()["full"]
	first := render(t, cfg)
	for range 50 {
		if b := render(t, cfg); !bytes.Equal(b, first) {
			t.Fatalf("Render cambió:\n%s\n%s", first, b)
		}
	}
}

func TestCloudflareRanges(t *testing.T) {
	v4, v6 := 0, 0
	for _, s := range CloudflareRanges {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			t.Errorf("%q: %v", s, err)
			continue
		}
		if p.Masked().String() != s {
			t.Errorf("%q no está en forma canónica (%s)", s, p.Masked())
		}
		if p.Addr().Is4() {
			v4++
		} else {
			v6++
		}
	}
	if v4 != 15 || v6 != 7 {
		t.Errorf("%d IPv4 y %d IPv6, esperaba 15 y 7", v4, v6)
	}
}
