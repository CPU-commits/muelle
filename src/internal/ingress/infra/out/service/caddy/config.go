// Package caddy es el adaptador de muelle-caddy (spec §4.2): el JSON de la
// configuración (Render), el cliente de la admin API por el socket unix
// (Admin) y el ciclo de vida del contenedor. caddytest es su admin API falsa.
package caddy

import (
	"encoding/json/v2"
	"fmt"
	"slices"
	"strings"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
)

const (
	// serverName es el único servidor HTTP de la configuración.
	serverName = "muelle"
	// clientIPPlaceholder es la IP real del cliente que resuelve
	// trusted_proxies. Va con su nombre completo: {client_ip} es un atajo del
	// Caddyfile que en JSON no se reemplaza (contrato §3.4).
	clientIPPlaceholder = "{http.vars.client_ip}"
)

// clientIPHeaders son los headers que dicen la IP del cliente, solo desde
// trusted_proxies.
var clientIPHeaders = []string{"CF-Connecting-IP", "X-Forwarded-For"}

// dnsResolvers son los que usa el desafío DNS-01 para ver la propagación: los
// de Cloudflare, no los del host.
var dnsResolvers = []string{"1.1.1.1:53", "1.0.0.1:53"}

// originTag va en cada certificado de Origin CA de load_pem, y
// originTag:<hostname> lo nombra: nadie los usa, sirven para reconocerlos
// en GET /config/ (§7.2 del diseño de Origin CA).
const originTag = "muelle_origin_ca"

// Los structs del JSON de Caddy. El orden de los campos es el del JSON.

type config struct {
	Admin adminConfig `json:"admin"`
	Apps  appsConfig  `json:"apps"`
}

type adminConfig struct {
	Listen string `json:"listen"`
}

type appsConfig struct {
	HTTP httpApp `json:"http"`
	TLS  *tlsApp `json:"tls,omitzero"`
	PKI  *pkiApp `json:"pki,omitzero"`
}

type httpApp struct {
	Servers map[string]server `json:"servers"`
}

type server struct {
	Listen               []string       `json:"listen"`
	TrustedProxies       trustedProxies `json:"trusted_proxies"`
	ClientIPHeaders      []string       `json:"client_ip_headers"`
	TrustedProxiesStrict int            `json:"trusted_proxies_strict"`
	Routes               []route        `json:"routes"`
	AutomaticHTTPS       *autoHTTPS     `json:"automatic_https,omitzero"`
}

// autoHTTPS: los hosts de skip_certificates no los maneja ACME (los de
// Origin CA), pero siguen con el redirect a HTTPS.
type autoHTTPS struct {
	SkipCertificates []string `json:"skip_certificates"`
}

type trustedProxies struct {
	Source string   `json:"source"`
	Ranges []string `json:"ranges"`
}

type route struct {
	Match    []matcherSet `json:"match,omitempty"`
	Handle   []any        `json:"handle"`
	Terminal bool         `json:"terminal,omitzero"`
}

type matcherSet struct {
	Host     []string     `json:"host,omitempty"`
	Not      []matcherSet `json:"not,omitempty"`
	ClientIP *ipRanges    `json:"client_ip,omitzero"`
}

type ipRanges struct {
	Ranges []string `json:"ranges"`
}

type subroute struct {
	Handler string  `json:"handler"` // "subroute"
	Routes  []route `json:"routes"`
}

type staticResponse struct {
	Handler    string `json:"handler"` // "static_response"
	StatusCode int    `json:"status_code"`
}

type reverseProxy struct {
	Handler   string        `json:"handler"` // "reverse_proxy"
	Upstreams []upstream    `json:"upstreams"`
	Headers   *proxyHeaders `json:"headers,omitzero"`
}

type upstream struct {
	Dial string `json:"dial"`
}

type proxyHeaders struct {
	Request headerOps `json:"request"`
}

type headerOps struct {
	Set map[string][]string `json:"set"`
}

type tlsApp struct {
	Certificates        *tlsCertificates `json:"certificates,omitzero"`
	Automation          *automation      `json:"automation,omitzero"`
	DisableOCSPStapling bool             `json:"disable_ocsp_stapling,omitzero"`
}

type tlsCertificates struct {
	LoadPEM []pemPair `json:"load_pem"`
}

// pemPair es un certificado cargado: la hoja y su clave en PEM.
type pemPair struct {
	Certificate string   `json:"certificate"`
	Key         string   `json:"key"`
	Tags        []string `json:"tags"`
}

type automation struct {
	Policies []policy `json:"policies"`
}

type policy struct {
	Subjects []string `json:"subjects"`
	Issuers  []issuer `json:"issuers"`
}

type issuer struct {
	Module     string      `json:"module"` // "acme" o "internal"
	Email      string      `json:"email,omitempty"`
	Challenges *challenges `json:"challenges,omitzero"`
}

type challenges struct {
	DNS dnsChallenge `json:"dns"`
}

type dnsChallenge struct {
	Provider  dnsProvider `json:"provider"`
	Resolvers []string    `json:"resolvers"`
}

type dnsProvider struct {
	Name     string `json:"name"` // "cloudflare"
	APIToken string `json:"api_token"`
}

type pkiApp struct {
	CertificateAuthorities map[string]certificateAuthority `json:"certificate_authorities"`
}

type certificateAuthority struct {
	InstallTrust bool `json:"install_trust"`
}

// Render arma el JSON que se manda con POST /load (contrato §3.4, §8 del
// spec, D16 del plan de la Fase 3):
//   - admin.listen siempre es el socket (naming.CaddyAdminListen);
//   - un servidor "muelle" en :443 con trusted_proxies estáticos (los rangos
//     de Cloudflare), client_ip_headers y trusted_proxies_strict;
//   - la ruta del panel va primero, como subroute: el 403 para las IPs que no
//     están en AllowedCIDRs (solo si hay una lista) y el reverse_proxy con
//     X-Real-IP; después las de los dominios, en el orden de cfg.Routes;
//     todas terminal;
//   - TLS en hasta dos políticas con subjects ordenados: cloudflare (DNS-01,
//     solo con token) y http. Con InternalCA, los emisores son la CA interna
//     de Caddy;
//   - un host con un certificado de Origin CA (OriginCA, contrato §3.7) no
//     va en ninguna política: su certificado va en
//     apps.tls.certificates.load_pem (por hostname, con sus tags), el host en
//     automatic_https.skip_certificates (ordenados) y, si hay alguno,
//     apps.tls.disable_ocsp_stapling (Caddy no espera al OCSP al cargarlos);
//   - sin políticas no va automation, y sin políticas ni certificados no va
//     apps.tls.
//
// Si el email ACME, el token o un certificado de Origin CA tienen { o },
// devuelve service.ErrPlaceholderInSettings sin armar nada: Caddy los
// expandiría como placeholders. Se serializa con orden fijo (los golden
// dependen de eso).
func Render(cfg service.ProxyConfig) ([]byte, error) {
	if strings.ContainsAny(cfg.ACMEEmail, "{}") || strings.ContainsAny(cfg.CloudflareToken, "{}") {
		return nil, service.ErrPlaceholderInSettings
	}
	hasToken := cfg.CloudflareToken != ""
	var cfHosts, httpHosts, skip []string
	var pems []pemPair
	addHost := func(host string, tls entity.TLSMode, origin *service.OriginPEM) {
		if origin != nil {
			skip = append(skip, host)
			pems = append(pems, pemPair{Certificate: origin.CertPEM, Key: origin.KeyPEM, Tags: []string{originTag, originTag + ":" + host}})
			return
		}
		if hasToken && tls == entity.TLSCloudflare {
			cfHosts = append(cfHosts, host)
		} else {
			httpHosts = append(httpHosts, host)
		}
	}

	routes := make([]route, 0, len(cfg.Routes)+1)
	if p := cfg.Panel; p != nil {
		routes = append(routes, panelRoute(p))
		addHost(p.Hostname, p.TLS, p.OriginCA)
	}
	for _, r := range cfg.Routes {
		routes = append(routes, route{
			Match:    []matcherSet{{Host: []string{r.Hostname}}},
			Handle:   []any{reverseProxy{Handler: "reverse_proxy", Upstreams: []upstream{{Dial: r.Upstream}}}},
			Terminal: true,
		})
		addHost(r.Hostname, r.TLS, r.OriginCA)
	}
	for _, p := range pems {
		if strings.ContainsAny(p.Certificate, "{}") || strings.ContainsAny(p.Key, "{}") {
			return nil, service.ErrPlaceholderInSettings
		}
	}

	srv := server{
		Listen:               []string{":443"},
		TrustedProxies:       trustedProxies{Source: "static", Ranges: CloudflareRanges},
		ClientIPHeaders:      clientIPHeaders,
		TrustedProxiesStrict: 1,
		Routes:               routes,
	}
	if len(skip) > 0 {
		slices.Sort(skip)
		srv.AutomaticHTTPS = &autoHTTPS{SkipCertificates: skip}
	}
	out := config{
		Admin: adminConfig{Listen: naming.CaddyAdminListen},
		Apps:  appsConfig{HTTP: httpApp{Servers: map[string]server{serverName: srv}}},
	}

	var policies []policy
	if len(cfHosts) > 0 {
		policies = append(policies, newPolicy(cfHosts, cfg, true))
	}
	if len(httpHosts) > 0 {
		policies = append(policies, newPolicy(httpHosts, cfg, false))
	}
	var tls tlsApp
	if len(policies) > 0 {
		tls.Automation = &automation{Policies: policies}
	}
	if len(pems) > 0 {
		slices.SortFunc(pems, func(a, b pemPair) int { return strings.Compare(a.Tags[1], b.Tags[1]) })
		tls.Certificates = &tlsCertificates{LoadPEM: pems}
		tls.DisableOCSPStapling = true
	}
	if tls.Automation != nil || tls.Certificates != nil {
		out.Apps.TLS = &tls
	}
	if cfg.InternalCA {
		out.Apps.PKI = &pkiApp{CertificateAuthorities: map[string]certificateAuthority{"local": {InstallTrust: false}}}
	}

	b, err := json.Marshal(out, json.Deterministic(true))
	if err != nil {
		// Son structs propios: no pasa. El error de json no trae valores.
		return nil, fmt.Errorf("armando la configuración de Caddy: %w", err)
	}
	return b, nil
}

// panelRoute es la ruta del panel (contrato §3.4).
func panelRoute(p *service.PanelRoute) route {
	var sub []route
	if len(p.AllowedCIDRs) > 0 {
		sub = append(sub, route{
			Match:  []matcherSet{{Not: []matcherSet{{ClientIP: &ipRanges{Ranges: p.AllowedCIDRs}}}}},
			Handle: []any{staticResponse{Handler: "static_response", StatusCode: 403}},
		})
	}
	sub = append(sub, route{Handle: []any{reverseProxy{
		Handler:   "reverse_proxy",
		Upstreams: []upstream{{Dial: p.Upstream}},
		Headers:   &proxyHeaders{Request: headerOps{Set: map[string][]string{"X-Real-IP": {clientIPPlaceholder}}}},
	}}})
	return route{
		Match:    []matcherSet{{Host: []string{p.Hostname}}},
		Handle:   []any{subroute{Handler: "subroute", Routes: sub}},
		Terminal: true,
	}
}

// newPolicy es la política de esos hosts (ordenados): ACME con DNS-01 en
// Cloudflare (dns) o sin desafío DNS, o la CA interna con InternalCA.
func newPolicy(hosts []string, cfg service.ProxyConfig, dns bool) policy {
	hosts = slices.Clone(hosts)
	slices.Sort(hosts)
	iss := issuer{Module: "acme", Email: cfg.ACMEEmail}
	switch {
	case cfg.InternalCA:
		iss = issuer{Module: "internal"}
	case dns:
		iss.Challenges = &challenges{DNS: dnsChallenge{
			Provider:  dnsProvider{Name: "cloudflare", APIToken: cfg.CloudflareToken},
			Resolvers: dnsResolvers,
		}}
	}
	return policy{Subjects: hosts, Issuers: []issuer{iss}}
}
