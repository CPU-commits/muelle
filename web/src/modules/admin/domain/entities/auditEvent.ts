import type { Brand, CursorPage } from '@/shared/domain'

export type AuditEventId = Brand<number, 'AuditEventId'>

/**
 * Quién hizo algo. El backend solo guarda el tipo y el id: el panel no puede saber el
 * nombre de un usuario por su id, y el de un token solo mientras el token exista.
 */
export type AuditActor =
	| { readonly kind: 'user'; readonly id: number | null }
	| { readonly kind: 'token'; readonly id: number | null }
	| { readonly kind: 'system' }
	| { readonly kind: 'other'; readonly type: string }

/**
 * Un evento de `GET /audit`. Los `details` del backend no llegan acá a propósito: nunca
 * traen valores secretos, pero la tabla solo muestra quién, qué, sobre qué y cuándo.
 */
export interface AuditEvent {
	readonly id: AuditEventId
	readonly actor: AuditActor
	/** `project.create`, `env.patch`, `auth.login`… */
	readonly action: string
	/** `app:auditfy/staging/api`, `user:felipe`, `token:5`… */
	readonly target: string
	readonly at: Date
}

/**
 * Una página, de la más nueva a la más vieja (el `CursorPage` de shared). `nextCursor` es
 * el id del último evento, como texto: es el `before` de la siguiente.
 */
export type AuditPage = CursorPage<AuditEvent>

export const AUDIT_PAGE_SIZE = 25

/**
 * El backend pagina por id (`before` = el id más chico ya visto) y no dice si hay más:
 * una página llena puede tener otra detrás; una incompleta es la última.
 */
export function toAuditPage(events: readonly AuditEvent[], limit: number): AuditPage {
	const last = events.at(-1)
	return { items: events, nextCursor: events.length >= limit && last ? String(last.id) : null }
}

/** El cursor de vuelta a id; `null` si no es uno (un cursor que no salió de `toAuditPage`). */
export function parseAuditCursor(cursor: string): AuditEventId | null {
	return /^[1-9]\d*$/.test(cursor) ? (Number(cursor) as AuditEventId) : null
}
