import { describe, expect, it } from 'vitest'
import { makeGetMe, MfaRequired, SessionExpired } from '../..'
import { InMemoryAuthRepository } from '../../repositories/__fakes__/inMemoryAuthRepository'

describe('getMe', () => {
	const users = [{ username: 'felipe', password: 'una-contraseña-larga', totpEnabled: true }]

	it('devuelve quién tiene la sesión', async () => {
		const auth = new InMemoryAuthRepository({ users })
		auth.session = { username: 'felipe', mfaPending: false, needsTotpSetup: false }

		await expect(makeGetMe({ auth })()).resolves.toEqual({
			username: 'felipe',
			totpEnabled: true,
			needsTotpSetup: false,
		})
	})

	it('distingue "sin sesión" de "falta el código"', async () => {
		const auth = new InMemoryAuthRepository({ users })
		await expect(makeGetMe({ auth })()).rejects.toBeInstanceOf(SessionExpired)

		auth.session = { username: 'felipe', mfaPending: true, needsTotpSetup: false }
		await expect(makeGetMe({ auth })()).rejects.toBeInstanceOf(MfaRequired)
	})
})
