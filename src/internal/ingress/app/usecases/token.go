package usecases

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// tokenTimeout es el plazo de la verificación, sobre el ctx de la petición.
const tokenTimeout = 30 * time.Second

// fieldToken es el campo de PATCH /settings en que van los 422 del token.
const fieldToken = "cloudflare_token"

// msgTokenPermissions es lo que le falta a un token que no lista zonas.
const msgTokenPermissions = "dale los permisos Zone:Read y DNS:Edit sobre las zonas de tus dominios"

// CheckCloudflareToken verifica un token nuevo antes de que admin lo guarde
// (PATCH /settings, contrato §3.5).
type CheckCloudflareToken struct{ dns service.DNS }

func NewCheckCloudflareToken(dns service.DNS) *CheckCloudflareToken {
	return &CheckCloudflareToken{dns: dns}
}

// Execute verifica el token (GET /user/tokens/verify) y cuenta las zonas que
// ve (GET /zones, todas las páginas), con un plazo de 30 s sobre el ctx de
// la petición: si el cliente se va, no se guarda nada. Devuelve cuántas
// zonas ve. Errores:
//   - Cloudflare no responde (sin red, 5xx, 429), en cualquiera de las dos
//     llamadas: un error que envuelve entity.ErrCloudflareUnavailable (502);
//   - el token rechazado (verify lo rechaza, o no está activo, y tampoco
//     lista zonas), o uno que no puede listar zonas o no ve ninguna: un
//     *validate.Error en cloudflare_token (422). Un token de una cuenta, que
//     verify rechaza pero que lista zonas, se acepta.
//
// Ningún error trae el token (el cliente de Cloudflare lo oculta).
func (u *CheckCloudflareToken) Execute(ctx context.Context, token string) (*int, error) {
	ctx, cancel := context.WithTimeout(ctx, tokenTimeout)
	defer cancel()
	verifyErr := u.dns.VerifyToken(ctx, token)
	if verifyErr != nil && !isRejection(verifyErr) {
		return nil, unavailableErr("verificando el token de Cloudflare", verifyErr)
	}
	zones, zonesErr := u.dns.Zones(ctx, token)
	if zonesErr != nil && !isRejection(zonesErr) {
		return nil, unavailableErr("listando las zonas de Cloudflare", zonesErr)
	}
	if verifyErr != nil && (zonesErr != nil || len(zones) == 0) {
		return nil, validate.Errorf(fieldToken, "Cloudflare rechazó el token (%s): revisa que esté bien copiado y activo",
			rejectionMsg(verifyErr))
	}
	if zonesErr != nil {
		return nil, validate.Errorf(fieldToken, "el token no puede listar las zonas (%s): %s", rejectionMsg(zonesErr), msgTokenPermissions)
	}
	if len(zones) == 0 {
		return nil, validate.Errorf(fieldToken, "el token no ve ninguna zona: %s", msgTokenPermissions)
	}
	n := len(zones)
	return &n, nil
}

// isRejection: Cloudflare respondió y rechazó el pedido (un 4xx que no es
// 429, también un 404, o success: false).
func isRejection(err error) bool {
	_, ok := errors.AsType[*service.DNSError](err)
	return ok && !errors.Is(err, shared.ErrCloudflareUnavailable)
}

// rejectionMsg es lo que dijo Cloudflare al rechazar el pedido (sin el token).
func rejectionMsg(err error) string {
	if de, ok := errors.AsType[*service.DNSError](err); ok {
		return de.Message
	}
	return "sin detalle"
}

// unavailableErr es err con lo que se hacía, envolviendo siempre
// entity.ErrCloudflareUnavailable (502): un error que no es un rechazo (un
// pedido que no se pudo armar, un ctx vencido) tampoco es culpa del token.
func unavailableErr(doing string, err error) error {
	if errors.Is(err, shared.ErrCloudflareUnavailable) {
		return fmt.Errorf("ingress: %s: %w", doing, err)
	}
	return fmt.Errorf("ingress: %s: %w: %w", doing, shared.ErrCloudflareUnavailable, err)
}
