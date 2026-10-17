import type { Brand, CursorPage } from '@/shared/domain'

export type AuditEventId = Brand<number, 'AuditEventId'>

/**
 * Quién hizo algo. `name` es el username o el nombre del token, copiado al escribir el
 * evento (contrato §4.2): sigue aunque se borren el usuario o el token. Es `''` solo en un
 * evento viejo cuyo autor ya no estaba al migrar (o con un backend sin `actor_name`): ahí
 * se muestra el tipo y el id.
 */
export type AuditActor =
	| { readonly kind: 'user'; readonly id: number | null; readonly name: string }
	| { readonly kind: 'token'; readonly id: number | null; readonly name: string }
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
