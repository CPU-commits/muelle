package service

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
)

// originClockSkew es cuánto antes de su NotBefore se acepta un certificado
// recién emitido: el reloj de Cloudflare puede ir un poco adelante.
const originClockSkew = 5 * time.Minute

// OriginKeys implementa service.OriginKeys con crypto/x509 (§5.1 del diseño
// de Origin CA). No guarda nada.
type OriginKeys struct{}

var _ service.OriginKeys = OriginKeys{}

// Generate arma una clave ECDSA P-256 (en PKCS#8, como "PRIVATE KEY") y un
// CSR con Subject.CommonName y DNSNames = [hostname].
func (OriginKeys) Generate(hostname string) (keyPEM, csrPEM string, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generando la clave de %s: %w", hostname, err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", fmt.Errorf("guardando la clave de %s: %w", hostname, err)
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: hostname}, DNSNames: []string{hostname},
	}, key)
	if err != nil {
		return "", "", fmt.Errorf("armando el CSR de %s: %w", hostname, err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})), nil
}

// Check hace lo que hace Caddy al cargarlo (tls.X509KeyPair: el PEM se lee
// y la hoja, la primera, va con la clave) y además mira el hostname y la
// vigencia. Los motivos de un *OriginCertError nunca traen la clave: el de
// X509KeyPair no se cita.
func (OriginKeys) Check(certPEM, keyPEM, hostname string, now time.Time) (string, time.Time, error) {
	reject := func(format string, args ...any) (string, time.Time, error) {
		return "", time.Time{}, &service.OriginCertError{Reason: fmt.Sprintf(format, args...)}
	}
	pair, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil || len(pair.Certificate) == 0 {
		return reject("el certificado no se lee o no va con la clave que generó muelle")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return reject("la hoja no se lee")
	}
	if err := leaf.VerifyHostname(hostname); err != nil {
		return reject("no es para %s", hostname)
	}
	if soon := now.Add(originClockSkew); leaf.NotBefore.After(soon) || !soon.Before(leaf.NotAfter) {
		return reject("no vale ahora (de %s a %s)", leaf.NotBefore.UTC().Format(time.RFC3339), leaf.NotAfter.UTC().Format(time.RFC3339))
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw})), leaf.NotAfter.UTC(), nil
}
