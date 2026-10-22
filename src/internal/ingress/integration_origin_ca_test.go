//go:build integration

package ingress

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	reposqlite "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/repository/sqlite"
	outservice "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// Los certificados de Cloudflare (Origin CA) contra Caddy real (§13.2 del
// diseño de Origin CA): las ranuras 4 y 5 de it3. Corren con caddy:2-alpine
// y, si MUELLE_IT_CADDY_IMAGE nombra una imagen que está en el nodo (la
// muelle-caddy local, por ejemplo it5-muelle-caddy:test, construida desde
// deploy/caddy), también con esa: la de producción es Caddy 2.11.7 fijo
// (riesgo R14). La imagen no se baja ni se borra: solo se usa.
const itCaddyImageEnv = "MUELLE_IT_CADDY_IMAGE"

// originImages son las imágenes de muelle-caddy con que corren estos tests.
func originImages(t *testing.T) []string {
	images := []string{itCaddyImage}
	if img := strings.TrimSpace(os.Getenv(itCaddyImageEnv)); img != "" && img != itCaddyImage {
		images = append(images, img)
	} else {
		t.Logf("sin %s: solo con %s", itCaddyImageEnv, itCaddyImage)
	}
	return images
}

// forEachImage corre f en un subtest por imagen (cada uno con su entorno y
// su limpieza). Una imagen que no está en el nodo se salta.
func forEachImage(t *testing.T, slotN int, f func(t *testing.T, e *itEnv, image string)) {
	for _, img := range originImages(t) {
		t.Run(strings.NewReplacer("/", "_", ":", "_").Replace(img), func(t *testing.T) {
			e := newITEnv(t, slotN)
			if _, err := e.docker.InspectImage(context.Background(), img); err != nil {
				t.Skipf("falta la imagen %s en el nodo: %v", img, err)
			}
			f(t, e, img)
		})
	}
}

// selfSigned es un certificado autofirmado ECDSA para host, con su clave en
// PKCS#8, los dos en PEM, y la hoja en DER.
func selfSigned(t *testing.T, host string) (certPEM, keyPEM string, der []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host, Organization: []string{"it3 Origin CA"}},
		DNSNames: []string{host}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(90 * 24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err = x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	k, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: k})), der
}

// chain es la cadena que sirve Caddy para host (SNI), sin verificarla: la
// hoja primero.
func (e *itEnv) chain(host string) ([]*x509.Certificate, error) {
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(e.ports.HTTPS))
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", addr, &tls.Config{ServerName: host, InsecureSkipVerify: true})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("sin certificado para %s", host)
	}
	return certs, nil
}

// servedBy es un check de eventually: Caddy sirve para host una hoja que
// verifica contra roots, con los intermedios que manda (y, si want no es
// nil, idéntica a want).
func (e *itEnv) servedBy(host string, roots *x509.CertPool, want []byte) func() (bool, error) {
	return func() (bool, error) {
		chain, err := e.chain(host)
		if err != nil {
			return false, err
		}
		l := chain[0]
		if want != nil && !bytes.Equal(l.Raw, want) {
			return false, fmt.Errorf("%s: otra hoja (emisor %q)", host, l.Issuer.String())
		}
		inter := x509.NewCertPool()
		for _, c := range chain[1:] {
			inter.AddCert(c)
		}
		if _, err := l.Verify(x509.VerifyOptions{DNSName: host, Roots: roots, Intermediates: inter}); err != nil {
			return false, fmt.Errorf("%s: %v (emisor %q)", host, err, l.Issuer.String())
		}
		return true, nil
	}
}

// Caddy real carga un PEM con load_pem para un host (skip_certificates y
// disable_ocsp_stapling) mientras la CA interna sirve a otro, y al pasar el
// host a DNS only vuelve a la CA interna: la vuelta a ACME funciona.
func TestIntegrationCaddyLoadPEM(t *testing.T) {
	forEachImage(t, 4, func(t *testing.T, e *itEnv, image string) {
		const token = "cf-token-it3-1234567890abcdef"
		e.cf.AddToken(token)
		m := e.module()
		ctx := context.Background()

		e.startAdminBridge()
		ingresstest.Stage(t, e.db, e.project, "s1")
		ocaID := ingresstest.App(t, e.db, e.project, "s1", "oca", "service")
		webID := ingresstest.App(t, e.db, e.project, "s1", "web", "service")
		oca, web := e.host("oca"), e.host("web")
		d := e.addDomain(ocaID, oca, "oca", 8080)
		d.TLSMode, d.CFZoneID, d.CFRecordID, d.Proxied = entity.TLSCloudflare, "z1", "rec-oca", true
		if err := reposqlite.NewDomains(e.db).Update(ctx, d); err != nil {
			t.Fatal(err)
		}
		e.addDomain(webID, web, "web", 8080)
		certPEM, keyPEM, der := selfSigned(t, oca)
		c := entity.OriginCert{Hostname: oca, ZoneID: "z1", CFCertID: "it3-" + e.id, CertPEM: certPEM, KeyPEM: keyPEM,
			ExpiresAt: time.Now().Add(90 * 24 * time.Hour), CreatedAt: time.Now()}
		if err := reposqlite.NewOriginCerts(e.db, e.cipher).Insert(ctx, &c); err != nil {
			t.Fatal(err)
		}
		hosts := reposqlite.NewOriginHosts(e.db)
		if err := hosts.SetObserved(ctx, oca, entity.ObservedProxied, time.Now()); err != nil {
			t.Fatal(err)
		}
		p := shared.DefaultProxySettings()
		p.CaddyImage, p.CloudflareToken, p.CloudflareOriginCA = image, token, true
		e.settings(p)
		if err := m.EnsureStageNetwork(ctx, e.project, "s1"); err != nil {
			t.Fatal(err)
		}
		e.startBackend("s1", "oca")
		e.startBackend("s1", "web")

		// El job crea it3-<id>-caddy y le manda la configuración: Caddy acepta
		// load_pem, skip_certificates y disable_ocsp_stapling.
		if err := m.Jobs()[0].Run(ctx); err != nil {
			t.Fatalf("job proxy: %v", err)
		}
		e.eventually(30*time.Second, "synced", e.proxyStatus(m, "synced"))
		if body := e.caddyConfig(); !strings.Contains(body, `"load_pem"`) || !strings.Contains(body, `"skip_certificates":["`+oca+`"]`) {
			t.Fatalf("la configuración no trae load_pem: %s", strings.ReplaceAll(body, keyPEM, "***"))
		}

		// oca: la hoja que cargó muelle, idéntica. La sonda la acepta fijada.
		self := x509.NewCertPool()
		self.AddCert(mustParse(t, der))
		e.eventually(30*time.Second, "Caddy sirve el PEM cargado para "+oca, e.servedBy(oca, self, der))
		if o, err := (outservice.TLSProbe{Addr: e.opts.CaddyTLSAddr, Roots: x509.NewCertPool()}).Probe(ctx, oca, der); err != nil || !o.Valid {
			t.Fatalf("la sonda con la hoja fijada: %+v %v", o, err)
		}
		// web: la CA interna.
		roots := e.internalRoots()
		e.eventually(30*time.Second, "la CA interna para "+web, e.servedBy(web, roots, nil))
		if c, err := e.chain(web); err != nil || !strings.HasPrefix(c[0].Issuer.CommonName, "Caddy Local Authority") {
			t.Fatalf("el emisor de %s: %v", web, err)
		}
		e.eventually(30*time.Second, "GET "+oca, e.expectGet(oca, "/", http.StatusOK, "hola oca"))
		e.eventually(30*time.Second, "GET "+web, e.expectGet(web, "/", http.StatusOK, "hola web"))

		// oca en DNS only: vuelve a la CA interna.
		if err := hosts.SetObserved(ctx, oca, entity.ObservedDNSOnly, time.Now()); err != nil {
			t.Fatal(err)
		}
		if err := m.SyncRoutes(ctx); err != nil {
			t.Fatal(err)
		}
		e.eventually(60*time.Second, "la CA interna para "+oca, e.servedBy(oca, roots, nil))
		e.eventually(30*time.Second, "GET "+oca+" otra vez", e.expectGet(oca, "/", http.StatusOK, "hola oca"))
	})
}

// El camino completo con el Cloudflare falso: un dominio con proxy, la
// pasada emite, Caddy sirve la hoja de Origin CA, la API dice active, la
// verificación (con la hoja fijada) guarda el emisor, y con proxied false
// vuelve a la CA interna y el certificado se revoca después del margen.
func TestIntegrationOriginCAEndToEnd(t *testing.T) {
	forEachImage(t, 5, func(t *testing.T, e *itEnv, image string) {
		const token = "cf-token-it3-1234567890abcdef"
		e.cf.AddToken(token)
		e.cf.AddZone("z1", e.project+".test")
		e.cf.GrantSSL(token, "z1")
		e.opts.Timing.OriginRevokeGrace = time.Second
		m := e.module()
		ctx := context.Background()

		e.startAdminBridge()
		ingresstest.Stage(t, e.db, e.project, "s1")
		ingresstest.App(t, e.db, e.project, "s1", "web", "service")
		p := shared.DefaultProxySettings()
		p.CaddyImage, p.CloudflareToken, p.CloudflareOriginCA, p.PublicIP = image, token, true, "203.0.113.24"
		e.settings(p)
		if err := m.EnsureStageNetwork(ctx, e.project, "s1"); err != nil {
			t.Fatal(err)
		}
		e.startBackend("s1", "web")
		if err := m.Jobs()[0].Run(ctx); err != nil {
			t.Fatalf("job proxy: %v", err)
		}
		e.eventually(30*time.Second, "synced", e.proxyStatus(m, "synced"))

		client := httptestutil.New(t, httptestutil.FakeAuth{Sessions: map[string]shared.Principal{"s": {UserID: 1, Username: "felipe"}}},
			m.RegisterRoutes)
		domains := "/api/v1/projects/" + e.project + "/stages/s1/apps/web/domains"
		web := e.host("web")
		w := client.Do("POST", domains, map[string]any{"hostname": web, "target_service": "web", "target_port": 8080, "proxied": true},
			httptestutil.WithSession("s"))
		httptestutil.Expect(t, w, http.StatusCreated)
		got := httptestutil.Decode[map[string]any](t, w)
		if got["tls_mode"] != "cloudflare" || got["cf_record_id"] == nil || got["cert_source"] != "letsencrypt" ||
			got["origin_ca"].(map[string]any)["status"] != "issuing" {
			t.Fatalf("POST: %v", got)
		}

		// La pasada emite y le manda a Caddy el certificado.
		if err := m.origin.Execute(ctx); err != nil {
			t.Fatal(err)
		}
		issued := e.cf.Issued()
		if len(issued) != 1 || issued[0].Hostnames[0] != web || issued[0].Validity != 90 {
			t.Fatalf("emitidos: %+v", issued)
		}
		originRoots := x509.NewCertPool()
		originRoots.AddCert(e.cf.OriginRoot())
		e.eventually(30*time.Second, "Caddy sirve la hoja de Origin CA", e.servedBy(web, originRoots, nil))
		e.eventually(30*time.Second, "GET "+web, e.expectGet(web, "/", http.StatusOK, "hola web"))

		list := func() map[string]any {
			t.Helper()
			w := client.Do("GET", domains, nil, httptestutil.WithSession("s"))
			httptestutil.Expect(t, w, http.StatusOK)
			return httptestutil.Decode[[]map[string]any](t, w)[0]
		}
		d := list()
		if oc, _ := d["origin_ca"].(map[string]any); d["cert_source"] != "cloudflare_origin_ca" || oc["status"] != "active" || oc["expires_at"] == nil {
			t.Fatalf("GET: %v", d)
		}

		// El job "certificados", con la hoja fijada: active y el emisor.
		if err := m.Jobs()[1].Run(ctx); err != nil {
			t.Fatalf("job certificados: %v", err)
		}
		if d := list(); d["status"] != "active" || d["cert_issuer"] != "CloudFlare, Inc." {
			t.Fatalf("después de la verificación: %v", d)
		}

		// proxied false: vuelve a la CA interna enseguida, y el de Origin CA
		// se revoca después de una carga sin él y del margen.
		w = client.Do("PATCH", domains+"/"+web, map[string]any{"proxied": false}, httptestutil.WithSession("s"))
		httptestutil.Expect(t, w, http.StatusOK)
		if got := httptestutil.Decode[map[string]any](t, w); got["cert_source"] != "letsencrypt" ||
			got["origin_ca"].(map[string]any)["status"] != "dns_only" {
			t.Fatalf("PATCH: %v", got)
		}
		roots := e.internalRoots()
		e.eventually(60*time.Second, "la CA interna para "+web, e.servedBy(web, roots, nil))
		e.eventually(30*time.Second, "revocado", func() (bool, error) {
			if err := m.SyncRoutes(ctx); err != nil {
				return false, err
			}
			if err := m.origin.Execute(ctx); err != nil {
				return false, err
			}
			return e.cf.Revoked(issued[0].ID), nil
		})
	})
}

func mustParse(t *testing.T, der []byte) *x509.Certificate {
	t.Helper()
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// caddyConfig es la configuración que tiene Caddy (GET /config/ por el
// puente de la admin API).
func (e *itEnv) caddyConfig() string {
	e.t.Helper()
	c := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext:       func(ctx context.Context, _, _ string) (net.Conn, error) { return e.opts.CaddyAdminDial(ctx) },
			DisableKeepAlives: true,
		},
	}
	resp, err := c.Get("http://caddy/config/")
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var b bytes.Buffer
	if _, err := b.ReadFrom(resp.Body); err != nil {
		e.t.Fatal(err)
	}
	return b.String()
}
