import {
	AmbiguousSessionError,
	BadOriginError,
	ForbiddenError,
	PanelMovedError,
	RateLimitedError,
	ReadOnlyTokenError,
	UnavailableError,
} from '@/shared/domain'
import { m } from '@/shared/ui/i18n/paraglide/messages'

// El texto para un error que la pantalla no conoce por su clase: las de shared que pueden
// llegar de cualquier endpoint. Cada módulo mira primero sus errores propios y cae acá.
export function describeError(error: unknown): string {
	if (error instanceof PanelMovedError) return m.common_panel_moved({ origin: error.origin })
	if (error instanceof BadOriginError) return m.common_bad_origin()
	if (error instanceof AmbiguousSessionError) return m.common_ambiguous_session()
	if (error instanceof UnavailableError) {
		// "Revisa la conexión" solo si de verdad no hubo respuesta.
		if (error.reason === 'server') return m.common_server_error()
		if (error.reason === 'timeout') return m.common_timeout()
		return m.common_unavailable()
	}
	if (error instanceof RateLimitedError) return m.common_rate_limited()
	if (error instanceof ReadOnlyTokenError) return m.common_read_only_token()
	if (error instanceof ForbiddenError) return m.common_forbidden()
	return m.common_error_generic()
}
