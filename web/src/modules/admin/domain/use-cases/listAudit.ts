import {
	AUDIT_PAGE_SIZE,
	type AuditEventId,
	type AuditPage,
	toAuditPage,
} from '../entities/auditEvent'
import type { AuditDeps } from '../repositories/adminRepositories'

export interface ListAuditInput {
	/** El `nextBefore` de la página anterior; sin él, la primera. */
	before?: AuditEventId | null
	limit?: number
}

/** Los eventos más nuevos primero, de a una página. */
export type ListAudit = (input?: ListAuditInput, signal?: AbortSignal) => Promise<AuditPage>

export function makeListAudit({ audit }: AuditDeps): ListAudit {
	return async ({ before = null, limit = AUDIT_PAGE_SIZE } = {}, signal) => {
		const events = await audit.list({ limit, ...(before !== null ? { before } : {}) }, signal)
		return toAuditPage(events, limit)
	}
}
