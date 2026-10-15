package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
)

// testCA es una CA de prueba: raíz, intermedia y hojas firmadas por la intermedia.
type testCA struct {
	root      *x509.Certificate
	inter     *x509.Certificate
	interKey  *ecdsa.PrivateKey
	interDER  []byte
	rootsPool *x509.CertPool
}

func newKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func sign(t *testing.T, tmpl, parent *x509.Certificate, pub any, key *ecdsa.PrivateKey) ([]byte, *x509.Certificate) {
	t.Helper()
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return der, c
}

// newTestCA arma la raíz y la intermedia. interOrg vacío: la intermedia no
// tiene Organization, solo CN.
func newTestCA(t *testing.T, interOrg string) *testCA {
	t.Helper()
	now := time.Now()
	rootKey := newKey(t)
	rootTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "it3 raíz", Organization: []string{"it3 Raíz"}},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	_, root := sign(t, rootTmpl, rootTmpl, &rootKey.PublicKey, rootKey)
	interKey := newKey(t)
	subject := pkix.Name{CommonName: "it3 Intermedia CN"}
	if interOrg != "" {
		subject.Organization = []string{interOrg}
	}
	interTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: subject,
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	interDER, inter := sign(t, interTmpl, root, &interKey.PublicKey, rootKey)
	pool := x509.NewCertPool()
	pool.AddCert(root)
	return &testCA{root: root, inter: inter, interKey: interKey, interDER: interDER, rootsPool: pool}
}

// leaf es una hoja para host que vence en notAfter, con la intermedia en la cadena.
func (ca *testCA) leaf(t *testing.T, host string, notAfter time.Time) (tls.Certificate, *x509.Certificate) {
	t.Helper()
	key := newKey(t)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: time.Now().Add(-2 * time.Hour), NotAfter: notAfter,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, c := sign(t, tmpl, ca.inter, &key.PublicKey, ca.interKey)
	return tls.Certificate{Certificate: [][]byte{der, ca.interDER}, PrivateKey: key}, c
}

// serveTLS arranca un servidor TLS con cert y devuelve su dirección.
func serveTLS(t *testing.T, cert tls.Certificate) string {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.Config.ErrorLog = log.New(io.Discard, "", 0) // los handshakes que corta la sonda
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().String()
}

const probeHost = "a.it3.test"

func TestProbeValid(t *testing.T) {
	for _, c := range []struct{ org, want string }{
		{"it3 Intermedia S.A.", "it3 Intermedia S.A."},
		{"", "it3 Intermedia CN"},
	} {
		ca := newTestCA(t, c.org)
		notAfter := time.Now().Add(20 * time.Hour)
		cert, leaf := ca.leaf(t, probeHost, notAfter)
		p := TLSProbe{Addr: serveTLS(t, cert), Roots: ca.rootsPool, Timeout: 5 * time.Second}
		o, err := p.Probe(context.Background(), probeHost)
		if err != nil {
			t.Fatal(err)
		}
		wantAfter := leaf.NotAfter.UTC().Truncate(time.Millisecond)
		if !o.Valid || o.Issuer != c.want || !o.NotAfter.Equal(wantAfter) || o.NotAfter.Location() != time.UTC || o.Problem != "" {
			t.Fatalf("org %q: %+v (esperaba issuer %q, vence %s)", c.org, o, c.want, wantAfter)
		}
	}
}

func TestProbeUnknownCA(t *testing.T) {
	ca := newTestCA(t, "it3")
	cert, _ := ca.leaf(t, probeHost, time.Now().Add(20*time.Hour))
	p := TLSProbe{Addr: serveTLS(t, cert), Roots: x509.NewCertPool()}
	o, err := p.Probe(context.Background(), probeHost)
	if err != nil {
		t.Fatal(err)
	}
	if o.Valid || !strings.Contains(o.Problem, "CA") || o.Issuer != "" {
		t.Fatalf("%+v", o)
	}
}

func TestProbeWrongName(t *testing.T) {
	ca := newTestCA(t, "it3")
	cert, _ := ca.leaf(t, probeHost, time.Now().Add(20*time.Hour))
	p := TLSProbe{Addr: serveTLS(t, cert), Roots: ca.rootsPool}
	o, err := p.Probe(context.Background(), "b.it3.test")
	if err != nil {
		t.Fatal(err)
	}
	if o.Valid || !strings.Contains(o.Problem, "b.it3.test") {
		t.Fatalf("%+v", o)
	}
}

func TestProbeExpired(t *testing.T) {
	ca := newTestCA(t, "it3")
	cert, _ := ca.leaf(t, probeHost, time.Now().Add(-time.Minute))
	p := TLSProbe{Addr: serveTLS(t, cert), Roots: ca.rootsPool}
	o, err := p.Probe(context.Background(), probeHost)
	if err != nil {
		t.Fatal(err)
	}
	if o.Valid || !strings.Contains(o.Problem, "venció") {
		t.Fatalf("%+v", o)
	}
}

// Caddy sin un certificado para ese SNI corta el handshake: inválido, no un error.
func TestProbeHandshakeFails(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	p := TLSProbe{Addr: l.Addr().String(), Roots: x509.NewCertPool()}
	o, err := p.Probe(context.Background(), probeHost)
	if err != nil {
		t.Fatal(err)
	}
	if o.Valid || o.Problem != "Caddy no presentó un certificado para "+probeHost {
		t.Fatalf("%+v", o)
	}
}

func TestProbeUnreachable(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	p := TLSProbe{Addr: addr}
	if _, err := p.Probe(context.Background(), probeHost); !errors.Is(err, service.ErrCaddyUnreachable) {
		t.Fatalf("err: %v", err)
	}
}

// Un handshake que no termina a tiempo (Caddy colgado) es "no se sabe", no
// un certificado inválido: si no, un Caddy lento pasaría los active a error.
func TestProbeHandshakeTimeout(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	conns := make(chan net.Conn, 4)
	t.Cleanup(func() {
		l.Close()
		close(conns)
		for c := range conns {
			c.Close()
		}
	})
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			conns <- c // no responde: el handshake queda esperando
		}
	}()
	p := TLSProbe{Addr: l.Addr().String(), Timeout: 50 * time.Millisecond}
	start := time.Now()
	if _, err := p.Probe(context.Background(), probeHost); !errors.Is(err, service.ErrCaddyUnreachable) {
		t.Fatalf("err: %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("tardó %s", d)
	}
}
