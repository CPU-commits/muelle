import { ValidationError } from '@/shared/domain'
import {
	AUDIT_PAGE_SIZE,
	type AuditPage,
	parseAuditCursor,
	toAuditPage,
} from '../entities/auditEvent'
import type { AuditDeps } from '../repositories/adminRepositories'

export interface ListAuditInput {
	/** El `nextCursor` de la página anterior; sin él, la primera. */
	before?: string | null
	limit?: number
}

/** Los eventos más nuevos primero, de a una página. */
export type ListAudit = (input?: ListAuditInput, signal?: AbortSignal) => Promise<AuditPage>

export function makeListAudit({ audit }: AuditDeps): ListAudit {
	return async ({ before = null, limit = AUDIT_PAGE_SIZE } = {}, signal) => {
		const beforeId = before === null ? null : parseAuditCursor(before)
		if (before !== null && beforeId === null) {
			throw new ValidationError(`Cursor de auditoría inválido: ${before}`, 'before')
		}
		const events = await audit.list(
			{ limit, ...(beforeId !== null ? { before: beforeId } : {}) },
			signal,
		)
		return toAuditPage(events, limit)
	}
}
