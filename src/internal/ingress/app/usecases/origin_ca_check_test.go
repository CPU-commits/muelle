package usecases

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	reposqlite "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/repository/sqlite"
	outservice "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare/cftest"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

const sslToken = "cf-token-ssl-1234567890abcdef"

func testCipherFor(t *testing.T) *crypto.Cipher {
	t.Helper()
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

type permFixture struct {
	cf    *cftest.Server
	certs *reposqlite.OriginCerts
	uc    *CheckOriginCAPermission
}

func newPermFixture(t *testing.T) *permFixture {
	t.Helper()
	db := sqlitetest.New(t)
	f := &permFixture{cf: cftest.New(t), certs: reposqlite.NewOriginCerts(db, testCipherFor(t))}
	f.cf.AddToken(sslToken)
	client := cloudflare.New(f.cf.URL())
	f.uc = NewCheckOriginCAPermission(client, client, outservice.OriginKeys{}, f.certs,
		clock.NewManual(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)), slog.New(slog.NewTextHandler(io.Discard, nil)))
	return f
}

// certCalls son las llamadas a /certificates.
func (f *permFixture) certCalls() []string {
	var out []string
	for _, c := range f.cf.Calls() {
		if strings.Contains(c, "/certificates") {
			out = append(out, c)
		}
	}
	return out
}

// permError exige un *validate.Error en field, sin el token, y devuelve su mensaje.
func permError(t *testing.T, err error, field string) string {
	t.Helper()
	ve, ok := errors.AsType[*validate.Error](err)
	if !ok || ve.Field != field {
		t.Fatalf("esperaba un 422 en %s: %v", field, err)
	}
	if strings.Contains(ve.Error(), sslToken) {
		t.Fatalf("el mensaje trae el token: %q", ve.Error())
	}
	return ve.Message
}

// Con #ssl:edit en todas las zonas (la pista de GET /zones) pasa sin tocar
// /certificates: sin efectos secundarios.
func TestCheckOriginCAHint(t *testing.T) {
	f := newPermFixture(t)
	for i, name := range []string{"a.cl", "b.cl", "c.cl"} {
		id := fmt.Sprintf("z%d", i)
		f.cf.AddZone(id, name)
		f.cf.GrantSSL(sslToken, id)
	}
	if err := f.uc.Execute(context.Background(), sslToken, "cloudflare_origin_ca"); err != nil {
		t.Fatal(err)
	}
	if c := f.certCalls(); len(c) != 0 {
		t.Fatalf("llamó a /certificates: %q", c)
	}
}

// Una zona que no está activa (pending) no se revisa; un token que solo ve
// zonas que no están activas pasa.
func TestCheckOriginCAOnlyActiveZones(t *testing.T) {
	f := newPermFixture(t)
	f.cf.AddZone("z1", "nueva.cl")
	f.cf.SetZoneStatus("z1", "pending")
	if err := f.uc.Execute(context.Background(), sslToken, "cloudflare_origin_ca"); err != nil {
		t.Fatal(err)
	}
	f.cf.AddZone("z2", "activa.cl")
	f.cf.GrantSSL(sslToken, "z2")
	if err := f.uc.Execute(context.Background(), sslToken, "cloudflare_origin_ca"); err != nil {
		t.Fatal(err)
	}
	if c := f.certCalls(); len(c) != 0 {
		t.Fatalf("revisó la pending: %q", c)
	}
}

// Sin la pista (permissions no viene): la prueba definitiva, un certificado
// de 7 días para el apex de la zona, que se revoca enseguida.
func TestCheckOriginCAProbe(t *testing.T) {
	f := newPermFixture(t)
	f.cf.AddZone("z1", "auditfy.cl")
	f.cf.GrantSSL(sslToken, "z1")
	f.cf.OmitZonePermissions()
	if err := f.uc.Execute(context.Background(), sslToken, "cloudflare_origin_ca"); err != nil {
		t.Fatal(err)
	}
	issued := f.cf.Issued()
	if len(issued) != 1 || !slices.Equal(issued[0].Hostnames, []string{"auditfy.cl"}) || issued[0].Validity != 7 ||
		issued[0].RequestType != "origin-ecc" || !issued[0].Revoked {
		t.Fatalf("la prueba: %+v", issued)
	}
	if c := f.certCalls(); len(c) != 2 || c[0] != "POST /certificates" || c[1] != "DELETE /certificates/"+issued[0].ID {
		t.Fatalf("llamadas: %q", c)
	}
	if r, _ := f.certs.Retired(context.Background()); len(r) != 0 {
		t.Fatalf("no queda nada guardado: %+v", r)
	}
}

// Sin el permiso: 422 en el campo pedido, con las zonas ordenadas por nombre
// (contrato §4.1); el final depende del campo.
func TestCheckOriginCAMissing(t *testing.T) {
	f := newPermFixture(t)
	f.cf.AddZone("z1", "auditfy.cl")
	f.cf.AddZone("z2", "bien.cl")
	f.cf.GrantSSL(sslToken, "z2")
	msg := permError(t, f.uc.Execute(context.Background(), sslToken, "cloudflare_origin_ca"), "cloudflare_origin_ca")
	if want := "el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona auditfy.cl (lo necesitan los certificados de Cloudflare): agrégalo al token en Cloudflare, o deja la opción apagada"; msg != want {
		t.Fatalf("mensaje:\n%s\nesperaba\n%s", msg, want)
	}
	msg = permError(t, f.uc.Execute(context.Background(), sslToken, "cloudflare_token"), "cloudflare_token")
	if !strings.HasSuffix(msg, "en la zona auditfy.cl (lo necesitan los certificados de Cloudflare): agrégalo al token en Cloudflare, o apaga antes los certificados de Cloudflare") {
		t.Fatalf("en cloudflare_token: %s", msg)
	}
	// La prueba que falló no deja nada que revocar.
	if r, _ := f.certs.Retired(context.Background()); len(r) != 0 {
		t.Fatalf("retirados: %+v", r)
	}

	// Dos zonas, y tres o más.
	f.cf.AddZone("z3", "cero.cl")
	msg = permError(t, f.uc.Execute(context.Background(), sslToken, "cloudflare_origin_ca"), "cloudflare_origin_ca")
	if !strings.Contains(msg, "en las zonas auditfy.cl y cero.cl (") {
		t.Fatalf("dos zonas: %s", msg)
	}
	f.cf.AddZone("z4", "dos.cl")
	f.cf.AddZone("z5", "tres.cl")
	msg = permError(t, f.uc.Execute(context.Background(), sslToken, "cloudflare_origin_ca"), "cloudflare_origin_ca")
	if !strings.Contains(msg, "en las zonas auditfy.cl, cero.cl y 2 más (") {
		t.Fatalf("cuatro zonas: %s", msg)
	}
}

// El token no puede listar las zonas, o no ve ninguna: los mismos textos que
// CheckCloudflareToken, en el campo pedido.
func TestCheckOriginCAZones(t *testing.T) {
	f := newPermFixture(t)
	msg := permError(t, f.uc.Execute(context.Background(), sslToken, "cloudflare_origin_ca"), "cloudflare_origin_ca")
	if msg != "el token no ve ninguna zona: dale los permisos Zone:Read y DNS:Edit sobre las zonas de tus dominios" {
		t.Fatalf("sin zonas: %s", msg)
	}
	f.cf.AddZone("z1", "auditfy.cl")
	f.cf.FailNext("GET", "/zones", 403, 9109, "Unauthorized to access requested resource "+sslToken)
	msg = permError(t, f.uc.Execute(context.Background(), sslToken, "cloudflare_token"), "cloudflare_token")
	if msg != "el token no puede listar las zonas (Unauthorized to access requested resource ***): dale los permisos Zone:Read y DNS:Edit sobre las zonas de tus dominios" {
		t.Fatalf("no lista: %s", msg)
	}
}

// Cloudflare rechaza la prueba por otro motivo: 422 con su mensaje, cortado
// a 300 runas.
func TestCheckOriginCARejected(t *testing.T) {
	f := newPermFixture(t)
	f.cf.AddZone("z1", "auditfy.cl")
	f.cf.OmitZonePermissions()
	f.cf.FailNext("POST", "/certificates", 400, 1011, "Failed to read CSR "+strings.Repeat("x", 400))
	msg := permError(t, f.uc.Execute(context.Background(), sslToken, "cloudflare_origin_ca"), "cloudflare_origin_ca")
	if !strings.HasPrefix(msg, "Cloudflare rechazó la prueba del permiso en la zona auditfy.cl (Failed to read CSR xxx") || !strings.HasSuffix(msg, "…)") {
		t.Fatalf("rechazo: %s", msg)
	}
}

// Cloudflare no responde (5xx, 429, o al listar las zonas): 502.
func TestCheckOriginCAUnavailable(t *testing.T) {
	f := newPermFixture(t)
	f.cf.AddZone("z1", "auditfy.cl")
	f.cf.OmitZonePermissions()
	for name, fail := range map[string]func(){
		"5xx":   func() { f.cf.FailNext("POST", "/certificates", 503, 1100, "boom") },
		"429":   func() { f.cf.FailNext("POST", "/certificates", 429, 971, "Please wait") },
		"zonas": func() { f.cf.FailNext("GET", "/zones", 500, 0, "boom") },
	} {
		fail()
		err := f.uc.Execute(context.Background(), sslToken, "cloudflare_origin_ca")
		if !errors.Is(err, shared.ErrCloudflareUnavailable) || strings.Contains(err.Error(), sslToken) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Si el DELETE de la prueba falla, el certificado queda retirado en
// origin_ca_certs (sin clave) y el reconciliador lo revoca después.
func TestCheckOriginCADeleteFails(t *testing.T) {
	f := newPermFixture(t)
	f.cf.AddZone("z1", "auditfy.cl")
	f.cf.GrantSSL(sslToken, "z1")
	f.cf.OmitZonePermissions()
	f.cf.FailNext("DELETE", "/certificates/", 503, 0, "boom")
	if err := f.uc.Execute(context.Background(), sslToken, "cloudflare_origin_ca"); err != nil {
		t.Fatal(err)
	}
	issued := f.cf.Issued()
	r, err := f.certs.Retired(context.Background())
	if err != nil || len(r) != 1 || r[0].CFCertID != issued[0].ID || r[0].Hostname != "auditfy.cl" || r[0].ZoneID != "z1" ||
		r[0].State != entity.OriginRetired || r[0].ExpiresAt.Sub(r[0].CreatedAt) > 8*24*time.Hour {
		t.Fatalf("retirado: %+v %v", r, err)
	}
}

// Un plazo de 30 s sobre el ctx de la petición (acá, corto).
func TestCheckOriginCATimeout(t *testing.T) {
	f := newPermFixture(t)
	f.cf.AddZone("z1", "auditfy.cl")
	f.uc.timeout = 50 * time.Millisecond
	f.cf.Hang(5 * time.Second)
	start := time.Now()
	err := f.uc.Execute(context.Background(), sslToken, "cloudflare_origin_ca")
	if !errors.Is(err, shared.ErrCloudflareUnavailable) || time.Since(start) > 3*time.Second {
		t.Fatalf("plazo: %v en %s", err, time.Since(start))
	}
}
