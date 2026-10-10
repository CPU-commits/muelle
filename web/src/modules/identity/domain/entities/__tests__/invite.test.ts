import { describe, expect, it } from 'vitest'
import { inviteHoursLeft } from '../..'

const NOW = new Date('2026-10-10T12:00:00.000Z')

describe('invite', () => {
	it('redondea hacia arriba las horas que le quedan', () => {
		const invite = { url: 'x', expiresAt: new Date('2026-10-11T12:00:00.000Z') }
		expect(inviteHoursLeft(invite, NOW)).toBe(24)
		expect(inviteHoursLeft(invite, new Date('2026-10-11T11:30:00.000Z'))).toBe(1)
	})

	it('una invitación vencida tiene 0 horas', () => {
		const invite = { url: 'x', expiresAt: new Date('2026-10-10T11:00:00.000Z') }
		expect(inviteHoursLeft(invite, NOW)).toBe(0)
	})
})
