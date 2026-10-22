package usecases

import (
	"cmp"
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

const (
	// FieldOriginCA es el campo de PATCH /settings de los certificados de
	// Cloudflare (contrato §4.1).
	FieldOriginCA = "cloudflare_origin_ca"
	// sslEditHint es el permiso de una zona de GET /zones que dice que el
	// token tiene Zone → SSL and Certificates → Edit (deprecado, una pista).
	sslEditHint = "#ssl:edit"
	// probeValidityDays es la validez del certificado de prueba: lo más corto
	// que acepta Cloudflare.
	probeValidityDays = 7
	// permissionWorkers es cuántas zonas se revisan a la vez.
	permissionWorkers = 4
)

// CheckOriginCAPermission comprueba que el token tenga Zone → SSL and
// Certificates → Edit en las zonas activas que ve, antes de que admin
// encienda los certificados de Cloudflare o cambie el token con la opción
// encendida (PATCH /settings, contrato §3.5 y §4.1; §3.3 del diseño de
// Origin CA).
type CheckOriginCAPermission struct {
	dns     service.DNS
	ca      service.OriginCA
	keys    service.OriginKeys
	certs   repository.OriginCerts
	clock   clock.Clock
	log     *slog.Logger
	timeout time.Duration
}

func NewCheckOriginCAPermission(dns service.DNS, ca service.OriginCA, keys service.OriginKeys, certs repository.OriginCerts,
	clk clock.Clock, log *slog.Logger) *CheckOriginCAPermission {
	return &CheckOriginCAPermission{dns: dns, ca: ca, keys: keys, certs: certs, clock: clk, log: log, timeout: tokenTimeout}
}

// zoneResult es lo que dio la revisión de una zona: nada (tiene el
// permiso), que le falta (missing), que Cloudflare rechazó la prueba por
// otro motivo (rejected, su mensaje) o un error (Cloudflare no responde).
type zoneResult struct {
	zone     service.Zone
	missing  bool
	rejected string
	err      error
}

// Execute, con un plazo de 30 s sobre el ctx de la petición (si el cliente
// se va, no se guarda nada):
//  1. lista las zonas del token (todas las páginas). Un rechazo, o ninguna
//     zona, es un 422 en field con los textos de CheckCloudflareToken;
//  2. revisa cada zona activa (status active; una pending no la usa
//     Cloudflare de proxy), de a 4: con #ssl:edit en permissions pasa sin
//     efectos; si no, la prueba definitiva: un certificado de 7 días para el
//     apex, que se revoca enseguida (si el DELETE falla, queda retirado en
//     origin_ca_certs y lo revoca el reconciliador);
//  3. el resultado: Cloudflare que no responde (o el plazo que vence antes
//     de revisarlas todas) es un error que envuelve
//     entity.ErrCloudflareUnavailable (502; con uno se cortan las demás
//     revisiones, que un 429 bloquea el token); las zonas sin el permiso,
//     ordenadas, un 422 en field; y una prueba que Cloudflare rechazó por
//     otro motivo, un 422 con su mensaje (la primera zona, por nombre).
//
// field es cloudflare_origin_ca o cloudflare_token (cambia el final del
// mensaje). Ningún error trae el token, y el mensaje de Cloudflare se corta
// a 300 runas.
func (u *CheckOriginCAPermission) Execute(ctx context.Context, token, field string) error {
	ctx, cancel := context.WithTimeout(ctx, u.timeout)
	defer cancel()
	zones, err := u.dns.Zones(ctx, token)
	if err != nil {
		if isRejection(err) {
			return validate.Errorf(field, "el token no puede listar las zonas (%s): %s", noToken(service.CloudflareMessage(err), token),
				msgTokenPermissions)
		}
		return unavailableErr("listando las zonas de Cloudflare", err)
	}
	if len(zones) == 0 {
		return validate.Errorf(field, "el token no ve ninguna zona: %s", msgTokenPermissions)
	}
	var check []service.Zone
	for _, z := range zones {
		if z.Status == service.ZoneActive && !slices.Contains(z.Permissions, sslEditHint) {
			check = append(check, z)
		}
	}
	slices.SortFunc(check, func(a, b service.Zone) int { return cmp.Compare(a.Name, b.Name) })

	results := make([]zoneResult, len(check))
	work := make(chan int)
	var wg sync.WaitGroup
	for range min(permissionWorkers, len(check)) {
		wg.Go(func() {
			for i := range work {
				results[i] = u.probe(ctx, token, check[i])
				if results[i].err != nil {
					cancel() // un 429 bloquea el token: no se sigue
				}
			}
		})
	}
	dispatched := 0
	for i := range check {
		if ctx.Err() != nil {
			break
		}
		work <- i
		dispatched++
	}
	close(work)
	wg.Wait()

	var missing []string
	rejected := ""
	for _, r := range results[:dispatched] {
		switch {
		case r.err != nil:
			return r.err
		case r.rejected != "" && rejected == "":
			rejected = r.rejected
		case r.missing:
			missing = append(missing, r.zone.Name)
		}
	}
	if dispatched < len(check) {
		return unavailableErr("comprobando el permiso de los certificados de Cloudflare", ctx.Err())
	}
	if len(missing) > 0 {
		end := "o deja la opción apagada"
		if field != FieldOriginCA {
			end = "o apaga antes los certificados de Cloudflare"
		}
		return validate.Errorf(field, "el token no tiene el permiso Zone → SSL and Certificates → Edit %s (lo necesitan los certificados de Cloudflare): agrégalo al token en Cloudflare, %s",
			service.ZonesPhrase(missing), end)
	}
	if rejected != "" {
		return validate.Errorf(field, "%s", rejected)
	}
	return nil
}

// probe es la prueba definitiva en z: un certificado de 7 días para su
// apex, revocado enseguida. Clasifica el POST con ClassifyOriginError.
func (u *CheckOriginCAPermission) probe(ctx context.Context, token string, z service.Zone) zoneResult {
	res := zoneResult{zone: z}
	_, csr, err := u.keys.Generate(z.Name)
	if err != nil {
		res.err = fmt.Errorf("ingress: la prueba del permiso en %s: %w", z.Name, err)
		return res
	}
	issued, err := u.ca.CreateOriginCert(ctx, token, z.Name, csr, probeValidityDays)
	if err != nil {
		switch service.ClassifyOriginError(err) {
		case entity.OriginPermission:
			res.missing = true
		case entity.OriginRejected:
			res.rejected = fmt.Sprintf("Cloudflare rechazó la prueba del permiso en la zona %s (%s)", z.Name,
				noToken(service.CloudflareMessage(err), token))
		default:
			res.err = unavailableErr("comprobando el permiso en la zona "+z.Name, err)
		}
		return res
	}
	err = u.ca.RevokeOriginCert(ctx, token, issued.ID)
	if de, ok := errors.AsType[*service.DNSError](err); err == nil || errors.Is(err, service.ErrDNSNotFound) || (ok && de.Code == 1014) {
		return res
	}
	// Nunca lo sirvió nadie: revocarlo después no corta nada.
	now := u.clock.Now()
	c := entity.OriginCert{Hostname: z.Name, ZoneID: z.ID, CFCertID: issued.ID, CertPEM: issued.CertPEM,
		ExpiresAt: notAfter(issued.CertPEM, now.Add(probeValidityDays*24*time.Hour)), CreatedAt: now}
	if serr := u.certs.InsertRetired(context.WithoutCancel(ctx), &c, now); serr != nil {
		u.log.Error("ingress: guardando la prueba del permiso para revocarla después", "zone", z.Name, "cf_cert_id", issued.ID, "error", serr)
		return res
	}
	u.log.Warn("ingress: no se pudo revocar la prueba del permiso; se revoca después", "zone", z.Name, "cf_cert_id", issued.ID, "error", err)
	return res
}

// notAfter es el vencimiento de la hoja del PEM, o def si no se lee.
func notAfter(certPEM string, def time.Time) time.Time {
	if block, _ := pem.Decode([]byte(certPEM)); block != nil {
		if c, err := x509.ParseCertificate(block.Bytes); err == nil {
			return c.NotAfter.UTC()
		}
	}
	return def
}

// noToken saca el token de un mensaje de Cloudflare (el cliente ya lo hace;
// es una segunda red).
func noToken(msg, token string) string {
	if token == "" {
		return msg
	}
	return strings.ReplaceAll(msg, token, "***")
}
