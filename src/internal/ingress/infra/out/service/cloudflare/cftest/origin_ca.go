package cftest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json/v2"
	"encoding/pem"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
)

// El Origin CA falso (§13.1 del diseño de Origin CA). Lo que imita de
// Cloudflare (verificado en la documentación, el OpenAPI y los reportes;
// los códigos de un token sin el permiso no están documentados):
//   - POST /certificates con {hostnames, request_type, requested_validity,
//     csr}: 1007 con un CSR vacío, 1002 con otro request_type, 1009 con
//     otra validez, 1011 con un CSR que no se lee, 1006 con hostnames
//     repetidos, 1010 con un hostname fuera de las zonas y, sin Zone → SSL
//     and Certificates → Edit en la zona (GrantSSL), 400 con 10000;
//   - la respuesta: {id (el serial en decimal), certificate (una sola hoja
//     firmada por OriginRoot, sin intermedios), hostnames, request_type,
//     requested_validity, expires_on, csr};
//   - DELETE /certificates/{id}: {id, revoked_at}; uno desconocido 404, uno
//     ya revocado 1014, y sin el permiso en la zona, 10000;
//   - GET /zones con status (active, salvo SetZoneStatus) y permissions
//     (#zone:read y #dns_records:edit, y #ssl:edit con GrantSSL), salvo
//     OmitZonePermissions.

// originValidities son los requested_validity que acepta Cloudflare (días).
var originValidities = []int{7, 30, 90, 365, 730, 1095, 5475}

// OriginOCSP es la URL de OCSP que traen las hojas de prueba. Los de
// Cloudflare traen una (NO VERIFICADO, §1.2 del diseño); esta no resuelve,
// así un Caddy que intentara el stapling nunca llega a Cloudflare.
const OriginOCSP = "http://ocsp.cftest.invalid/origin_ca"

// IssuedCert es un certificado que emitió el Origin CA falso.
type IssuedCert struct {
	ID          string
	Hostnames   []string
	RequestType string
	Validity    int
	CertPEM     string
	// Token es el que lo pidió.
	Token   string
	Revoked bool
}

// originCA es el estado de Origin CA del servidor, bajo Server.mu.
type originCA struct {
	root      *x509.Certificate
	rootKey   *ecdsa.PrivateKey
	ssl       map[string]map[string]bool // token → zona → SSL and Certificates · Edit
	status    map[string]string          // zona → status, si no es active
	omitPerms bool
	issued    []*IssuedCert
}

func newOriginCA(t testing.TB) originCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{Organization: []string{"CloudFlare, Inc."}, OrganizationalUnit: []string{"CloudFlare Origin SSL ECC Certificate Authority"},
			CommonName: "cftest Origin ECC Root"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return originCA{root: root, rootKey: key, ssl: map[string]map[string]bool{}, status: map[string]string{}}
}

// OriginRoot es la raíz de prueba que firma los certificados de Origin CA.
func (s *Server) OriginRoot() *x509.Certificate { return s.origin.root }

// GrantSSL le da al token el permiso Zone → SSL and Certificates → Edit en
// la zona.
func (s *Server) GrantSSL(token, zoneID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.origin.ssl[token] == nil {
		s.origin.ssl[token] = map[string]bool{}
	}
	s.origin.ssl[token][zoneID] = true
}

// RevokeSSL le quita el permiso.
func (s *Server) RevokeSSL(token, zoneID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.origin.ssl[token], zoneID)
}

// SetZoneStatus deja la zona en otro status de GET /zones (pending,
// initializing, moved); "active" es el de siempre.
func (s *Server) SetZoneStatus(zoneID, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.origin.status[zoneID] = status
}

// OmitZonePermissions saca permissions de las zonas de GET /zones (el campo
// está deprecado y puede no venir).
func (s *Server) OmitZonePermissions() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.origin.omitPerms = true
}

// Issued son copias de los certificados emitidos, en orden.
func (s *Server) Issued() []IssuedCert {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]IssuedCert, 0, len(s.origin.issued))
	for _, c := range s.origin.issued {
		cp := *c
		cp.Hostnames = slices.Clone(c.Hostnames)
		out = append(out, cp)
	}
	return out
}

// Revoked dice si el certificado de ese id se emitió y se revocó.
func (s *Server) Revoked(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.origin.issued {
		if c.ID == id {
			return c.Revoked
		}
	}
	return false
}

func (s *Server) zoneStatus(id string) string {
	if st, ok := s.origin.status[id]; ok {
		return st
	}
	return "active"
}

func (s *Server) zonePermissions(token, zoneID string) []string {
	perms := []string{"#dns_records:edit", "#dns_records:read", "#zone:read"}
	if s.origin.ssl[token][zoneID] {
		perms = append(perms, "#ssl:edit", "#ssl:read")
	}
	return perms
}

// zoneOf es la zona de un hostname (un *. de un nivel cae en la suya).
func (s *Server) zoneOf(hostname string) (string, bool) {
	zones := make([]service.Zone, 0, len(s.zones))
	for id, name := range s.zones {
		zones = append(zones, service.Zone{ID: id, Name: name})
	}
	z, ok := service.MatchZone(strings.TrimPrefix(strings.ToLower(hostname), "*."), zones)
	return z.ID, ok
}

type originBody struct {
	Hostnames         []string `json:"hostnames"`
	RequestType       string   `json:"request_type"`
	RequestedValidity *int     `json:"requested_validity"`
	CSR               string   `json:"csr"`
}

func (s *Server) createOriginCert(w http.ResponseWriter, r *http.Request, token string) {
	var b originBody
	if err := json.UnmarshalRead(r.Body, &b); err != nil {
		fail(w, http.StatusBadRequest, 9207, "Request body is invalid.")
		return
	}
	validity := 5475
	if b.RequestedValidity != nil {
		validity = *b.RequestedValidity
	}
	switch {
	case strings.TrimSpace(b.CSR) == "":
		fail(w, http.StatusBadRequest, 1007, "CSR parsed as empty")
		return
	case b.RequestType != "origin-rsa" && b.RequestType != "origin-ecc" && b.RequestType != "keyless-certificate":
		fail(w, http.StatusBadRequest, 1002, "Unsupported request type")
		return
	case !slices.Contains(originValidities, validity):
		fail(w, http.StatusBadRequest, 1009, "Invalid requested validity")
		return
	case len(b.Hostnames) == 0 || len(b.Hostnames) > 100:
		fail(w, http.StatusBadRequest, 1005, "Too many or too few hostnames")
		return
	}
	block, _ := pem.Decode([]byte(b.CSR))
	var csr *x509.CertificateRequest
	if block != nil {
		csr, _ = x509.ParseCertificateRequest(block.Bytes)
	}
	if csr == nil || csr.CheckSignature() != nil {
		fail(w, http.StatusBadRequest, 1011, "Failed to read CSR")
		return
	}
	seen := map[string]bool{}
	for _, h := range b.Hostnames {
		h = strings.ToLower(h)
		if seen[h] {
			fail(w, http.StatusBadRequest, 1006, "Duplicate hostnames")
			return
		}
		seen[h] = true
		zone, found := s.zoneOf(h)
		if !found {
			fail(w, http.StatusBadRequest, 1010, "Failed to validate requested hostname "+h+": This zone is either not part of your account, or you do not have access to it. Please contact support if using a multi-user organization.")
			return
		}
		if !s.origin.ssl[token][zone] {
			fail(w, http.StatusBadRequest, 10000, "Authentication error")
			return
		}
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		fail(w, http.StatusInternalServerError, 1200, "Failed to sign certificate")
		return
	}
	now := time.Now().UTC().Truncate(time.Second)
	tmpl := &x509.Certificate{
		SerialNumber: serial.Add(serial, big.NewInt(1000)),
		Subject: pkix.Name{Organization: []string{"CloudFlare, Inc."}, OrganizationalUnit: []string{"CloudFlare Origin CA"},
			CommonName: "CloudFlare Origin Certificate"},
		DNSNames:    b.Hostnames,
		NotBefore:   now,
		NotAfter:    now.Add(time.Duration(validity) * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		OCSPServer:  []string{OriginOCSP},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, s.origin.root, csr.PublicKey, s.origin.rootKey)
	if err != nil {
		fail(w, http.StatusBadRequest, 1201, "Failed to sign certificate")
		return
	}
	c := &IssuedCert{
		ID: tmpl.SerialNumber.String(), Hostnames: slices.Clone(b.Hostnames), RequestType: b.RequestType, Validity: validity,
		CertPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), Token: token,
	}
	s.origin.issued = append(s.origin.issued, c)
	ok(w, map[string]any{
		"id": c.ID, "certificate": c.CertPEM, "hostnames": c.Hostnames, "request_type": c.RequestType,
		"requested_validity": validity, "expires_on": tmpl.NotAfter.Format("2006-01-02 15:04:05 -0700 MST"), "csr": b.CSR,
	}, nil)
}

func (s *Server) revokeOriginCert(w http.ResponseWriter, token, id string) {
	var c *IssuedCert
	for _, x := range s.origin.issued {
		if x.ID == id {
			c = x
		}
	}
	if c == nil {
		fail(w, http.StatusNotFound, 1004, "Certificate not found")
		return
	}
	if zone, _ := s.zoneOf(c.Hostnames[0]); !s.origin.ssl[token][zone] {
		fail(w, http.StatusBadRequest, 10000, "Authentication error")
		return
	}
	if c.Revoked {
		fail(w, http.StatusBadRequest, 1014, "Certificate already revoked")
		return
	}
	c.Revoked = true
	ok(w, map[string]string{"id": id, "revoked_at": time.Now().UTC().Format(time.RFC3339Nano)}, nil)
}
