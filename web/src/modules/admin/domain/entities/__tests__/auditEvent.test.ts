import { describe, expect, it } from 'vitest'
import { toAuditPage } from '../..'
import { fakeAuditEvents } from '../../repositories/__fakes__/inMemoryAdminRepository'

describe('auditEvent', () => {
	it('una página llena puede tener otra detrás: sigue desde el id más chico', () => {
		const events = fakeAuditEvents(5)

		expect(toAuditPage(events, 5).nextCursor).toBe('1')
	})

	it('una página incompleta es la última', () => {
		expect(toAuditPage(fakeAuditEvents(3), 5).nextCursor).toBeNull()
		expect(toAuditPage([], 5)).toEqual({ items: [], nextCursor: null })
	})
})
