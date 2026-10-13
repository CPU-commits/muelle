import { describe, expect, it } from 'vitest'
import { ValidationError } from '@/shared/domain'
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
		const second = await list({ limit: 3, before: first.nextCursor })
		const third = await list({ limit: 3, before: second.nextCursor })

		expect(first.items.map((e) => e.id)).toEqual([7, 6, 5])
		expect(second.items.map((e) => e.id)).toEqual([4, 3, 2])
		expect(third.items.map((e) => e.id)).toEqual([1])
		expect(third.nextCursor).toBeNull()
		expect(audit.queries).toEqual([
			{ limit: 3 },
			{ limit: 3, before: 5 },
			{ limit: 3, before: 2 },
		])
	})

	it('un cursor que no salió de una página no se manda', async () => {
		const audit = new InMemoryAuditRepository(fakeAuditEvents(3))

		await expect(makeListAudit({ audit })({ before: 'abc' })).rejects.toBeInstanceOf(
			ValidationError,
		)
		expect(audit.queries).toEqual([])
	})

	it('sin parámetros pide la primera página del tamaño por defecto', async () => {
		const audit = new InMemoryAuditRepository([])

		await expect(makeListAudit({ audit })()).resolves.toEqual({ items: [], nextCursor: null })
		expect(audit.queries).toEqual([{ limit: AUDIT_PAGE_SIZE }])
	})
})
