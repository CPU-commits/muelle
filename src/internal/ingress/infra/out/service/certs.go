package service

import (
	"bytes"
	"cmp"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"slices"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
)

// DefaultProbeTimeout es el plazo de una verificación: la conexión y el
// handshake.
const DefaultProbeTimeout = 5 * time.Second

// TLSProbe verifica el certificado que sirve Caddy (implementa
// service.CertProbe, §8 del spec): se conecta a Addr (muelle-caddy:443 por
// muelle_panel) con SNI = hostname, sin verificar en el handshake, y después
// verifica la hoja contra Roots con el resto de la cadena que mandó Caddy.
type TLSProbe struct {
	// Addr es host:puerto del TLS de Caddy.
	Addr string
	// Roots verifican la cadena (nil: las raíces del sistema).
	Roots *x509.CertPool
	// Timeout es el plazo de la conexión y el handshake (0: DefaultProbeTimeout).
	Timeout time.Duration
}

var _ service.CertProbe = TLSProbe{}

// Probe sigue D9 del plan de la Fase 3 y §8 del diseño de Origin CA:
//   - una conexión TCP que falla, o un handshake que no termina a tiempo,
//     es "no se sabe": service.ErrCaddyUnreachable;
//   - un handshake que falla (Caddy no tiene un certificado para ese SNI) o
//     un certificado que no verifica es una observación inválida, con el
//     motivo en Problem, y sin error;
//   - uno que verifica es Valid, con el emisor (su primera Organization, o
//     su CN) y el vencimiento en UTC y a milisegundos;
//   - con pinned (los DER de las hojas de Origin CA que guardó muelle), una
//     hoja que no verifica contra Roots pero es idéntica a una de pinned vale
//     si es para hostname y está vigente: se verifica con ella misma como
//     raíz.
//     Una que verifica contra Roots vale igual (el cambio entre Let's
//     Encrypt y Origin CA no la deja inválida). No se embeben las raíces de
//     Origin CA: fijar la hoja es más estricto.
func (p TLSProbe) Probe(ctx context.Context, hostname string, pinned [][]byte) (service.CertObservation, error) {
	ctx, cancel := context.WithTimeout(ctx, cmp.Or(p.Timeout, DefaultProbeTimeout))
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", p.Addr)
	if err != nil {
		return service.CertObservation{}, fmt.Errorf("%w: el TLS de Caddy (%s): %w", service.ErrCaddyUnreachable, p.Addr, err)
	}
	defer conn.Close()
	// InsecureSkipVerify: el handshake no verifica, así se ve qué sirve Caddy
	// aunque no sea válido; la verificación va abajo, con Verify.
	tc := tls.Client(conn, &tls.Config{ServerName: hostname, InsecureSkipVerify: true, MinVersion: tls.VersionTLS12})
	if err := tc.HandshakeContext(ctx); err != nil {
		if ne, ok := errors.AsType[net.Error](err); ctx.Err() != nil || (ok && ne.Timeout()) {
			return service.CertObservation{}, fmt.Errorf("%w: el handshake TLS con Caddy no terminó a tiempo: %w", service.ErrCaddyUnreachable, err)
		}
		return service.CertObservation{Problem: "Caddy no presentó un certificado para " + hostname}, nil
	}
	chain := tc.ConnectionState().PeerCertificates
	if len(chain) == 0 {
		return service.CertObservation{Problem: "Caddy no presentó un certificado para " + hostname}, nil
	}
	leaf := chain[0]
	intermediates := x509.NewCertPool()
	for _, c := range chain[1:] {
		intermediates.AddCert(c)
	}
	_, err = leaf.Verify(x509.VerifyOptions{DNSName: hostname, Roots: p.Roots, Intermediates: intermediates})
	if err != nil && slices.ContainsFunc(pinned, func(pin []byte) bool { return bytes.Equal(leaf.Raw, pin) }) {
		self := x509.NewCertPool()
		self.AddCert(leaf)
		_, err = leaf.Verify(x509.VerifyOptions{DNSName: hostname, Roots: self})
	}
	if err != nil {
		return service.CertObservation{Problem: verifyProblem(err, leaf, hostname)}, nil
	}
	issuer := leaf.Issuer.CommonName
	if len(leaf.Issuer.Organization) > 0 && leaf.Issuer.Organization[0] != "" {
		issuer = leaf.Issuer.Organization[0]
	}
	return service.CertObservation{Valid: true, Issuer: issuer, NotAfter: leaf.NotAfter.UTC().Truncate(time.Millisecond)}, nil
}

// verifyProblem dice en español por qué no verificó.
func verifyProblem(err error, leaf *x509.Certificate, hostname string) string {
	if _, ok := errors.AsType[x509.UnknownAuthorityError](err); ok {
		return "el certificado no lo firmó una CA de confianza (¿el autofirmado que sirve Caddy mientras lo emite?)"
	}
	if _, ok := errors.AsType[x509.HostnameError](err); ok {
		return "el certificado que sirve Caddy no es para " + hostname
	}
	if ce, ok := errors.AsType[x509.CertificateInvalidError](err); ok && ce.Reason == x509.Expired {
		if time.Now().After(leaf.NotAfter) {
			return "el certificado venció el " + leaf.NotAfter.UTC().Format(time.DateOnly)
		}
		return "el certificado todavía no es válido (desde el " + leaf.NotBefore.UTC().Format(time.DateOnly) + ")"
	}
	return "el certificado que sirve Caddy no es válido"
}
