import type { OriginCaErrorCode, OriginCaStatus } from '../value-objects/domainStatus'

// El certificado de Cloudflare Origin CA de un hostname (contrato §3.7): el de un dominio y
// el del panel (`panel_certificate` de `GET /proxy`). Lo emite y lo renueva muelle, en
// segundo plano; solo Cloudflare confía en él (los navegadores no).

export interface OriginCaState {
	readonly status: OriginCaStatus
	/**
	 * El motivo de la última falla, solo con `failed` y con `active` (una renovación que
	 * falló). null también con un código que el panel no conoce: va como una falla cualquiera,
	 * con su `error`.
	 */
	readonly errorCode: OriginCaErrorCode | null
	/** El mensaje de esa falla, en español, para mostrar tal cual. */
	readonly error: string | null
	/** Cuándo se reintenta; en el pasado justo después de un Reintentar. */
	readonly retryAt: Date | null
	/** El vencimiento del que guardó muelle, se sirva o no; con `active`, nunca null. */
	readonly expiresAt: Date | null
}
