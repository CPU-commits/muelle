import { describe, expect, it, vi } from 'vitest'
import { AmbiguousSessionError, UnavailableError } from '@/shared/domain'
import { makeLogout, MfaRequired } from '../..'
import { InMemoryAuthRepository } from '../../repositories/__fakes__/inMemoryAuthRepository'

describe('logout', () => {
	it('cierra la sesión', async () => {
		const auth = new InMemoryAuthRepository()
		auth.session = { username: 'felipe', mfaPending: false, needsTotpSetup: false }

		await makeLogout({ auth })()

		expect(auth.session).toBeNull()
	})

	it('si la sesión ya no existía (401), cuenta como cerrada', async () => {
		const auth = new InMemoryAuthRepository()

		await expect(makeLogout({ auth })()).resolves.toBeUndefined()
		expect(auth.calls.logout).toBe(1)
	})

	it('una sesión que esperaba el código (401 mfa_required) también cuenta como cerrada', async () => {
		const auth = new InMemoryAuthRepository()
		auth.logout = vi.fn().mockRejectedValue(new MfaRequired())

		await expect(makeLogout({ auth })()).resolves.toBeUndefined()
	})

	it('cualquier otra falla se propaga: la sesión sigue abierta y hay que decirlo', async () => {
		const down = new InMemoryAuthRepository()
		down.logout = vi.fn().mockRejectedValue(new UnavailableError('caído'))
		await expect(makeLogout({ auth: down })()).rejects.toBeInstanceOf(UnavailableError)

		const twoCookies = new InMemoryAuthRepository()
		twoCookies.logout = vi.fn().mockRejectedValue(new AmbiguousSessionError())
		await expect(makeLogout({ auth: twoCookies })()).rejects.toBeInstanceOf(
			AmbiguousSessionError,
		)
	})
})
