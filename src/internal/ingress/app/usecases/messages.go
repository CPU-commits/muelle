package usecases

import (
	"errors"
	"fmt"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
)

// Los mensajes de error de un dominio (last_error), en español, que ve el
// panel. Nunca traen el token.
const (
	msgNoPublicIP     = "configura la IP pública en Ajustes"
	msgNoToken        = "agrega el token de Cloudflare en Ajustes"
	msgCloudflareDown = "Cloudflare no responde; reintenta en unos minutos"
)

// msgRecordExists: hay un registro de ese nombre que muelle no creó (→ Adoptar).
func msgRecordExists(typ, host string) string {
	return fmt.Sprintf("ya hay un registro %s de %s en Cloudflare que muelle no creó; adóptalo, o bórralo en Cloudflare y reintenta", typ, host)
}

// msgMultiple: adoptar necesita un solo registro de ese tipo.
func msgMultiple(typ, host string) string {
	return fmt.Sprintf("hay más de un registro %s de %s en Cloudflare; deja uno solo y reintenta", typ, host)
}

// msgForeignAAAA: muelle maneja el A, pero ya hay un AAAA que no creó.
func msgForeignAAAA(host string) string {
	return fmt.Sprintf("ya hay un registro AAAA de %s en Cloudflare que muelle no creó; bórralo en Cloudflare, o quita la IPv6 pública en Ajustes, y reintenta", host)
}

// msgRecordGone: el registro propio ya no está en Cloudflare (D29).
// aaaaKept: el AAAA propio no se pudo borrar y quedó ajeno.
func msgRecordGone(typ, host string, aaaaKept bool) string {
	msg := fmt.Sprintf("el registro %s de %s ya no está en Cloudflare; reintenta para crearlo de nuevo", typ, host)
	if aaaaKept {
		msg += "; su AAAA no se pudo borrar y quedó en Cloudflare"
	}
	return msg
}

// cloudflareMsg es el mensaje de un error de Cloudflare: lo que dijo
// Cloudflare si rechazó el pedido, o que no responde.
func cloudflareMsg(err error) string {
	if de, ok := errors.AsType[*service.DNSError](err); ok {
		return de.Error()
	}
	return msgCloudflareDown
}
