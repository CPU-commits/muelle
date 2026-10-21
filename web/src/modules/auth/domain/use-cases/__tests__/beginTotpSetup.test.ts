import { describe, expect, it } from 'vitest'
import { makeBeginTotpSetup, SessionExpired } from '../..'
import {
	FAKE_TOTP_SETUP,
	InMemoryAuthRepository,
} from '../../repositories/__fakes__/inMemoryAuthRepository'

describe('beginTotpSetup', () => {
	it('devuelve el secreto y la URL otpauth para el QR', async () => {
		const auth = new InMemoryAuthRepository()
		auth.session = { username: 'felipe', mfaPending: false, needsTotpSetup: true }

		await expect(makeBeginTotpSetup({ auth })()).resolves.toEqual(FAKE_TOTP_SETUP)
		expect(auth.pendingTotp).toBe(true)
	})

	it('sin sesión propaga SessionExpired', async () => {
		const auth = new InMemoryAuthRepository()

		await expect(makeBeginTotpSetup({ auth })()).rejects.toBeInstanceOf(SessionExpired)
	})
})
