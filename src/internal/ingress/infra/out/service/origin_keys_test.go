package service

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
)

const originHost = "api.auditfy.cl"

// signCSR firma la clave pública del CSR con ca (una hoja de Origin CA de
// prueba) para hosts, de notBefore a notAfter.
func signCSR(t *testing.T, ca *testCA, csrPEM string, notBefore, notAfter time.Time, hosts ...string) string {
	t.Helper()
	block, _ := pem.Decode([]byte(csrPEM))
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "CloudFlare Origin Certificate"},
		DNSNames: hosts, NotBefore: notBefore, NotAfter: notAfter,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, _ := sign(t, tmpl, ca.root, csr.PublicKey, ca.rootKey)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// Generate: una clave ECDSA P-256 en PKCS#8 ("PRIVATE KEY") y un CSR con
// CN y SAN del hostname, firmado con esa clave.
func TestOriginKeysGenerate(t *testing.T) {
	keyPEM, csrPEM, err := OriginKeys{}.Generate(originHost)
	if err != nil {
		t.Fatal(err)
	}
	kb, rest := pem.Decode([]byte(keyPEM))
	if kb == nil || kb.Type != "PRIVATE KEY" || len(strings.TrimSpace(string(rest))) != 0 {
		t.Fatalf("clave: %q", kb)
	}
	k, err := x509.ParsePKCS8PrivateKey(kb.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	ek, ok := k.(*ecdsa.PrivateKey)
	if !ok || ek.Curve != elliptic.P256() {
		t.Fatalf("no es P-256: %T", k)
	}
	cb, _ := pem.Decode([]byte(csrPEM))
	if cb == nil || cb.Type != "CERTIFICATE REQUEST" {
		t.Fatalf("CSR: %q", csrPEM)
	}
	csr, err := x509.ParseCertificateRequest(cb.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatalf("la firma del CSR: %v", err)
	}
	if csr.Subject.CommonName != originHost || len(csr.DNSNames) != 1 || csr.DNSNames[0] != originHost {
		t.Fatalf("CN y SAN: %q %q", csr.Subject.CommonName, csr.DNSNames)
	}
	if pub, ok := csr.PublicKey.(*ecdsa.PublicKey); !ok || !pub.Equal(&ek.PublicKey) {
		t.Fatal("el CSR no es de la clave")
	}
	// Cada llamada es una clave nueva.
	other, _, err := OriginKeys{}.Generate(originHost)
	if err != nil || other == keyPEM {
		t.Fatalf("la misma clave dos veces: %v", err)
	}
}

// Check acepta la hoja que va con la clave, para el hostname y vigente, y
// devuelve el PEM normalizado desde el DER (sin la cadena) y el NotAfter.
func TestOriginKeysCheck(t *testing.T) {
	ca := newTestCA(t, "")
	now := time.Now().UTC().Truncate(time.Second)
	keyPEM, csrPEM, err := OriginKeys{}.Generate(originHost)
	if err != nil {
		t.Fatal(err)
	}
	notAfter := now.Add(90 * 24 * time.Hour)
	certPEM := signCSR(t, ca, csrPEM, now.Add(-time.Minute), notAfter, originHost)
	// Con CRLF y la raíz detrás: se normaliza a la hoja sola.
	messy := strings.ReplaceAll(certPEM, "\n", "\r\n") + string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.root.Raw}))
	leafPEM, got, err := OriginKeys{}.Check(messy, keyPEM, originHost, now)
	if err != nil {
		t.Fatal(err)
	}
	if leafPEM != certPEM || !got.Equal(notAfter) {
		t.Fatalf("hoja %q vence %s", leafPEM, got)
	}
	// Uno que vale recién en 3 min también pasa (a lo más 5 min de reloj).
	soon := signCSR(t, ca, csrPEM, now.Add(3*time.Minute), notAfter, originHost)
	if _, _, err := (OriginKeys{}).Check(soon, keyPEM, originHost, now); err != nil {
		t.Fatalf("vale en 3 min: %v", err)
	}

	otherKey, _, err := OriginKeys{}.Generate(originHost)
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct{ cert, key, host string }{
		"otra clave":           {certPEM, otherKey, originHost},
		"otro hostname":        {certPEM, keyPEM, "web.auditfy.cl"},
		"vencido":              {signCSR(t, ca, csrPEM, now.Add(-48*time.Hour), now.Add(-time.Hour), originHost), keyPEM, originHost},
		"vence justo ahora":    {signCSR(t, ca, csrPEM, now.Add(-48*time.Hour), now.Add(5*time.Minute), originHost), keyPEM, originHost},
		"todavía no vale":      {signCSR(t, ca, csrPEM, now.Add(time.Hour), notAfter, originHost), keyPEM, originHost},
		"no es un certificado": {"-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n", keyPEM, originHost},
		"vacío":                {"", keyPEM, originHost},
	} {
		_, _, err := OriginKeys{}.Check(c.cert, c.key, c.host, now)
		ce, ok := errors.AsType[*service.OriginCertError](err)
		if !ok || ce.Reason == "" {
			t.Errorf("%s: %v, esperaba un OriginCertError", name, err)
			continue
		}
		// El motivo nunca trae la clave.
		if strings.Contains(err.Error(), "PRIVATE KEY") || strings.Contains(err.Error(), strings.Split(keyPEM, "\n")[1]) {
			t.Errorf("%s: el error trae la clave: %v", name, err)
		}
	}
}
