import { describe, expect, it } from 'vitest'
import { inviteHoursLeft, makeCreateInvite } from '../..'
import { InMemoryIdentityRepository } from '../../repositories/__fakes__/inMemoryIdentityRepository'

describe('createInvite', () => {
	it('devuelve la URL una vez, con 24 h por delante', async () => {
		const now = new Date('2026-10-10T12:00:00.000Z')
		const repo = new InMemoryIdentityRepository({ now: () => now })
		const createInvite = makeCreateInvite({ users: repo.asUsers() })

		const invite = await createInvite()

		expect(invite.url).toMatch(/\/invite\/inv-1$/)
		expect(inviteHoursLeft(invite, now)).toBe(24)
		await expect(createInvite()).resolves.toMatchObject({
			url: expect.stringMatching(/inv-2$/),
		})
	})
})
