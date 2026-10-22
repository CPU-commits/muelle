package cftest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json/v2"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

// testCSR es un CSR ECDSA P-256 en PEM para los hostnames.
func testCSR(t *testing.T, hostnames ...string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: hostnames[0]}, DNSNames: hostnames,
	}, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
}

func certBody(t *testing.T, csr string, validity int, hostnames ...string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"hostnames": hostnames, "request_type": "origin-ecc", "requested_validity": validity, "csr": csr})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

type certResult struct {
	ID                string   `json:"id"`
	Certificate       string   `json:"certificate"`
	Hostnames         []string `json:"hostnames"`
	RequestType       string   `json:"request_type"`
	RequestedValidity int      `json:"requested_validity"`
	ExpiresOn         string   `json:"expires_on"`
	CSR               string   `json:"csr"`
}

func originServer(t *testing.T) *Server {
	t.Helper()
	s := New(t)
	s.AddToken("ssl")
	s.AddToken("dns")
	s.AddZone("z1", "auditfy.cl")
	s.AddZone("z2", "otro.cl")
	s.GrantSSL("ssl", "z1")
	return s
}

// POST /certificates firma el CSR con la CA de prueba (OriginRoot): una sola
// hoja para los hostnames pedidos, con la clave del CSR y la validez pedida.
func TestCreateOriginCert(t *testing.T) {
	s := originServer(t)
	csr := testCSR(t, "api.auditfy.cl")
	r := call(t, s, "ssl", "POST", "/certificates", certBody(t, csr, 90, "api.auditfy.cl"))
	if r.status != 200 || !r.env.Success {
		t.Fatalf("POST: %d %s", r.status, r.body)
	}
	var res certResult
	if err := json.Unmarshal(r.env.Result, &res); err != nil {
		t.Fatal(err)
	}
	if res.ID == "" || res.RequestType != "origin-ecc" || res.RequestedValidity != 90 || res.CSR != csr ||
		len(res.Hostnames) != 1 || res.Hostnames[0] != "api.auditfy.cl" || res.ExpiresOn == "" {
		t.Fatalf("resultado: %+v", res)
	}
	block, _ := pem.Decode([]byte(res.Certificate))
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("certificado: %q", res.Certificate)
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if leaf.SerialNumber.String() != res.ID {
		t.Errorf("el id es el serial en decimal: %s %s", res.ID, leaf.SerialNumber)
	}
	pool := x509.NewCertPool()
	pool.AddCert(s.OriginRoot())
	if _, err := leaf.Verify(x509.VerifyOptions{DNSName: "api.auditfy.cl", Roots: pool}); err != nil {
		t.Errorf("no verifica contra OriginRoot: %v", err)
	}
	if d := leaf.NotAfter.Sub(leaf.NotBefore); d < 89*24*time.Hour || d > 91*24*time.Hour {
		t.Errorf("validez: %s", d)
	}
	if strings.Contains(res.Certificate, "PRIVATE") {
		t.Error("el resultado no trae una clave")
	}
	issued := s.Issued()
	if len(issued) != 1 || issued[0].ID != res.ID || issued[0].Hostnames[0] != "api.auditfy.cl" || issued[0].Validity != 90 ||
		issued[0].RequestType != "origin-ecc" || issued[0].Token != "ssl" || s.Revoked(res.ID) {
		t.Fatalf("Issued: %+v", issued)
	}
}

// Sin el permiso en la zona del hostname: 400 con 10000 (lo que se vio en
// los reportes, §1.1 del diseño). Un hostname fuera de las zonas: 1010.
func TestCreateOriginCertRejects(t *testing.T) {
	s := originServer(t)
	csr := testCSR(t, "api.auditfy.cl")
	cases := []struct {
		name, token, body string
		status, code      int
	}{
		{"sin el permiso", "dns", certBody(t, csr, 90, "api.auditfy.cl"), 400, 10000},
		{"permiso en otra zona", "ssl", certBody(t, testCSR(t, "www.otro.cl"), 90, "www.otro.cl"), 400, 10000},
		{"fuera de toda zona", "ssl", certBody(t, testCSR(t, "x.ejemplo.com"), 90, "x.ejemplo.com"), 400, 1010},
		{"CSR vacío", "ssl", certBody(t, "", 90, "api.auditfy.cl"), 400, 1007},
		{"CSR ilegible", "ssl", certBody(t, "-----BEGIN CERTIFICATE REQUEST-----\nnoesbase64\n-----END CERTIFICATE REQUEST-----\n", 90, "api.auditfy.cl"), 400, 1011},
		{"validez inválida", "ssl", certBody(t, csr, 91, "api.auditfy.cl"), 400, 1009},
		{"tipo inválido", "ssl", strings.Replace(certBody(t, csr, 90, "api.auditfy.cl"), "origin-ecc", "origin-dsa", 1), 400, 1002},
		{"hostnames repetidos", "ssl", certBody(t, csr, 90, "api.auditfy.cl", "api.auditfy.cl"), 400, 1006},
		{"token desconocido", "otro", certBody(t, csr, 90, "api.auditfy.cl"), 401, 1000},
	}
	for _, c := range cases {
		r := call(t, s, c.token, "POST", "/certificates", c.body)
		if r.status != c.status || r.env.Success || len(r.env.Errors) != 1 || r.env.Errors[0].Code != c.code {
			t.Errorf("%s: %d %s", c.name, r.status, r.body)
		}
	}
	if len(s.Issued()) != 0 {
		t.Fatalf("se emitió algo: %+v", s.Issued())
	}
	// FailNext imita lo que no se ve de otra forma (1010 de una zona pending, 403).
	s.FailNext("POST", "/certificates", 403, 9109, "Unauthorized to access requested resource")
	if r := call(t, s, "ssl", "POST", "/certificates", certBody(t, csr, 90, "api.auditfy.cl")); r.status != 403 || r.env.Errors[0].Code != 9109 {
		t.Fatalf("FailNext: %d %s", r.status, r.body)
	}
}

// DELETE /certificates/{id} revoca (revoked_at); uno desconocido es 404, y
// uno ya revocado 1014. Revocar también pide el permiso en la zona.
func TestRevokeOriginCert(t *testing.T) {
	s := originServer(t)
	r := call(t, s, "ssl", "POST", "/certificates", certBody(t, testCSR(t, "api.auditfy.cl"), 7, "api.auditfy.cl"))
	var res certResult
	if err := json.Unmarshal(r.env.Result, &res); err != nil || res.ID == "" {
		t.Fatalf("POST: %s %v", r.body, err)
	}
	if r := call(t, s, "dns", "DELETE", "/certificates/"+res.ID, ""); r.status != 400 || r.env.Errors[0].Code != 10000 || s.Revoked(res.ID) {
		t.Fatalf("sin el permiso: %d %s", r.status, r.body)
	}
	r = call(t, s, "ssl", "DELETE", "/certificates/"+res.ID, "")
	if r.status != 200 || !r.env.Success || !strings.Contains(string(r.env.Result), `"id":"`+res.ID+`"`) || !strings.Contains(string(r.env.Result), `"revoked_at":"`) {
		t.Fatalf("DELETE: %d %s", r.status, r.body)
	}
	if !s.Revoked(res.ID) || !s.Issued()[0].Revoked {
		t.Fatal("Revoked")
	}
	if r := call(t, s, "ssl", "DELETE", "/certificates/"+res.ID, ""); r.status != 400 || r.env.Errors[0].Code != 1014 {
		t.Fatalf("ya revocado: %d %s", r.status, r.body)
	}
	if r := call(t, s, "ssl", "DELETE", "/certificates/123", ""); r.status != 404 || r.env.Success {
		t.Fatalf("desconocido: %d %s", r.status, r.body)
	}
	if s.Revoked("123") {
		t.Fatal("uno desconocido no está revocado")
	}
}

type zoneJSON struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	Permissions []string `json:"permissions"`
}

func zones(t *testing.T, s *Server, token string) map[string]zoneJSON {
	t.Helper()
	r := call(t, s, token, "GET", "/zones", "")
	var list []zoneJSON
	if err := json.Unmarshal(r.env.Result, &list); err != nil {
		t.Fatalf("%s: %v", r.body, err)
	}
	out := map[string]zoneJSON{}
	for _, z := range list {
		out[z.ID] = z
	}
	return out
}

// GET /zones trae status y permissions de cada zona: #ssl:edit según
// GrantSSL, para ese token. OmitZonePermissions saca el campo (está
// deprecado y puede no venir).
func TestZonesPermissions(t *testing.T) {
	s := originServer(t)
	s.SetZoneStatus("z2", "pending")
	has := func(z zoneJSON, p string) bool {
		for _, x := range z.Permissions {
			if x == p {
				return true
			}
		}
		return false
	}
	ssl := zones(t, s, "ssl")
	if !has(ssl["z1"], "#ssl:edit") || has(ssl["z2"], "#ssl:edit") || !has(ssl["z1"], "#dns_records:edit") {
		t.Fatalf("ssl: %+v", ssl)
	}
	if ssl["z1"].Status != "active" || ssl["z2"].Status != "pending" {
		t.Fatalf("status: %+v", ssl)
	}
	if dns := zones(t, s, "dns"); has(dns["z1"], "#ssl:edit") || !has(dns["z1"], "#zone:read") {
		t.Fatalf("dns: %+v", dns)
	}
	s.OmitZonePermissions()
	r := call(t, s, "ssl", "GET", "/zones", "")
	if strings.Contains(string(r.env.Result), "permissions") {
		t.Fatalf("sin el campo: %s", r.body)
	}
}

// SetClock cambia la hora con que el Origin CA falso firma: los tests con un
// reloj manual emiten certificados que valen en esa hora.
func TestOriginCertClock(t *testing.T) {
	s := originServer(t)
	at := time.Date(2027, 3, 1, 12, 30, 0, 0, time.UTC)
	s.SetClock(func() time.Time { return at })
	r := call(t, s, "ssl", "POST", "/certificates", certBody(t, testCSR(t, "api.auditfy.cl"), 90, "api.auditfy.cl"))
	var res certResult
	if err := json.Unmarshal(r.env.Result, &res); err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode([]byte(res.Certificate))
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if !leaf.NotBefore.Equal(at) || !leaf.NotAfter.Equal(at.Add(90*24*time.Hour)) {
		t.Fatalf("de %s a %s", leaf.NotBefore, leaf.NotAfter)
	}
}
