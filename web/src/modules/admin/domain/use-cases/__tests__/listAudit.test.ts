import { describe, expect, it } from 'vitest'
import { AUDIT_PAGE_SIZE, makeListAudit } from '../..'
import {
	fakeAuditEvents,
	InMemoryAuditRepository,
} from '../../repositories/__fakes__/inMemoryAdminRepository'

describe('listAudit', () => {
	it('pagina con before hasta la última página', async () => {
		const audit = new InMemoryAuditRepository(fakeAuditEvents(7))
		const list = makeListAudit({ audit })

		const first = await list({ limit: 3 })
		const second = await list({ limit: 3, before: first.nextBefore })
		const third = await list({ limit: 3, before: second.nextBefore })

		expect(first.events.map((e) => e.id)).toEqual([7, 6, 5])
		expect(second.events.map((e) => e.id)).toEqual([4, 3, 2])
		expect(third.events.map((e) => e.id)).toEqual([1])
		expect(third.nextBefore).toBeNull()
		expect(audit.queries).toEqual([
			{ limit: 3 },
			{ limit: 3, before: 5 },
			{ limit: 3, before: 2 },
		])
	})

	it('sin parámetros pide la primera página del tamaño por defecto', async () => {
		const audit = new InMemoryAuditRepository([])

		await expect(makeListAudit({ audit })()).resolves.toEqual({ events: [], nextBefore: null })
		expect(audit.queries).toEqual([{ limit: AUDIT_PAGE_SIZE }])
	})
})
