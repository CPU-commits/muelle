import { describe, expect, it } from 'vitest'
import { makeLogout } from '../..'
import { InMemoryAuthRepository } from '../../repositories/__fakes__/inMemoryAuthRepository'

describe('logout', () => {
	it('cierra la sesión', async () => {
		const auth = new InMemoryAuthRepository()
		auth.session = { username: 'felipe', mfaPending: false, needsTotpSetup: false }

		await makeLogout({ auth })()

		expect(auth.session).toBeNull()
	})

	it('no falla si la sesión ya no existía', async () => {
		const auth = new InMemoryAuthRepository()

		await expect(makeLogout({ auth })()).resolves.toBeUndefined()
		expect(auth.calls.logout).toBe(1)
	})
})
